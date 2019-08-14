package api

import (
	"net/http"
	"encoding/json"
	"log"
	"errors"

	"github.com/geobeau/k8s-ephemeral-resources/controller"

	"github.com/gorilla/mux"
)

type requestData struct {
	Owner string
}

// GetResource display all instances for a type of resource
func GetResource(w http.ResponseWriter, r *http.Request, c controller.Controller) {
	resourceName := mux.Vars(r)["resource"]
	resource := c.Resources[resourceName]
	log.Println(resource)
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// CreateResource create a new instance of a resource
func CreateResource(w http.ResponseWriter, r *http.Request,  c controller.Controller) {
	resourceName := mux.Vars(r)["resource"]
	if r.Body == nil {
		wrapError(errors.New("Invalid JSON. Please provide owner like: {\"Owner\":\"resourceowner\"}"), w, http.StatusBadRequest)
		return
	}
	requestData := requestData{}
	err := json.NewDecoder(r.Body).Decode(&requestData)
	if err != nil {
		wrapError(errors.New("Invalid JSON. Please provide owner like: {\"Owner\":\"resourceowner\"}"), w, http.StatusBadRequest)
		return
	}
	instance, err := c.CreateNewInstance(resourceName, requestData.Owner)
	if err != nil {
		wrapError(err, w, http.StatusInternalServerError)
		return
	}
	response := instance.ToStringMap()
	json.NewEncoder(w).Encode(response)
}

// DeleteResource delete an instnace of a resource
func DeleteResource(w http.ResponseWriter, r *http.Request,  c controller.Controller) {
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func wrapError(err error, w http.ResponseWriter, status int) {
	log.Println("API returned error: ", err)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"ok": "false", "reason":err.Error()})
}