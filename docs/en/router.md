# Router System Documentation

## 1. Overview

Router is the core routing component of the Go Simple Server framework, implemented based on Trie tree (prefix tree), providing high-performance HTTP route matching functionality.

Key Features:
- Support for RESTful HTTP methods (GET, POST, PUT, DELETE, PATCH)
- Support for path parameters (e.g., `/users/:id`)
- Support for route grouping
- Support for automatic registration of RESTful resource routes
- High-performance route matching algorithm

## 2. Core Types

### 2.1 Handler Type

Handler is an alias to the standard library's `http.Handler` (and `HandlerFunc`), so you register handlers just like you would with `http.ServeMux`:

```go
// Handler defines the function signature for handling HTTP requests
type Handler = http.Handler
type HandlerFunc = http.HandlerFunc
```

The router injects path parameters into `r.Context()`. Use helpers like `Param(r, "id")` or `RequestContextFrom(r.Context())` to read them without custom handler signatures.

### 2.2 RouteRegistrar Interface

RouteRegistrar is the route registrar interface, used to organize and register related routes:

```go
// RouteRegistrar defines the route registrar interface
type RouteRegistrar interface {
    Register(r *Router)
}
```

## 3. Router Struct

Router is the core struct of the routing system:

```go
// Router is a radix/trie-based HTTP router
type Router struct {
    trees      map[string]*node // Each HTTP method corresponds to a radix tree
    registrars []RouteRegistrar // List of route registrars
    routes     map[string][]route // Registered route information (for debugging/printing)
    paramBuf   *sync.Pool // Parameter mapping pool (performance optimization)
    prefix     string     // Prefix for all routes (used for grouping)
    mu         sync.Mutex // Mutex for concurrent safety
}
```

## 4. Route Registration Methods

### 4.1 Basic Route Registration

Router provides a series of methods for registering routes for different HTTP methods:

```go
// Get registers a GET request route
Get(path string, handler Handler)

// Post registers a POST request route
Post(path string, handler Handler)

// Put registers a PUT request route
Put(path string, handler Handler)

// Delete registers a DELETE request route
Delete(path string, handler Handler)

// Patch registers a PATCH request route
Patch(path string, handler Handler)

// Any registers a route that matches all HTTP methods
Any(path string, handler Handler)
```

Example:

```go
router.Get("/hello", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte(`{"message":"Hello, World!"}`))
})

router.Post("/submit", func(w http.ResponseWriter, r *http.Request) {
    // Handle POST request
})
```

### 4.2 Parameter Routes

The framework supports defining parameters in routes, prefixed with `:`:

```go
router.Get("/users/:id", func(w http.ResponseWriter, r *http.Request) {
    params := router.ParamsFromContext(r.Context())
    id := params["id"]
    w.Write([]byte(`{"user_id":"` + id + `"}`))
})

// Support multiple parameters
router.Get("/users/:id/posts/:postId", func(w http.ResponseWriter, r *http.Request) {
    params := router.ParamsFromContext(r.Context())
    id := params["id"]
    postId := params["postId"]
    w.Write([]byte(`{"user_id":"` + id + `", "post_id":"` + postId + `"}`))
})
```

### 4.3 Route Grouping

Route groups can be created through the Group method to add a common prefix to a set of routes:

```go
// Create API route group
api := router.Group("/api")

// Actual route is /api/users
api.Get("/users", userListHandler)

// Actual route is /api/posts
api.Get("/posts", postListHandler)

// Support nested grouping
v1 := api.Group("/v1")

// Actual route is /api/v1/users
v1.Get("/users", userListV1Handler)
```

### 4.4 RESTful Resource Routes

The Resource method provides functionality for quickly registering RESTful resource routes:

```go
// ResourceController defines the resource controller interface
// Note: This is an implicit interface and does not need to be explicitly declared
// Just implement the corresponding methods

type ResourceController interface {
    Index(context.Context, http.ResponseWriter, *http.Request)  // GET    /resources
    Create(context.Context, http.ResponseWriter, *http.Request) // POST   /resources
    Show(context.Context, http.ResponseWriter, *http.Request)   // GET    /resources/:id
    Update(context.Context, http.ResponseWriter, *http.Request) // PUT    /resources/:id
    Delete(context.Context, http.ResponseWriter, *http.Request) // DELETE /resources/:id
    Patch(context.Context, http.ResponseWriter, *http.Request)  // PATCH  /resources/:id
}

// Register resource routes
router.Resource("/users", &UserController{})
```

### 4.5 Route Registrars

By implementing the RouteRegistrar interface, related routes can be organized together:

```go
// Define user route registrar
type UserHandler struct{}

// Implement Register method
func (h *UserHandler) Register(r *router.Router) {
    r.Get("/users", h.List)
    r.Post("/users", h.Create)
    r.Get("/users/:id", h.Get)
    r.Put("/users/:id", h.Update)
    r.Delete("/users/:id", h.Delete)
}

// Implement specific handling methods
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
    // Implement user list logic
}

// Register route registrar
router.Register(&UserHandler{})
```

## 5. Initialization and Route Printing

### 5.1 Initializing Routes

After registering all routes, the Init method needs to be called to complete the router's initialization:

```go
router.Init()
```

Note: When using the framework's App component, the App.Boot() method automatically calls router.Init(), so there's no need to call it manually.

### 5.2 Printing Routes

The Print method can be used to print all registered routes for debugging:

```go
// Print to standard output
router.Print(os.Stdout)

// Print to file
file, _ := os.Create("routes.txt")
router.Print(file)
file.Close()
```

## 6. Route Matching Algorithm

Router uses Radix Trie (prefix tree) to achieve efficient route matching. Radix tree is a compressed form of prefix tree that can reduce storage space and improve lookup efficiency.

### 6.1 Route Compilation

When registering routes, the framework compiles route paths into segments:

```go
// compileTemplate precompiles route strings into segments
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
```

### 6.2 Route Lookup

When receiving an HTTP request, the router looks for a matching handler function based on the request method and path:

```go
// ServeHTTP matches the correct method tree based on the request path
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
    method := req.Method
    tree := r.trees[method]

    // Provide fallback for ANY routes
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

    // Route matching logic...
}
```

## 7. Performance Optimization

Router implements several performance optimization measures:

1. **Radix Trie Algorithm**: Provides O(k) lookup time complexity, where k is the path length
2. **Parameter Mapping Pooling**: Uses sync.Pool to reduce memory allocation and garbage collection pressure
3. **Route Precompilation**: Precompiles route templates during route registration to speed up matching
4. **Concurrent Safety**: Uses mutex to ensure safe operation in concurrent environments

## 8. Best Practices

### 8.1 Route Organization

- Organize related routes in a single route registrar
- Use route grouping for API versioning
- Follow RESTful design principles to organize route structure

### 8.2 Route Naming Conventions

- Use lowercase letters and hyphens (not underscores) to separate words
- For parameterized routes, use meaningful parameter names
- Keep routes concise and clear

### 8.3 Avoiding Common Pitfalls

- Avoid route conflicts (e.g., `/users/new` and `/users/:id`)
- Use ANY routes cautiously, only when necessary
- For complex routing requirements, consider using multiple route registrars rather than one large route registration function

## 9. Examples

### 9.1 Basic Route Example

```go
// Create router
r := router.NewRouter()

// Register basic routes
r.Get("/", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("Welcome!"))
})

// Register routes with parameters
r.Get("/users/:id", func(w http.ResponseWriter, r *http.Request) {
    params := router.ParamsFromContext(r.Context())
    w.Write([]byte(`User ID: ` + params["id"]))
})

// Initialize router
r.Init()

// Create HTTP server
server := &http.Server{
    Addr:    ":8080",
    Handler: r,
}

// Start server
server.ListenAndServe()
```

### 9.2 Advanced Route Example

```go
// Create router
r := router.NewRouter()

// Create route groups
api := r.Group("/api/v1")

// Register user routes
users := api.Group("/users")
users.Get("", listUsers)
users.Post("", createUser)
users.Get("/:id", getUser)
users.Put("/:id", updateUser)
users.Delete("/:id", deleteUser)

// Register article routes
posts := api.Group("/posts")
posts.Get("", listPosts)
posts.Get("/:id", getPost)

// Use route registrars
r.Register(&CommentHandler{}, &TagHandler{})

// Initialize router
r.Init()

// Print all routes
r.Print(os.Stdout)
```

### 9.3 Route grouping with middleware pipelines

Route groups can carry their own middleware stack. The outer router middleware runs first, followed by group middleware, and finally the handler:

```go
r := router.NewRouter()

// Global middleware (applies to all requests)
r.Use(middleware.Logging(log.NewGLogger(), nil, nil))
r.Use(middleware.RateLimit(50, 100, time.Minute, 5*time.Minute))

// API group with auth + audit middleware
api := r.Group("/api")
auth := middleware.FromFuncMiddleware(func(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, req *http.Request) {
        if req.Header.Get("X-Token") == "" {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }
        next(w, req)
    }
})
api.Use(auth, middleware.FromHTTPHandlerMiddleware(middleware.RecoveryMiddleware))

// Nested group inherits parent middleware and adds version-specific logic.
v1 := api.Group("/v1")
v1.GetFunc("/orders/:id", func(w http.ResponseWriter, req *http.Request) {
    params := router.ParamsFromContext(req.Context())
    w.Write([]byte("order=" + params["id"]))
})

r.Init()
```

## 10. Summary

The Router component is a core part of the Go Simple Server framework, providing efficient and flexible routing functionality. Through this documentation, you should now understand the main features and usage methods of the router. In actual projects, reasonably organizing and using routes can make your code clearer and more maintainable.