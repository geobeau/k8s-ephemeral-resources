package controller

import (
	"k8s.io/client-go/kubernetes"
)

// Config is an Ephemeral resources manager configuration
type Config struct {
	Resources []Resource `yaml:"resources"`
}

// ResourceConfig is a resource definition
type ResourceConfig struct {
	ResourceName		string `yaml:"resourceName"`
	DeploymentTemplate	string `yaml:"deploymentTemplate"`
	ResourceTemplate	string `yaml:"serviceTemplate"`
}

// Controller controls a set of Resources
type Controller struct {
	Resources map[string]Resource
	kubeConfig *kubernetes.Clientset
}

// NewControllerFromConfig return a new controller from configuration
func NewControllerFromConfig(config Config, kubeConfig *kubernetes.Clientset) Controller {
	resources := make(map[string]Resource)
	for _, resource := range config.Resources {
		resources[resource.Name] = resource
	}
	return Controller{
		Resources: resources,
		kubeConfig: kubeConfig,
	}
}

// Resource is a type of resource that can contains instances
type Resource struct {
	Name				string `yaml:"resourceName"`
	DeploymentTemplate	string `yaml:"deploymentTemplate"`
	ResourceTemplate	string `yaml:"serviceTemplate"`
}

// Instance is an instance of resource
type Instance struct {
	name string
}