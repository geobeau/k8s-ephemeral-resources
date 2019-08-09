// Contains all the custom types that the application use
package main

import (
	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	v1 "k8s.io/api/core/v1"
	"time"
)

// WatcherContext is the main type of the application
// It holds all the information necessary to watch a single cluster/namespace over time
type WatcherContext struct {
	namespaceName        string                             // name of the namespace this watcher is responsible of
	podsStatus           map[string]v1.PodStatus            // Store the status of all the running pods
	clusterState         ClusterState                       // State of the
	killPod              func(string, string, v1.PodStatus) // Function to use when we want to kill a pod (namespaceName, podName, pod)
	retaliateGracePeriod time.Duration
	stop                 chan struct{}                      // Channel the watcher is listenning on in order to know when to watch for changes from the API
	podKilledCounter     prometheus.Counter                 // Metric regarding the number of pods the watcher has killed
}

// ctr for WatcherContext
func makeWatcherContext(namespaceName string, gracePeriod time.Duration, killPod func(string, string, v1.PodStatus)) WatcherContext {
	return WatcherContext{
		namespaceName:        namespaceName,
		podsStatus:           make(map[string]v1.PodStatus),
		clusterState:         makeClusterState(map[string]v1.PodStatus{}),
		killPod:              killPod,
		retaliateGracePeriod: gracePeriod,
		stop:                 make(chan struct{}),
		podKilledCounter: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "statefulmanager_pods_killed",
			Help:        "The total number of time a given pod has been killed by the app",
			ConstLabels: map[string]string{"namespace": namespaceName},
		}),
	}
}

// evaluateClusterState is responsible of creating the state of the cluster from the current state of the pods
// it does not modify the the current watchercontext
func (watcher *WatcherContext) evaluateClusterState() ClusterState {
	isPodHealthy := func(pod *v1.PodStatus) bool {
		if pod.Phase != v1.PodRunning {
			return false
		}

		// Conditions holds the phase of the pod {Scheduled, Initialized, ContainerReady, Ready, ...}
		// If one step is missing, it means that the pod is not fully functional
		for _, condition := range pod.Conditions {
			if condition.Status != v1.ConditionTrue {
				return false
			}
		}

		return true
	}

	unhealthyPods := make(map[string]v1.PodStatus)
	for podName, pod := range watcher.podsStatus {
		if !isPodHealthy(&pod) {
			unhealthyPods[podName] = pod
		}
	}

	return makeClusterState(unhealthyPods)
}

// updateClusterState update the current cluster state if needed
// It important to not override the current state if the new state has not changed as we rely on clusterState.since
// in order to know for how long the cluster has been in the same state
// A better approach would have been to use a ring buffer with all the states, but go does not have it ...
func (watcher *WatcherContext) updateClusterState() {
	state := watcher.evaluateClusterState()
	if watcher.clusterState.health != state.health {
		watcher.clusterState = state
		return
	}

	if len(watcher.clusterState.unhealthyPods) != len(state.unhealthyPods) {
		watcher.clusterState = state
		return
	}

	for podName, pod := range watcher.clusterState.unhealthyPods {
		neoPod, exist := state.unhealthyPods[podName]
		if exist == false {
			watcher.clusterState = state
			return
		}

		if neoPod.Phase != pod.Phase || !cmp.Equal(neoPod.Conditions, pod.Conditions) {
			watcher.clusterState = state
			return
		}

	}
}

// clusterHealth is an high lvl state of the cluster
type ClusterHealth string

const (
	UNHEALTHY ClusterHealth = "UNHEALTHY"
	HEALTHY   ClusterHealth = "HEALTHY"
)

// clusterState is an high lvl view of the cluster state
// It only records the information needed in order for the watcher to do actions in case of UNHEALTHYness
type ClusterState struct {
	health        ClusterHealth
	unhealthyPods map[string]v1.PodStatus
	since         time.Time // Since when the cluster is in this state
}

// ctr of makeClusterState
func makeClusterState(unhealthyPods map[string]v1.PodStatus) ClusterState {
	var health ClusterHealth
	if len(unhealthyPods) > 0 {
		health = UNHEALTHY
	} else {
		health = HEALTHY
	}

	return ClusterState{
		health:        health,
		unhealthyPods: unhealthyPods,
		since:         time.Now(),
	}
}
