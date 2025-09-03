package router

import (
	"net/http"
	"strings"
)

var routes []Route

type Handler func(http.ResponseWriter, *http.Request, map[string]string)

type Route struct {
	Path    string
	Handler Handler
}

func AddRoute(path string, handler Handler) {
	routes = append(routes, Route{Path: path, Handler: handler})
}

func Handle(w http.ResponseWriter, r *http.Request) {
	for _, route := range routes {
		params, ok := matchRoute(route.Path, r.URL.Path)
		if ok {
			route.Handler(w, r, params)
			return
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

// SplitPath("/users/123/posts/456") => ["users", "123", "posts", "456"]
func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}
