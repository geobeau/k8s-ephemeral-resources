package controller

import (
	"fmt"
	"errors"

	"github.com/lithammer/shortuuid"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	Resources 	map[string]Resource
	kubeClient	*kubernetes.Clientset
	suffix 		string
}

// NewControllerFromConfig return a new controller from configuration
func NewControllerFromConfig(config Config, kubeClient *kubernetes.Clientset, suffix string) Controller {
	resources := make(map[string]Resource)
	for _, resource := range config.Resources {
		resources[resource.Name] = resource
	}
	return Controller{
		Resources: resources,
		kubeClient: kubeClient,
		suffix: suffix,
	}
}

// CreateNewInstance creates a new instance inside Kubernetes
func (c *Controller) CreateNewInstance(name string) (Instance, error) {
	resource, ok := c.Resources[name]
	if ok != true {
		return Instance{}, errors.New("Resource Not found")
	}
	u := shortuuid.New()
	identifier := fmt.Sprintf("%s%s-%s", c.suffix, resource.Name, u)

	namespace := &apiv1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: identifier}}
	c.kubeClient.CoreV1().Namespaces().Create(namespace)
	
	return Instance{
		name: identifier,
	}, nil
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

// ToStringMap returns a string map representation of the object
func (i *Instance) ToStringMap() map[string]string {
	result := make(map[string]string)
	result["name"] = i.name
	return result
}