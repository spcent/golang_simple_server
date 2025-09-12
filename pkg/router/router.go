package router

import (
	"fmt"
	"io"
	"net/http"
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

// segment represents a single precompiled route segment.
type segment struct {
	raw       string // original segment text
	isParam   bool   // true if this segment is a parameter
	paramName string // parameter name (without ":")
}

// node represents a single node in the radix trie.
type node struct {
	segment    string           // path segment for this node
	handler    Handler          // associated handler, if any
	children   map[string]*node // child nodes for exact matches
	paramChild *node            // child node for parameterized routes (e.g. ":id")
	paramName  string           // name of the parameter (for paramNode only)
	template   []segment        // precompiled segments if this is a terminal route
	paramKeys  []string         // list of parameter names for this route
}

// Route holds route metadata for debugging/printing
type route struct {
	Method string
	Path   string
}

// Router is the radix/trie-based HTTP router.
type Router struct {
	trees      map[string]*node // one radix tree per HTTP method
	registrars []RouteRegistrar
	routes     map[string][]route
	paramBuf   *sync.Pool // pool for param maps
	prefix     string     // prefix for all routes
	mu         sync.Mutex
}

func NewRouter() *Router {
	return &Router{
		trees:      make(map[string]*node),
		registrars: []RouteRegistrar{},
		routes:     make(map[string][]route),
		paramBuf: &sync.Pool{
			New: func() any { return make(map[string]string) },
		},
	}
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

// Group creates a sub-group with a prefix
func (r *Router) Group(prefix string) *Router {
	return &Router{
		trees:    r.trees,
		routes:   r.routes,
		paramBuf: r.paramBuf,
		prefix:   r.prefix + strings.TrimRight(prefix, "/"),
	}
}

// AddRoute precompiles a route and inserts it into the corresponding method tree
func (r *Router) AddRoute(method, path string, handler Handler) {
	fullPath := r.prefix + path
	if r.trees[method] == nil {
		r.trees[method] = &node{children: make(map[string]*node)}
	}
	segments := compileTemplate(fullPath)
	current := r.trees[method]
	var paramKeys []string

	for _, seg := range segments {
		if seg.isParam {
			paramKeys = append(paramKeys, seg.paramName)
			if current.paramChild == nil {
				current.paramChild = &node{
					segment:   seg.raw,
					children:  make(map[string]*node),
					paramName: seg.paramName,
				}
			}
			current = current.paramChild
			continue
		}
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

	current.handler = handler
	current.template = segments
	current.paramKeys = paramKeys

	// Save for printing/debugging
	r.routes[method] = append(r.routes[method], route{Method: method, Path: fullPath})
}

// ServeHTTP matches a request path against the correct method trie
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	method := req.Method
	tree := r.trees[method]

	// fallback for ANY routes
	if tree == nil {
		tree = r.trees[ANY]
	}

	if tree == nil {
		http.NotFound(w, req)
		return
	}

	path := strings.Trim(req.URL.Path, "/")
	if path == "" {
		if tree.handler != nil {
			tree.handler(w, req, nil)
			return
		}
		http.NotFound(w, req)
		return
	}

	parts := strings.Split(path, "/")
	params := r.paramBuf.Get().(map[string]string)
	for k := range params {
		delete(params, k)
	}

	current := tree
	for _, part := range parts {
		if child, ok := current.children[part]; ok {
			current = child
			continue
		}
		if current.paramChild != nil {
			current = current.paramChild
			params[current.paramName] = part
			continue
		}
		http.NotFound(w, req)
		r.paramBuf.Put(params)
		return
	}

	if current.handler != nil {
		current.handler(w, req, params)
		r.paramBuf.Put(params)
		return
	}

	http.NotFound(w, req)
	r.paramBuf.Put(params)
}

// compileTemplate precompiles a route string into segments
func compileTemplate(path string) []segment {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	segments := make([]segment, 0, len(parts))
	for _, p := range parts {
		if strings.HasPrefix(p, ":") {
			segments = append(segments, segment{
				raw:       p,
				isParam:   true,
				paramName: p[1:],
			})
		} else {
			segments = append(segments, segment{
				raw:     p,
				isParam: false,
			})
		}
	}
	return segments
}

// --- Helper API for business usage ---

func (r *Router) Get(path string, handler Handler)    { r.AddRoute(GET, path, handler) }
func (r *Router) Post(path string, handler Handler)   { r.AddRoute(POST, path, handler) }
func (r *Router) Put(path string, handler Handler)    { r.AddRoute(PUT, path, handler) }
func (r *Router) Delete(path string, handler Handler) { r.AddRoute(DELETE, path, handler) }
func (r *Router) Patch(path string, handler Handler)  { r.AddRoute(PATCH, path, handler) }
func (r *Router) Any(path string, handler Handler)    { r.AddRoute(ANY, path, handler) }
func (r *Router) Resource(path string, controller ResourceController) {
	path = strings.TrimSuffix(path, "/")

	r.Get(path, controller.Index)
	r.Post(path, controller.Create)

	r.Get(path+"/:id", controller.Show)
	r.Put(path+"/:id", controller.Update)
	r.Delete(path+"/:id", controller.Delete)
	r.Patch(path+"/:id", controller.Patch)
}

// PrintRoutes shows all registered routes grouped by method
func (r *Router) Print(w io.Writer) {
	fmt.Fprintln(w, "Registered Routes:")
	for method, methodRoutes := range r.routes {
		for _, route := range methodRoutes {
			fmt.Fprintf(w, "%-6s %s\n", method, route.Path)
		}
	}
}
