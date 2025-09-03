package middleware

import (
	"net/http"
)

type Middleware func(http.HandlerFunc) http.HandlerFunc

var middlewares []Middleware

func Use(middleware Middleware) {
	middlewares = append(middlewares, middleware)
}

func Apply(h http.HandlerFunc, middlewares ...Middleware) http.HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}

	for _, m := range middlewares {
		h = m(h)
	}

	return h
}
