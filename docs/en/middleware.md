# Middleware System Documentation

## 1. Overview

Middleware is an important concept in the Go Simple Server framework, used to execute specific logic before and after HTTP request processing. Middleware can be used to implement functions such as logging, authentication and authorization, error handling, and response time measurement.

The framework's middleware system is designed to be concise and flexible, supporting the combination of multiple middleware and the development of custom middleware.

## 2. Middleware Basics

### 2.1 Middleware Type Definition

In the framework, middleware is defined as a function type:

```go
// Middleware defines the middleware function type
type Middleware func(http.HandlerFunc) http.HandlerFunc
```

Middleware receives an `http.HandlerFunc` type function as input and returns a new `http.HandlerFunc` type function. In this way, middleware can execute custom logic before and after calling the original handler function.

### 2.2 Middleware Application Mechanism

The framework provides the `Apply` function for applying middleware:

```go
// Apply applies one or more middleware to a handler function
func Apply(h http.HandlerFunc, m ...Middleware) http.HandlerFunc {
    for i := len(m) - 1; i >= 0; i-- {
        h = m[i](h)
    }

    for _, m := range m {
        h = m(h)
    }

    return h
}
```

Note: There is a potential issue in the code where middleware is applied twice (through two loops). In actual use, this may cause the middleware logic to be executed twice.

## 3. Built-in Middleware

The framework provides two built-in middleware:

### 3.1 Logging Middleware

The logging middleware is used to record HTTP request information, including request method, path, and processing time:

```go
// Logging middleware records request information and processing time
func Logging(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next(w, r) // Call the next handler function
        fmt.Printf("[%s] %s %s (%s)\n", time.Now().Format("15:04:05"), r.Method, r.URL.Path, time.Since(start))
    }
}
```

### 3.2 Authentication Middleware (Auth)

The authentication middleware performs simple identity verification based on tokens. It checks whether the `X-Token` field in the request header matches the environment variable `AUTH_TOKEN`:

```go
// Auth middleware (requires request header: X-Token: secret)
func Auth(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        token := r.Header.Get("X-Token")
        authToken := os.Getenv("AUTH_TOKEN")
        if authToken != "" && token != authToken {
            w.WriteHeader(http.StatusUnauthorized)
            w.Write([]byte(`{"error":"unauthorized"}`))
            return
        }
        next(w, r) // Authentication passed, call the next handler function
    }
}
```

## 4. Applying Middleware

### 4.1 Applying Middleware Through App

When using the framework's `App` component, you can apply middleware through the `Use` method:

```go
app := foundation.New()

// Apply a single middleware
app.Use(app.Logging())

// Apply multiple middleware (Note: The order of middleware is important)
app.Use(app.Logging(), app.Auth())
```

The `App.Use` method applies middleware to all requests processed through the router:

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

### 4.2 Applying Middleware Directly

Without using the `App` component, you can directly use the `middleware.Apply` function to apply middleware:

```go
// Define handler function
originalHandler := func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("Hello, World!"))
}

// Apply middleware
handlerWithMiddleware := middleware.Apply(originalHandler, middleware.Logging, middleware.Auth)

// Register handler function
http.HandleFunc("/", handlerWithMiddleware)
```

## 5. Developing Custom Middleware

### 5.1 Basic Middleware Development

Developing custom middleware is very simple; you just need to implement the corresponding function according to the `Middleware` type definition:

```go
// Custom middleware example: Add response header
func AddResponseHeader(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Before request processing: Add response header
        w.Header().Set("X-Powered-By", "Go Simple Server")
        
        // Call the next handler function
        next(w, r)
        
        // After request processing: You can add subsequent processing logic here
    }
}
```

### 5.2 Middleware with Configuration

For middleware that requires configuration, closure functions can be used:

```go
// Configurable middleware: Basic authentication
func BasicAuth(username, password string) middleware.Middleware {
    return func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            // Get Authorization header
            auth := r.Header.Get("Authorization")
            if auth == "" {
                w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
                w.WriteHeader(http.StatusUnauthorized)
                w.Write([]byte("Unauthorized"))
                return
            }
            
            // Decode and verify credentials
            const prefix = "Basic "
            if !strings.HasPrefix(auth, prefix) {
                w.WriteHeader(http.StatusUnauthorized)
                w.Write([]byte("Invalid authorization format"))
                return
            }
            
            decoded, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
            if err != nil {
                w.WriteHeader(http.StatusUnauthorized)
                w.Write([]byte("Invalid authorization format"))
                return
            }
            
            userPass := string(decoded)
            parts := strings.SplitN(userPass, ":", 2)
            if len(parts) != 2 || parts[0] != username || parts[1] != password {
                w.WriteHeader(http.StatusUnauthorized)
                w.Write([]byte("Invalid credentials"))
                return
            }
            
            // Authentication passed, call the next handler function
            next(w, r)
        }
    }
}

// Use configurable middleware
authMiddleware := BasicAuth("admin", "password123")
app.Use(authMiddleware)
```

## 6. Middleware Chain

Middleware can be combined in a chain to form a processing pipeline. The execution order of middleware follows the Last-In-First-Out (LIFO) principle:

```go
// Apply multiple middleware
app.Use(Middleware1, Middleware2, Middleware3)
```

Execution order:
1. Middleware1's pre-processing logic
2. Middleware2's pre-processing logic
3. Middleware3's pre-processing logic
4. Actual handler function
5. Middleware3's post-processing logic
6. Middleware2's post-processing logic
7. Middleware1's post-processing logic

Practical chain layout when mixing global, group, and per-route middleware:

```go
// Global middleware registered on the App
app.EnableLogging()
app.EnableRecovery()

api := app.Router().Group("/api")

// Group-specific middleware (only runs for /api/* routes)
authz := middleware.FromFuncMiddleware(func(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("X-Trace") == "" {
            http.Error(w, "missing trace id", http.StatusBadRequest)
            return
        }
        next(w, r)
    }
})
api.Use(authz)

// Per-route middleware is chained manually when needed.
metrics := middleware.FromHTTPHandlerMiddleware(func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)
        log.Infof("latency=%s", time.Since(start))
    })
})

api.GetFunc("/orders/:id", middleware.NewChain(metrics).ApplyFunc(func(w http.ResponseWriter, r *http.Request) {
    params := router.ParamsFromContext(r.Context())
    w.Write([]byte("order=" + params["id"]))
}))
```

## 7. Middleware Best Practices

### 7.1 Middleware Design Principles

- **Single Responsibility**: Each middleware is only responsible for one function
- **Composable**: Middleware should be able to be combined with other middleware
- **Stateless**: Try to avoid storing state between requests in middleware
- **Error Handling**: Properly handle errors in middleware to avoid affecting subsequent processing

### 7.2 Common Middleware Use Cases

- **Logging**: Record request and response information
- **Authentication and Authorization**: Verify user identity and permissions
- **CORS Handling**: Set cross-origin resource sharing headers
- **Compression**: Compress response content
- **Caching**: Cache response results
- **Rate Limiting**: Limit request frequency
- **Timeout Handling**: Set request processing timeout
- **Recovery**: Catch and handle panics in handler functions

### 7.3 Middleware Usage Recommendations

- Organize middleware in logical order (e.g., authentication middleware should come before logging middleware)
- Avoid overusing middleware, as each middleware adds overhead to request processing
- For logic that only applies to specific routes, consider implementing it directly in the handler function instead of creating middleware
- Write unit tests for complex middleware

## 8. Middleware Example Collection

### 8.1 CORS Middleware

```go
// CORS middleware handles cross-origin requests
func CORS(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Set CORS headers
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Token")
        
        // Handle OPTIONS requests
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        
        // Call the next handler function
        next(w, r)
    }
}
```

### 8.2 Error Handling Middleware

```go
// ErrorHandler middleware catches and handles errors in handler functions
func ErrorHandler(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Create a response writer wrapper to capture status codes
        rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
        
        // Use defer and recover to catch panics
        defer func() {
            if err := recover(); err != nil {
                // Record error information (should use a logging library in actual applications)
                fmt.Printf("Error: %v\n", err)
                
                // Return 500 error
                w.WriteHeader(http.StatusInternalServerError)
                w.Write([]byte(`{"error":"Internal server error"}`))
            }
        }()
        
        // Call the next handler function
        next(rw, r)
    }
}

// responseWriter is a wrapper for http.ResponseWriter used to capture status codes
type responseWriter struct {
    http.ResponseWriter
    statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
    rw.statusCode = code
    rw.ResponseWriter.WriteHeader(code)
}
```

### 8.3 JSON Middleware

```go
// JSONMiddleware automatically sets Content-Type to application/json
func JSONMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Set Content-Type header
        w.Header().Set("Content-Type", "application/json")
        
        // Call the next handler function
        next(w, r)
    }
}
```

## 9. Summary

Middleware is a powerful tool for building web applications, helping to separate concerns and improve code reusability and maintainability. The Go Simple Server framework provides a concise and flexible middleware system that supports the development and combination of custom middleware.

Through this documentation, you should now understand the basic concepts, usage methods, and best practices of the framework's middleware system. In actual projects, reasonable use of middleware can make your code clearer, more modular, and easier to test.