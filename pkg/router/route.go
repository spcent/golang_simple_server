package router

import (
	"fmt"
	"net/http"
	"strings"
)

const (
	GET    = "GET"
	POST   = "POST"
	PUT    = "PUT"
	DELETE = "DELETE"
	PATCH  = "PATCH"
	ANY    = "ANY"
)

var routes = map[string][]Route{}

// Handler defines the function signature for handling HTTP requests.
type Handler func(http.ResponseWriter, *http.Request, map[string]string)

type Route struct {
	Method  string
	Path    string
	Handler Handler
}

func Get(path string, handler Handler) {
	addRoute(GET, path, handler)
}

func Post(path string, handler Handler) {
	addRoute(POST, path, handler)
}

func Put(path string, handler Handler) {
	addRoute(PUT, path, handler)
}

func Delete(path string, handler Handler) {
	addRoute(DELETE, path, handler)
}

func Patch(path string, handler Handler) {
	addRoute(PATCH, path, handler)
}

func Any(path string, handler Handler) {
	addRoute(ANY, path, handler)
}

func PrintRoutes() {
	fmt.Println("Registered Routes:")
	for method, methodRoutes := range routes {
		for _, route := range methodRoutes {
			fmt.Printf("%-6s %s\n", method, route.Path)
		}
	}
}

func addRoute(method, path string, handler Handler) {
	if _, exists := routes[method]; !exists {
		routes[method] = make([]Route, 0)
	}

	routes[method] = append(routes[method], Route{
		Method:  method,
		Path:    path,
		Handler: handler,
	})
}

func Handle(w http.ResponseWriter, r *http.Request) {
	method := r.Method
	if methodRoutes, exists := routes[method]; exists {
		for _, route := range methodRoutes {
			params, ok := matchRoute(route.Path, r.URL.Path)
			if ok {
				route.Handler(w, r, params)
				return
			}
		}
	}

	if anyRoutes, exists := routes[ANY]; exists {
		for _, route := range anyRoutes {
			params, ok := matchRoute(route.Path, r.URL.Path)
			if ok {
				route.Handler(w, r, params)
				return
			}
		}
	}

	http.NotFound(w, r)
}

// matchRoute("/users/{id}/posts/{postID}", "/users/123/posts/456") => {"id": "123", "postID": "456"}
// support {param}
func matchRoute(routePath, reqPath string) (map[string]string, bool) {
	routeParts := splitPath(routePath)
	reqParts := splitPath(reqPath)
	if len(routeParts) != len(reqParts) {
		return nil, false
	}

	params := make(map[string]string)
	for i := 0; i < len(routeParts); i++ {
		if strings.HasPrefix(routeParts[i], "{") && strings.HasSuffix(routeParts[i], "}") {
			key := routeParts[i][1 : len(routeParts[i])-1]
			params[key] = reqParts[i]
		} else if routeParts[i] != reqParts[i] {
			return nil, false
		}
	}
	return params, true
}

// splitPath splits the path into segments, ignoring empty parts.
// Example:
// ("/users/123/posts/456") => ["users", "123", "posts", "456"]
func splitPath(path string) []string {
	parts := strings.Split(path, "/")
	segments := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			segments = append(segments, p)
		}
	}
	return segments
}
