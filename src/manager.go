package main

import (
	"flag"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

func main() {
	kubeconfig := flag.String("kubeconfig", filepath.Join(os.Getenv("HOME"), ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	httpListenPort := flag.Int("httpListenPort", 8080, "Port on which the http server should bind on")
	verboseMode := flag.Bool("verbose", false, "Enable verbose logging of the app")
	namespaceToWatch := flag.String("filterNamespaces", "", "Regex to match in order for the namespace name to be watched i.e: mem|couch")
	dryRun := flag.Bool("dry-run", false, "if enabled do not trigger any actions on faulty cluster/namespace/pod")
	retaliateGracePeriodFlag := flag.Int("retaliateGracePeriodMin", 10, "For how long in minute the cluster should be in an unhealthy state before retaliating")
	runInsideKube := flag.Bool("runInsideKube", false, "if true will setup")
	flag.Parse()

	retaliateGracePeriod := time.Duration(*retaliateGracePeriodFlag) * time.Minute
	log.Info("GracePeriod before killing a pod is ", retaliateGracePeriod)

	if *verboseMode {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	isAllowedNamespace, err := regexp.Compile(*namespaceToWatch)
	if err != nil {
		log.Fatal("Cannot compile the regex '", *namespaceToWatch, "': ", err)
	}

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Start prometheus endpoint
	log.WithField("component", "HTTPServer").Info("Starting HTTP server on port ", *httpListenPort)
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":"+strconv.Itoa(*httpListenPort), nil)
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Create handle to the kube API
	var config *rest.Config
	if *runInsideKube {
		config, err = rest.InClusterConfig()
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	}

	if err != nil {
		log.Fatal("Cannot create the kube client driver ", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal("Cannot create the kube client driver ", err)
	}
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Build the function that will kill the pod
	var killPod func(string, string, v1.PodStatus)
	if *dryRun {
		killPod = func(namespace string, podName string, pod v1.PodStatus) {
			log.WithField("namespace", namespace).Info("FAKE Killing pod ", podName)
		}
	} else {
		killPod = func(namespace string, podName string, pod v1.PodStatus) {
			// Force the grace period to 0
			deleteOptions := metav1.DeleteOptions{GracePeriodSeconds: new(int64)}
			log.WithField("namespace", namespace).Info("KILLING pod ", podName)
			err := clientset.CoreV1().Pods(namespace).Delete(podName, &deleteOptions)
			if err != nil {
				log.WithField("namespace", namespace).Info("Cannot Kill pod ", podName, ": ", err)
			}
		}
	}
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Start watching changes in the available namespaces
	watchNamespaces(clientset, isAllowedNamespace, retaliateGracePeriod, killPod)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
//  - In our setup, we have a mapping where 1 namespace == 1 cluster
// 	- On connect, the API will send an ADDED event for all the already existing namespaces
// 	- We don't care of the MODIFIED event only ADDED/DELETE, aka creation/deletion of a cluster
func watchNamespaces(clientset *kubernetes.Clientset, isAllowedNamespace *regexp.Regexp, retaliateGracePeriod time.Duration, killPod func(string, string, v1.PodStatus)) {
	watcherContexts := make(map[string]WatcherContext)

restart:
	for {
		namespaces, err := clientset.CoreV1().Namespaces().Watch(metav1.ListOptions{})
		if err != nil {
			log.Fatal("Cannot watch namespaces changes from kubeAPI: ", err)
		}

		for event := range namespaces.ResultChan() {
			switch event.Type {
			case watch.Added:
				namespaceName := event.Object.(*v1.Namespace).Name
				log.WithField("namespace", namespaceName).Debug("Namespace ", namespaceName, " has been added")

				// If we don't want to watch this namespace
				if !isAllowedNamespace.MatchString(namespaceName) {
					break
				}

				// In case of the watcher being restarted, we will receive ADDED events again
				// So avoid overwriting our context and having spawn 2 goroutines for a namespace and leak 1 goroutine in the wild.
				if _, isPresent := watcherContexts[namespaceName]; isPresent {
					break
				}

				// Normal case when we start to watch this specific namespace
				context := makeWatcherContext(namespaceName, retaliateGracePeriod, killPod)
				watcherContexts[namespaceName] = context
				go watchPodsInNamespace(clientset, &context)
				break

			case watch.Deleted:
				namespaceName := event.Object.(*v1.Namespace).Name
				log.WithField("namespace", namespaceName).Debug("Namespace ", namespaceName, " has been deleted")
				watcherContext, isPresent := watcherContexts[namespaceName]
				if !isPresent || watcherContext.namespaceName == "" {
					break
				}

				watcherContext.stop <- struct{}{}
				delete(watcherContexts, namespaceName)
				break

			case watch.Modified:
				namespaceName := event.Object.(*v1.Namespace).Name
				log.WithField("namespace", namespaceName).Debug("Namespace ", event.Type, " ", event.Object.(*v1.Namespace).Status)
				break

			case watch.Error:
				fallthrough
			default:
				log.Error("Event ", event.Type, " ", event.Object)
				if event.Object == nil {
					log.Error("Restarting watcher as it is closed")
					namespaces.Stop()
					goto restart
				}
				break
			}
		}
	}
}

func watchPodsInNamespace(clientset *kubernetes.Clientset, context *WatcherContext) {
	logger := log.WithField("namespace", context.namespaceName)

restart:
	podsEvents, err := clientset.CoreV1().Pods(context.namespaceName).Watch(metav1.ListOptions{})
	if err != nil {
		logger.Error("Cannot watch pods change from kubeAPI", err)
		return
	}

	// Endless loop in order to wait
	logger.Info("Starting to watch pods change")
	for {
		select {

		// When the namespace has been deleted, we need to stop the routine
		// So wait for a signal from the main thread
		case <-context.stop:
			logger.Info("Notified to stop, stopping to watch for pods changes")
			podsEvents.Stop()
			return

		// Force a check every minute in case there is no change in the pods states
		// and that the cluster is in an unhealthy state
		case <-time.After(1 * time.Minute):
			break

		// Main loop where we store the state of all pods of the namespace/cluster
		// There is no logic there, we only record the state of the pods
		case podEvent := <-podsEvents.ResultChan():
			switch podEvent.Type {

			case watch.Added, watch.Modified:
				pod := podEvent.Object.(*v1.Pod)
				logger.WithField("pod", pod.Name).Info("Pod ", podEvent.Type)
				context.podsStatus[pod.Name] = pod.Status
				break

			case watch.Deleted:
				pod := podEvent.Object.(*v1.Pod)
				logger.WithField("pod", pod.Name).Info("Pod ", podEvent.Type)
				delete(context.podsStatus, pod.Name)
				break

			case watch.Error:
				fallthrough
			default:
				logger.Error("Event ", podEvent.Type, " ", podEvent.Object)
				if podEvent.Object == nil {
					logger.Error("Restarting watcher as it is close")
					podsEvents.Stop()
					goto restart
				}
				break
			}
		}

		context.updateClusterState()
		logger.Info("Cluster state is ", context.clusterState.health)
		for podName := range context.clusterState.unhealthyPods {
			logger.Info("pod ", podName, " is unhealthy")
		}
		if retaliate(context) {
			context.clusterState.since = time.Now()
		}

	}

}

func retaliate(context *WatcherContext) bool {
	logger := log.WithField("namespace", context.namespaceName)
	// Do nothing if the cluster is HEALTHY or is the state of the cluster
	// has been not been stable for enough time
	elapsed := time.Now().Sub(context.clusterState.since)
	if context.clusterState.health == HEALTHY || elapsed < context.retaliateGracePeriod {
		return false
	}

	// Safeguard to avoid a killing spree
	// Do nothing if there is more than 1 pod unhealthy in a cluster/namespace
	if len(context.clusterState.unhealthyPods) > 1 {
		logger.Info(len(context.clusterState.unhealthyPods), " pods unhealthy, doing nothing")
		return false
	}

	for podName, pod := range context.clusterState.unhealthyPods {
		context.podKilledCounter.Inc()
		context.killPod(context.namespaceName, podName, pod)
	}

	return true
}
