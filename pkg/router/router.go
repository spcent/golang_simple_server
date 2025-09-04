package router

import (
	"net/http"
	"strings"
	"sync"
)

type RouteRegistrar interface {
	Register(mux *http.ServeMux)
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

// Router is the radix/trie-based HTTP router.
type Router struct {
	root       *node
	mux        *http.ServeMux
	registrars []RouteRegistrar
	paramBuf   sync.Pool // pool for param maps
}

func NewRouter(mux ...*http.ServeMux) *Router {
	if len(mux) == 0 {
		mux = append(mux, http.NewServeMux())
	}
	return &Router{
		mux:  mux[0],
		root: &node{children: make(map[string]*node)},
		paramBuf: sync.Pool{
			New: func() any { return make(map[string]string) },
		},
	}
}

func (r *Router) Register(registrar RouteRegistrar) {
	r.registrars = append(r.registrars, registrar)
}

func (r *Router) Init() {
	for _, registrar := range r.registrars {
		registrar.Register(r.mux)
	}
}

// AddRoute precompiles a route template and inserts it into the trie.
func (r *Router) AddRoute(path string, handler Handler) {
	segments := compileTemplate(path)
	current := r.root
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

	// Assign handler and precompiled template
	current.handler = handler
	current.template = segments
	current.paramKeys = paramKeys
}

// ServeHTTP matches a request path against the trie and executes the handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := strings.Trim(req.URL.Path, "/")
	if path == "" {
		if r.root.handler != nil {
			r.root.handler(w, req, nil)
			return
		}
		http.NotFound(w, req)
		return
	}

	parts := strings.Split(path, "/")
	params := r.paramBuf.Get().(map[string]string)
	for k := range params { // clear old values
		delete(params, k)
	}

	current := r.root
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

// compileTemplate precompiles a route string into segment metadata.
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
