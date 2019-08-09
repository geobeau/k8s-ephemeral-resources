package main

import (
	"os"
	"path/filepath"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/geobeau/k8s-ephemeral-resources/api"
	"github.com/geobeau/k8s-ephemeral-resources/controller"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	yaml "gopkg.in/yaml.v3"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"github.com/gorilla/mux"
)


func main() {
	app := kingpin.New("k8s-ephemeral-resources", "A controller to manage and deploy short lived resources")
	app.HelpFlag.Short('h')

	confPath := app.Flag("conf", "Configuration to be used by the manager").Short('c').Required().String()
	
	kubeconfig := app.Flag("kubeconfig", "(optional) absolute path to a kubeconfig file").Default(filepath.Join(os.Getenv("HOME"), ".kube", "config")).String()
	runInsideKube := app.Flag("runInsideKube", "if true will setup").Default("false").Bool()
	httpListenPort := app.Flag("httpListenPort", "Port on which the http server should bind on").Default("8080").String()
	app.Parse(os.Args[1:])

	// Parsing Configuration
	config := controller.Config{}

	log.Println("Reading configuration file:", *confPath)
	data, err := ioutil.ReadFile(*confPath)
	if err != nil {
		log.Fatalf("error while reading %s: %v", *confPath, err)
	}
	err = yaml.Unmarshal([]byte(data), &config)
	if err != nil {
		log.Fatalf("error while parsing yaml: %v", err)
	}

	// Init kubernetes controller
	var k8sConfig *rest.Config
	if *runInsideKube {
		k8sConfig, err = rest.InClusterConfig()
	} else {
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	}

	if err != nil {
		log.Fatal("Cannot create the kube client driver ", err)
	}
	kubeClient, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		log.Fatal("Cannot create the kube client driver ", err)
	}

	contrl := controller.NewControllerFromConfig(config, kubeClient)

	r := mux.NewRouter()
	r.HandleFunc("/resources/{resource}", func(w http.ResponseWriter, r *http.Request) {
		api.GetResource(w, r, contrl)
	}).Methods("GET")
	r.HandleFunc("/resources/{resource}", func(w http.ResponseWriter, r *http.Request) {
		api.CreateResource(w, r, contrl)
	}).Methods("POST")
	r.HandleFunc("/resources/{resource}/{resourceId}", func(w http.ResponseWriter, r *http.Request) {
		api.DeleteResource(w, r, contrl)
	}).Methods("DELETE")
	http.Handle("/", r)

	log.Println("Serving api on:", *httpListenPort)
	log.Fatal(http.ListenAndServe(":" + *httpListenPort, nil))
}

