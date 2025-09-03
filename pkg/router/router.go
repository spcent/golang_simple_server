package router

import "net/http"

type RouteRegistrar interface {
	RegisterRoutes(mux *http.ServeMux)
}

type Router struct {
	mux        *http.ServeMux
	registrars []RouteRegistrar
}

func NewRouter(mux ...*http.ServeMux) *Router {
	if len(mux) == 0 {
		mux = append(mux, http.NewServeMux())
	}
	return &Router{
		mux: mux[0],
	}
}

func (r *Router) Register(registrar RouteRegistrar) {
	r.registrars = append(r.registrars, registrar)
}

func (r *Router) Init() {
	for _, registrar := range r.registrars {
		registrar.RegisterRoutes(r.mux)
	}
}

func (r *Router) Handler() http.Handler {
	return r.mux
}
