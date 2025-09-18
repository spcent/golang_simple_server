package router

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
)

const (
	GET    = "GET"
	POST   = "POST"
	PUT    = "PUT"
	DELETE = "DELETE"
	PATCH  = "PATCH"
	ANY    = "ANY"
)

// Handler defines the function signature for handling HTTP requests.
type Handler func(http.ResponseWriter, *http.Request, map[string]string)

type RouteRegistrar interface {
	Register(r *Router)
}

type segment struct {
	raw       string
	isParam   bool
	isWild    bool
	paramName string
}

type node struct {
	segment    string
	children   map[string]*node
	paramChild *node
	wildChild  *node
	paramName  string
	handler    Handler
	paramKeys  []string
}

type route struct {
	Method string
	Path   string
}

type Router struct {
	trees      map[string]*node
	registrars []RouteRegistrar
	routes     map[string][]route
	frozen     bool
	mu         sync.RWMutex
}

func NewRouter() *Router {
	return &Router{
		trees:  make(map[string]*node),
		routes: make(map[string][]route),
	}
}

func (r *Router) Freeze() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frozen = true
}

func (r *Router) Register(registrars ...RouteRegistrar) {
	r.mu.Lock()
	defer r.mu.Unlock()

	seen := make(map[RouteRegistrar]bool)
	for _, reg := range r.registrars {
		seen[reg] = true
	}
	for _, reg := range registrars {
		if !seen[reg] {
			r.registrars = append(r.registrars, reg)
			reg.Register(r)
		}
	}
}

func (r *Router) Group(prefix string) *Router {
	return &Router{
		trees:  r.trees,
		routes: r.routes,
		frozen: r.frozen,
	}
}

func (r *Router) AddRoute(method, path string, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.frozen {
		panic("router is frozen, cannot add route after freeze")
	}

	fullPath := strings.TrimRight(path, "/")
	if fullPath == "" {
		fullPath = "/"
	}

	if r.trees[method] == nil {
		r.trees[method] = &node{children: make(map[string]*node)}
	}

	segments := compileTemplate(fullPath)
	current := r.trees[method]
	var paramKeys []string

	for i, seg := range segments {
		switch {
		case seg.isWild:
			if i != len(segments)-1 {
				panic("wildcard must be the last segment: " + fullPath)
			}
			if current.wildChild != nil {
				panic("duplicate wildcard route: " + fullPath)
			}
			current.wildChild = &node{segment: seg.raw, paramName: seg.paramName}
			paramKeys = append(paramKeys, seg.paramName)
			current = current.wildChild

		case seg.isParam:
			if current.paramChild != nil && current.paramChild.paramName != seg.paramName {
				panic("conflicting param route at: " + seg.raw)
			}
			if current.paramChild == nil {
				current.paramChild = &node{
					segment:   seg.raw,
					children:  make(map[string]*node),
					paramName: seg.paramName,
				}
			}
			paramKeys = append(paramKeys, seg.paramName)
			current = current.paramChild

		default:
			child, ok := current.children[seg.raw]
			if !ok {
				child = &node{
					segment:  seg.raw,
					children: make(map[string]*node),
				}
				current.children[seg.raw] = child
			}
			current = child
		}
	}

	if current.handler != nil {
		panic("duplicate route registration: " + method + " " + fullPath)
	}
	current.handler = handler
	current.paramKeys = paramKeys

	r.routes[method] = append(r.routes[method], route{Method: method, Path: fullPath})
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	method := req.Method
	tree := r.trees[method]
	if tree == nil {
		tree = r.trees[ANY]
		if tree == nil {
			http.NotFound(w, req)
			return
		}
	}

	path := req.URL.Path
	if path != "/" {
		path = strings.Trim(path, "/")
	}
	parts := []string{}
	if path != "/" && path != "" {
		parts = strings.Split(path, "/")
	}

	params := make(map[string]string)
	current := tree

	for i, part := range parts {
		if child, ok := current.children[part]; ok {
			current = child
			continue
		}
		if current.paramChild != nil {
			params[current.paramChild.paramName] = part
			current = current.paramChild
			continue
		}
		if current.wildChild != nil {
			params[current.wildChild.paramName] = strings.Join(parts[i:], "/")
			current = current.wildChild
			break
		}
		http.NotFound(w, req)
		return
	}

	if current.handler != nil {
		current.handler(w, req, params)
		return
	}

	http.NotFound(w, req)
}

func compileTemplate(path string) []segment {
	if path == "/" {
		return nil
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	segments := make([]segment, 0, len(parts))
	for _, p := range parts {
		if strings.HasPrefix(p, ":") {
			segments = append(segments, segment{
				raw:       p,
				isParam:   true,
				paramName: p[1:],
			})
		} else if strings.HasPrefix(p, "*") {
			segments = append(segments, segment{
				raw:       p,
				isWild:    true,
				paramName: p[1:],
			})
		} else {
			segments = append(segments, segment{raw: p})
		}
	}
	return segments
}

// --- Helper API ---

func (r *Router) Get(path string, h Handler)    { r.AddRoute(GET, path, h) }
func (r *Router) Post(path string, h Handler)   { r.AddRoute(POST, path, h) }
func (r *Router) Put(path string, h Handler)    { r.AddRoute(PUT, path, h) }
func (r *Router) Delete(path string, h Handler) { r.AddRoute(DELETE, path, h) }
func (r *Router) Patch(path string, h Handler)  { r.AddRoute(PATCH, path, h) }
func (r *Router) Any(path string, h Handler)    { r.AddRoute(ANY, path, h) }

// Resource REST-style routes
func (r *Router) Resource(path string, c ResourceController) {
	path = strings.TrimSuffix(path, "/")

	r.Get(path, c.Index)
	r.Post(path, c.Create)
	r.Get(path+"/:id", c.Show)
	r.Put(path+"/:id", c.Update)
	r.Delete(path+"/:id", c.Delete)
	r.Patch(path+"/:id", c.Patch)
}

func (r *Router) Print(w io.Writer) {
	fmt.Fprintln(w, "Registered Routes:")
	methods := make([]string, 0, len(r.routes))
	for m := range r.routes {
		methods = append(methods, m)
	}
	sort.Strings(methods)
	for _, m := range methods {
		rs := r.routes[m]
		sort.Slice(rs, func(i, j int) bool { return rs[i].Path < rs[j].Path })
		for _, rt := range rs {
			fmt.Fprintf(w, "%-6s %s\n", rt.Method, rt.Path)
		}
	}
}
