package api

import (
	"net/http"
	"encoding/json"
	"log"

	"github.com/geobeau/k8s-ephemeral-resources/controller"

	"github.com/gorilla/mux"
)

// GetResource display all instances for a type of resource
func GetResource(w http.ResponseWriter, r *http.Request, c controller.Controller) {
	resourceName := mux.Vars(r)["resource"]
	resource := c.Resources[resourceName]
	log.Println(resource)
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// CreateResource create a new instance of a resource
func CreateResource(w http.ResponseWriter, r *http.Request,  c controller.Controller) {
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// DeleteResource delete an instnace of a resource
func DeleteResource(w http.ResponseWriter, r *http.Request,  c controller.Controller) {
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}