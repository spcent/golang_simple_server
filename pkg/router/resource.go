package router

import (
	"net/http"
)

type ResourceController interface {
	Index(w http.ResponseWriter, r *http.Request, params map[string]string)
	Show(w http.ResponseWriter, r *http.Request, params map[string]string)
	Create(w http.ResponseWriter, r *http.Request, params map[string]string)
	Update(w http.ResponseWriter, r *http.Request, params map[string]string)
	Delete(w http.ResponseWriter, r *http.Request, params map[string]string)
	Patch(w http.ResponseWriter, r *http.Request, params map[string]string)
}

type BaseResourceController struct{}

func (c *BaseResourceController) Index(w http.ResponseWriter, r *http.Request, params map[string]string) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}

func (c *BaseResourceController) Show(w http.ResponseWriter, r *http.Request, params map[string]string) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}

func (c *BaseResourceController) Create(w http.ResponseWriter, r *http.Request, params map[string]string) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}

func (c *BaseResourceController) Update(w http.ResponseWriter, r *http.Request, params map[string]string) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}

func (c *BaseResourceController) Delete(w http.ResponseWriter, r *http.Request, params map[string]string) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}

func (c *BaseResourceController) Patch(w http.ResponseWriter, r *http.Request, params map[string]string) {
	http.Error(w, "Not Implemented", http.StatusNotImplemented)
}
