# Go Simple Server User Documentation

## 1. Framework Overview

Go Simple Server is a lightweight HTTP server framework that provides a concise API for quickly building web applications.

Key Features:
- High-performance routing system based on Trie tree
- Flexible middleware support
- Graceful shutdown functionality
- Environment configuration management
- Route parameter support
- Route grouping functionality

## 2. Project Structure

```
├── main.go              # Application entry point
├── handlers/            # HTTP handlers
│   ├── handler.go       # Route registration
│   ├── health.go        # Health check related routes
│   └── user.go          # User related routes
├── pkg/                 # Framework core packages
│   ├── foundation/      # Application core
│   ├── router/          # Routing system
│   ├── middleware/      # Middleware
│   └── glog/            # Logging system
└── docs/                # Documentation directory
```

## 3. Quick Start

### 3.1 Creating a Basic Application

```go
package main

import (
    "net/http"
    "github.com/spcent/golang_simple_server/pkg/foundation"
)

func main() {
    // Create application instance
    app := foundation.New()
    
    // Register routes
    app.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("pong"))
    })
    
    // Start application
    app.Boot()
}
```

### 3.2 Using the Routing System

```go
package main

import (
    "net/http"
    "github.com/spcent/golang_simple_server/pkg/foundation"
)

func main() {
    app := foundation.New()
    
    // Register basic routes directly on the app
    app.Get("/hello", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
        w.Write([]byte(`{"message":"Hello, World!"}`))
    })

    // Register routes with parameters
    app.Get("/hello/:name", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
        name := params["name"]
        w.Write([]byte(`{"message":"Hello, ` + name + `!"}`))
    })
    
    // Apply middleware
    app.Use(app.Logging(), app.Auth())
    
    // Start application
    app.Boot()
}
```

## 4. Core Components Detailed Explanation

### 4.1 App Application Core

App is the core component of the framework, responsible for initializing and managing the entire HTTP server.

#### 4.1.1 Creating App

```go
// Create with default configuration
app := foundation.New()

// Create with custom configuration using option pattern
app := foundation.New(
    foundation.WithAddr(":9090"),         // Custom listening address
    foundation.WithEnvPath("./config.env"), // Custom environment variable file path
)
```

#### 4.1.2 Main Methods

- `Get/Post/Put/Delete/Patch/Any(path string, handler foundation.Handler)`: Register HTTP method handlers with path parameters
- `Group(prefix string) foundation.RouteRegister`: Create sub-route groups
- `Register(registrars ...foundation.RouteRegistrar)`: Register route registrars without importing the router package
- `Resource(path string, controller foundation.ResourceController)`: Quickly wire RESTful resources
- `HandleFunc(pattern string, handler http.HandlerFunc)`: Register handler function (standard http.HandlerFunc)
- `Handle(pattern string, handler http.Handler)`: Register handler (standard http.Handler)
- `Use(middlewares ...middleware.Middleware)`: Apply middleware
- `Router() *router.Router`: Get framework router (advanced usage)
- `Boot()`: Initialize and start HTTP server
- `Logging() middleware.Middleware`: Get logging middleware
- `Auth() middleware.Middleware`: Get authentication middleware

### 4.2 Router Routing System

Router is the routing component of the framework, implementing high-performance route matching based on Trie tree.

#### 4.2.1 Route Registration Methods

- `Get(path string, handler Handler)`: Register GET request route
- `Post(path string, handler Handler)`: Register POST request route  
- `Put(path string, handler Handler)`: Register PUT request route
- `Delete(path string, handler Handler)`: Register DELETE request route
- `Patch(path string, handler Handler)`: Register PATCH request route
- `Any(path string, handler Handler)`: Register route that matches all HTTP methods
- `Resource(path string, controller ResourceController)`: Register RESTful resource routes
- `Group(prefix string)`: Create route group
- `Register(registrars ...RouteRegistrar)`: Register route registrars

#### 4.2.2 Route Parameters

The framework supports path parameters, identified by the `:` prefix, such as `/users/:id`.

```go
r.Get("/users/:id", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
    id := params["id"]
    w.Write([]byte(`{"user_id":"` + id + `"}`))
})
```

#### 4.2.3 Route Groups

Related routes can be organized through grouping functionality:

```go
api := r.Group("/api")
api.Get("/users", userListHandler)
api.Get("/posts", postListHandler)
// Actual routes are /api/users and /api/posts
```

#### 4.2.4 RESTful Resource Routes

The framework provides convenient RESTful resource route registration methods:

```go
// Define resource controller
type UserController struct{}

func (c *UserController) Index(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // GET /users - List all users
}

func (c *UserController) Create(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // POST /users - Create new user
}

func (c *UserController) Show(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // GET /users/:id - Get single user
}

func (c *UserController) Update(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // PUT /users/:id - Update user
}

func (c *UserController) Delete(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // DELETE /users/:id - Delete user
}

func (c *UserController) Patch(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // PATCH /users/:id - Partially update user
}

// Register resource routes
r.Resource("/users", &UserController{})
```

### 4.3 Middleware System

Middleware is used to execute specific logic before and after request processing, such as logging, authentication, etc.

#### 4.3.1 Built-in Middleware

- `Logging`: Records request information and processing time
- `Auth`: Token-based authentication (checks X-Token request header)

#### 4.3.2 Applying Middleware

```go
app.Use(app.Logging(), app.Auth())
```

#### 4.3.3 Custom Middleware

```go
// Define middleware
func CustomMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Logic before request processing
        next(w, r) // Call next handler
        // Logic after request processing
    }
}

// Apply custom middleware
app.Use(CustomMiddleware)
```

## 5. Handlers

Handlers are responsible for processing specific HTTP requests and returning responses. The framework supports two ways to register handlers:

### 5.1 Directly Registering Handler Functions

```go
r.Get("/hello", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
    w.Write([]byte(`{"message":"Hello, World!"}`))
})
```

### 5.2 Using the RouteRegistrar Interface

```go
// Define handler struct
type UserHandler struct{}

// Implement Register method
func (h *UserHandler) Register(r *router.Router) {
    r.Get("/users", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
        w.Write([]byte("User List"))
    })
    
    r.Post("/users", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
        w.Write([]byte("Create User"))
    })
}

// Register handler
r.Register(&UserHandler{})
```

## 6. Environment Configuration

The framework supports loading environment variables through `.env` files:

### 6.1 Creating .env File

```
APP_DEBUG=true
AUTH_TOKEN=my-secret-token
SERVER_PORT=8080
```

### 6.2 Custom .env File Path

```go
app := foundation.New(foundation.WithEnvPath("./custom.env"))
```

### 6.3 Command Line Parameters

The framework supports overriding configuration through command line parameters:

```
./server -addr=:9090 -env=./config.env
```

## 7. Logging System

The framework integrates a logging system that automatically records application startup, request processing, shutdown, and other information.

## 8. Graceful Shutdown

The framework supports graceful shutdown functionality. When receiving a SIGTERM signal, it waits for active connections to complete processing before exiting.

## 9. Practical Examples

### 9.1 Complete Application Example

```go
package main

import (
    "fmt"
    "net/http"

    "github.com/spcent/golang_simple_server/pkg/foundation"
)

// Define route registrar
type UserHandler struct{}

func (h *UserHandler) Register(r foundation.RouteRegister) {
    r.Get("/users", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
        w.Header().Set("Content-Type", "application/json")
        fmt.Fprintln(w, `{"users": ["user1", "user2", "user3"]}`)
    })

    r.Get("/users/:id", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
        id := params["id"]
        w.Header().Set("Content-Type", "application/json")
        fmt.Fprintf(w, `{"user_id": "%s"}`, id)
    })
}

func main() {
    // Create application instance
    app := foundation.New(
        foundation.WithAddr(":8080"),
    )
    
    // Register handler
    app.Register(&UserHandler{})

    // Register direct route
    app.Get("/", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
        w.Write([]byte("Welcome to Go Simple Server!"))
    })
    
    // Apply middleware
    app.Use(app.Logging())
    
    // Start application
    app.Boot()
}
```

### 9.2 Testing API

After starting the application, you can test the API using curl:

```bash
# Test root path
curl http://localhost:8080/

# Test user list
curl http://localhost:8080/users

# Test single user
curl http://localhost:8080/users/123

# Test authenticated route (if AUTH_TOKEN is set)
curl -H "X-Token: my-secret-token" http://localhost:8080/users
```

## 10. Configuration and Deployment

### 10.1 Building the Application

```bash
go build -o server main.go
```

### 10.2 Running the Application

```bash
# Run with default configuration
./server

# Run with custom port
./server -addr=:9090

# Run with custom environment variable file
./server -env=./config/prod.env
```

### 10.3 Production Environment Recommendations

- Set appropriate AUTH_TOKEN to protect API
- Turn off APP_DEBUG mode
- Use systemd or other process management tools to manage the service
- Configure log rotation

## 11. Summary

Go Simple Server is a lightweight yet fully functional HTTP server framework suitable for rapid development of small to medium-sized web applications and API services. It provides an intuitive API, high-performance routing system, and flexible middleware support, which can meet most web development needs.

Through this documentation, you should now understand the core components and usage methods of the framework and can start building your own web applications.