package controller

import (
	"fmt"
	"errors"
	"log"
	"strings"
	"bytes"
	"text/template"
	"encoding/json"

	"github.com/lithammer/shortuuid"
	"github.com/ghodss/yaml"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1beta2"
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
	u := strings.ToLower(shortuuid.New())
	identifier := fmt.Sprintf("%s%s-%s", c.suffix, resource.Name, u)

	instance := Instance{
		Namespace: identifier,
	}

	namespace := &apiv1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: identifier}}

	log.Println("Creating namespace: ", identifier)

	_, err := c.kubeClient.CoreV1().Namespaces().Create(namespace)
	if err != nil {
		log.Println(err.Error())
		return instance, nil
	}

	log.Println("Parsing deployment configuration")
	deployment, err := instance.GenerateKubeDeploymentFromTemplate(resource.DeploymentTemplate)
	if err != nil {
		log.Println(err.Error())
		return instance, nil
	}

	log.Println("Creating kubernetes deployment")
	_, err = c.kubeClient.AppsV1beta2().Deployments(identifier).Create(&deployment)
	if err != nil {
		log.Println(err.Error())
		return instance, nil
	}

	log.Println("Parsing service configuration")
	service, err := instance.GenerateKubeServiceFromTemplate(resource.ServiceTemplate)
	if err != nil {
		log.Println(err.Error())
		return instance, nil
	}

	log.Println("Creating kubernetes service")
	_, err = c.kubeClient.CoreV1().Services(identifier).Create(&service)
	if err != nil {
		log.Println(err.Error())
		return instance, nil
	}

	return instance, nil
}

// Resource is a type of resource that can contains instances
type Resource struct {
	Name				string `yaml:"resourceName"`
	DeploymentTemplate	string `yaml:"deploymentTemplate"`
	ServiceTemplate	string `yaml:"serviceTemplate"`
}

// Instance is an instance of resource
type Instance struct {
	Namespace string
}

// ToStringMap returns a string map representation of the object
func (i *Instance) ToStringMap() map[string]string {
	result := make(map[string]string)
	result["name"] = i.Namespace
	return result
}

// GenerateKubeDeploymentFromTemplate Generate a kubernetes deployment from template
func (i *Instance) GenerateKubeDeploymentFromTemplate(templateString string) (appsv1.Deployment, error) {
	deployment, err := i.generateConfigFromTemplate(templateString)

	jsonBytes, err := yaml.YAMLToJSON([]byte(deployment))
	if err != nil {
		return appsv1.Deployment{}, err
	}

	var kubeDeployment = appsv1.Deployment{}
	err = json.Unmarshal(jsonBytes, &kubeDeployment)
	if err != nil {
		return kubeDeployment, err
	}
	return kubeDeployment, nil
}

// GenerateKubeServiceFromTemplate Generate a kubernetes service from template
func (i *Instance) GenerateKubeServiceFromTemplate(templateString string) (apiv1.Service, error) {
	service, err := i.generateConfigFromTemplate(templateString)

	jsonBytes, err := yaml.YAMLToJSON([]byte(service))
	if err != nil {
		return apiv1.Service{}, err
	}

	var kubeService = apiv1.Service{}
	err = json.Unmarshal(jsonBytes, &kubeService)
	if err != nil {
		return kubeService, err
	}
	return kubeService, nil
}

// generateDeploymentFromTemplate Generate a deployment from template
func (i *Instance) generateConfigFromTemplate(templateString string) (string, error) {
	tmpl, err := template.New(i.Namespace).Parse(templateString)
	if err != nil {
		return "", err 
	}

	var resultBytes bytes.Buffer
	err = tmpl.Execute(&resultBytes, i)
	if err != nil {
		return "", err 
	}
	return resultBytes.String(), nil
}