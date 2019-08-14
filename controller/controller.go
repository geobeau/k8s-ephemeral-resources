package controller

import (
	"fmt"
	"errors"
	"log"
	"strings"
	"bytes"
	"text/template"
	"encoding/json"
	"time"
	"strconv"

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
		ExpirationDate: time.Now().Add(resource.DurationDefault).Unix(),
	}
	labels := make(map[string]string)
	labels["k8s-ephemeral-resource"] = name
	labels["ExpirationDate"] = strconv.FormatInt(instance.ExpirationDate, 10)
	namespace := &apiv1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: identifier, Labels: labels}}

	log.Println("Creating namespace: ", identifier)

	_, err := c.kubeClient.CoreV1().Namespaces().Create(namespace)
	if err != nil {
		return instance, err
	}

	log.Println("Parsing deployment configuration")
	deployment, err := instance.GenerateKubeDeploymentFromTemplate(resource.DeploymentTemplate)
	if err != nil {
		return instance, err
	}

	log.Println("Creating kubernetes deployment")
	_, err = c.kubeClient.AppsV1beta2().Deployments(identifier).Create(&deployment)
	if err != nil {
		return instance, err
	}

	log.Println("Parsing service configuration")
	service, err := instance.GenerateKubeServiceFromTemplate(resource.ServiceTemplate)
	if err != nil {
		return instance, err
	}

	log.Println("Creating kubernetes service")
	_, err = c.kubeClient.CoreV1().Services(identifier).Create(&service)
	if err != nil {
		log.Println("Error while create resource, removing namespace")
		c.kubeClient.CoreV1().Namespaces().Delete(identifier, nil)
		return instance, err
	}

	return instance, nil
}

// CleanupLoop wakes up every @delay to remove expired resources
func (c *Controller) CleanupLoop(delay time.Duration) {
	for {
		log.Println("Running verification loop")
		for _, resource := range c.Resources {
			listOptions := metav1.ListOptions{LabelSelector: "k8s-ephemeral-resource="+resource.Name}
			list, err := c.kubeClient.CoreV1().Namespaces().List(listOptions)
			if err != nil {
				log.Println("Error:", err)
				continue
			}
			for _, namespace := range list.Items {
				expirationDateStr, ok := namespace.Labels["ExpirationDate"]
				if ok != true {
					log.Printf("Ignoring: %s, expiration label not found", namespace.Name)
					continue
				}
				expirationEpoch, err := strconv.ParseInt(expirationDateStr, 10, 64)
				if err != nil {
					log.Println("Error:", err)
					continue
				}
				expirationDate := time.Unix(expirationEpoch, 0)
				if time.Now().After(expirationDate) {
					log.Printf("%s is expired: now:%s / expire at:%s", namespace.Name, time.Now(), expirationDate)
					log.Printf("Removing %s", namespace.Name)
					err = c.kubeClient.CoreV1().Namespaces().Delete(namespace.Name, nil)
					if err != nil {
						log.Println("Error:", err)
						continue
					}
				}
			}
		}
		time.Sleep(delay)
	}
}

// Resource is a type of resource that can contains instances
type Resource struct {
	Name				string			`yaml:"resourceName"`
	DurationDefault     time.Duration 	`yaml:"durationDefault"`
	DeploymentTemplate	string			`yaml:"deploymentTemplate"`
	ServiceTemplate		string			`yaml:"serviceTemplate"`
}

// Instance is an instance of resource
type Instance struct {
	Namespace		string
	ExpirationDate	int64
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
	if err != nil {
		return apiv1.Service{}, err
	}
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