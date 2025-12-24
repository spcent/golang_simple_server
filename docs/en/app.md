# App Core Documentation

## 1. Overview

App is the core component of the Go Simple Server framework, responsible for initializing and managing the entire HTTP server application. It provides a unified interface for configuring and launching web services, coordinating the work of routing, middleware, and other framework components.

Key Features:
- Application initialization and configuration
- Routing management and registration
- Middleware application
- HTTP server startup and graceful shutdown
- Environment variable management
- Logging system integration

## 2. App Struct

The App struct is the core of the framework application:

```go
// App represents an HTTP server application
type App struct {
    addr    string         // Binding address
    envFile string         // .env file path
    mux     *http.ServeMux // HTTP server multiplexer
    router  *router.Router // HTTP request router
}
```

## 3. Creating an Application Instance

The framework provides the `New` function to create an App instance, supporting the function option pattern for configuration:

```go
// New creates a new App instance
func New(options ...Option) *App {
    app := &App{
        // Set default values
        mux:     http.NewServeMux(),
        router:  router.NewRouter(),
        addr:    ":8080",
        envFile: ".env",
    }

    // Apply all provided options
    for _, opt := range options {
        opt(app)
    }

    return app
}
```

### 3.1 Function Option Pattern

The framework uses the function option pattern to configure App instances, providing the following option functions:

```go
// WithRouter sets the router
func WithRouter(router *router.Router) Option

// WithAddr sets the server address
func WithAddr(address string) Option

// WithEnvPath sets the .env file path
func WithEnvPath(path string) Option
```

### 3.2 Application Creation Example

```go
// Create app with default configuration
app := foundation.New()

// Create app with custom configuration
app := foundation.New(
    foundation.WithAddr(":9090"),           // Custom listening address
    foundation.WithEnvPath("./config.env"), // Custom environment variable file path
)
```

## 4. Routing Management

### 4.1 Direct Route Registration

App provides methods to directly register route handlers, which wrap the standard `http.ServeMux`:

```go
// HandleFunc registers a handler function to a specific path
func (a *App) HandleFunc(pattern string, handler http.HandlerFunc)

// Handle registers a handler to a specific path
func (a *App) Handle(pattern string, handler http.Handler)
```

Example:

```go
app.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("pong"))
})
```

### 4.2 Using the Framework Router

For more complex routing requirements, you can get the framework's router and use its advanced features:

```go
// Router returns the underlying router for advanced configuration
func (a *App) Router() *router.Router
```

Example:

```go
r := app.Router()

// Register routes with parameters
r.Get("/users/:id", func(w http.ResponseWriter, r *http.Request) {
    params := router.ParamsFromContext(r.Context())
    id := params["id"]
    w.Write([]byte(`{"user_id":"` + id + `"}`))
})

// Register route registrars
r.Register(&UserHandler{}, &PostHandler{})
```

## 5. Middleware Application

App provides the `Use` method to apply middleware:

```go
// Use applies middleware to the router
func (a *App) Use(middlewares ...middleware.Middleware) {
    // Apply middleware to the router's ServeHTTP method
    handler := a.router.ServeHTTP
    for i := len(middlewares) - 1; i >= 0; i-- {
        handler = middleware.Apply(handler, middlewares[i])
    }
    a.mux.HandleFunc("/", handler)
}
```

Example:

```go
// Apply a single middleware
app.Use(app.Logging())

// Apply multiple middleware
app.Use(app.Logging(), app.Auth())
```

### 5.1 Getting Built-in Middleware

App provides methods to get built-in middleware:

```go
// Logging returns the logging middleware
func (a *App) Logging() middleware.Middleware

// Auth returns the authentication middleware
func (a *App) Auth() middleware.Middleware
```

## 6. Application Startup

The `Boot` method is the core method for starting the application, which initializes various components and starts the HTTP server:

```go
// Boot initializes and starts the application
func (a *App) Boot() {
    // Set up router and multiplexer
    glog.Init()
    defer glog.Flush()
    defer glog.Close()

    // Load .env file if exists
    if env != nil && *env != "" {
        a.envFile = *env
    }
    if _, err := os.Stat(a.envFile); err == nil {
        glog.Infof("Load .env file: %s", a.envFile)
        err := config.LoadEnv(a.envFile, true)
        if err != nil {
            glog.Fatalf("Load .env failed: %v", err)
        }
    }

    a.router.Init()
    // Middleware is applied via Use() method
    if os.Getenv("APP_DEBUG") == "true" {
        a.router.Print(os.Stdout)
    }

    if addr != nil && *addr != "" {
        a.addr = *addr
    }

    server := &http.Server{
        Addr:    a.addr,
        Handler: a.mux,
    }

    // Gracefully shut down the server when receiving SIGTERM signal
    idleConnsClosed := make(chan struct{})
    go func() {
        sig := make(chan os.Signal, 1)
        signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
        <-sig

        glog.Info("SIGTERM received, shutting down...")
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        if err := server.Shutdown(ctx); err != nil {
            glog.Errorf("Server shutdown error: %v", err)
        }
        close(idleConnsClosed)
    }()

    glog.Infof("Server running on %s", a.addr)
    if err := server.ListenAndServe(); err != http.ErrServerClosed {
        glog.Errorf("Server error: %v", err)
    }

    <-idleConnsClosed
    glog.Info("Server stopped gracefully")
}
```

### 6.1 Graceful Shutdown

The `Boot` method implements graceful shutdown functionality for the server. When receiving `SIGTERM` or `SIGINT` signals, the server will:

1. Stop accepting new connections
2. Wait for active connections to complete processing within the specified timeout period (default 5 seconds)
3. Close all connections and exit

This ensures that ongoing request processing is not interrupted when the application is shut down.

## 7. Environment Variable Management

App supports loading environment variables through `.env` files, which is very useful for configuration management in different environments (development, testing, production).

### 7.1 Loading .env Files

In the `Boot` method, the application checks for and loads the `.env` file if it exists:

```go
// Load .env file if exists
if _, err := os.Stat(a.envFile); err == nil {
    glog.Infof("Load .env file: %s", a.envFile)
    err := config.LoadEnv(a.envFile, true)
    if err != nil {
        glog.Fatalf("Load .env failed: %v", err)
    }
}
```

### 7.2 Command Line Parameters

App supports overriding configuration through command line parameters:

```go
var (
    addr = flag.String("addr", "", "Server address to listen on")
    env  = flag.String("env", "", "Path to .env file")
)
```

These parameters can be specified when starting the application:

```bash
./server -addr=:9090 -env=./config.env
```

## 8. Logging System Integration

App integrates a logging system to record key information during application startup, operation, and shutdown:

```go
// Initialize logging system
glog.Init()
// Ensure logs are flushed and closed when the application exits
defer glog.Flush()
defer glog.Close()

// Record server startup information
glog.Infof("Server running on %s", a.addr)

// Record server shutdown information
glog.Info("SIGTERM received, shutting down...")
// ...
glog.Info("Server stopped gracefully")

// Record error information
glog.Errorf("Server shutdown error: %v", err)
```

## 9. Complete Application Example

Here is an example of building a complete application using the App component:

```go
package main

import (
    "fmt"
    "net/http"
    "github.com/spcent/golang_simple_server/pkg/foundation"
    "github.com/spcent/golang_simple_server/pkg/router"
)

// Define route registrar
type UserHandler struct{}

func (h *UserHandler) Register(r *router.Router) {
    r.Get("/users", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        fmt.Fprintln(w, `{"users": [{"id": 1, "name": "Alice"}, {"id": 2, "name": "Bob"}]}`)
    })
    
    r.Get("/users/:id", func(w http.ResponseWriter, r *http.Request) {
        params := router.ParamsFromContext(r.Context())
        id := params["id"]
        w.Header().Set("Content-Type", "application/json")
        fmt.Fprintf(w, `{"user": {"id": %s, "name": "User %s"}}`, id, id)
    })
}

func main() {
    // Create application instance
    app := foundation.New(
        foundation.WithAddr(":8080"),         // Listen on port 8080
        foundation.WithEnvPath("./.env"),     // Load .env file from current directory
    )
    
    // Get router
    r := app.Router()
    
    // Register route registrar
    r.Register(&UserHandler{})
    
    // Directly register routes
    app.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("pong"))
    })
    
    // Apply middleware
    app.Use(app.Logging())
    
    // Start application
    app.Boot()
}
```

## 10. Practical Usage Patterns

### 10.1 Compose routes and middleware declaratively

The App exposes thin helpers over the Router and middleware chain so you can define routes, groups, and cross-cutting concerns in one place:

```go
app := core.New(
    core.WithAddr(":8081"),
    core.WithEnvPath("./.env.local"),
    core.WithShutdownTimeout(10*time.Second),
)

// Global middlewares are applied in registration order.
app.EnableRecovery()
app.EnableLogging()
app.EnableRateLimit(20, 40)

api := app.Router().Group("/api")
v1 := api.Group("/v1")

// Group-level middleware only wraps handlers registered on that group.
authn := middleware.FromFuncMiddleware(func(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("X-Debug") == "" {
            http.Error(w, "missing debug header", http.StatusUnauthorized)
            return
        }
        next(w, r)
    }
})
v1.Use(authn)

v1.GetFunc("/profile/:id", func(w http.ResponseWriter, r *http.Request) {
    params := router.ParamsFromContext(r.Context())
    fmt.Fprintf(w, "user=%s", params["id"])
})

app.Boot()
```

### 10.2 Boot with TLS and WebSocket enabled

To exercise the full startup path (TLS, environment loading, WebSocket registration, graceful shutdown), configure the app before calling `Boot`:

```go
app := core.New(
    core.WithAddr(":8443"),
    core.WithEnvPath("./.env"),
    core.WithTLS("./cert.pem", "./key.pem"),
    core.WithShutdownTimeout(2*time.Second),
)

// Routes can be registered directly on App
app.Get("/ping", func(w http.ResponseWriter, _ *http.Request) {
    w.Write([]byte("pong"))
})

// WebSocket uses WS_SECRET from the env file via ConfigureWebSocket.
if _, err := app.ConfigureWebSocket(); err != nil {
    log.Fatalf("websocket setup failed: %v", err)
}

// Start the server (blocks until SIGTERM/interrupt triggers graceful shutdown)
if err := app.Boot(); err != nil {
    log.Fatal(err)
}
```

## 11. Best Practices

### 11.1 Application Organization

- Separate routes, handler functions, and middleware into different packages
- Use route registrars to organize related routes
- Keep application initialization code concise in the main function

### 11.2 Configuration Management

- Use `.env` files to manage environment-specific configurations
- Store sensitive information (such as passwords, API keys) in environment variables rather than in code
- Create different `.env` files for different environments (development, testing, production)

### 11.3 Error Handling

- Use middleware for unified error handling
- Use appropriate error logging for key operations
- Provide meaningful error messages to users while avoiding disclosing internal system information

### 11.4 Performance Optimization

- Avoid performing time-consuming operations in middleware
- Use route grouping to reduce duplicate code
- For high-traffic applications, consider using connection pooling and caching

## 12. Summary

The App component is the core of the Go Simple Server framework, providing complete web server lifecycle management functionality. Through this documentation, you should now understand the main features, usage methods, and best practices of App.

In actual projects, properly using the App component can help you quickly build fully functional, high-performance web applications while maintaining code maintainability and scalability.