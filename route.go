package main

import (
	"net/http"
	"strings"
)

var routes []Route

type Handler func(http.ResponseWriter, *http.Request, map[string]string)

// 路由注册表
type Route struct {
	Path    string
	Handler Handler
}

// 添加路由
func AddRoute(path string, handler Handler) {
	routes = append(routes, Route{Path: path, Handler: handler})
}

// 路由匹配器
func routerHandler(w http.ResponseWriter, r *http.Request) {
	for _, route := range routes {
		params, ok := matchRoute(route.Path, r.URL.Path)
		if ok {
			route.Handler(w, r, params)
			return
		}
	}
	http.NotFound(w, r)
}

// 匹配路径，支持 {param} 参数
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

// 路径拆分
func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}
