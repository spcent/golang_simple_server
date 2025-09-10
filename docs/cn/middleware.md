# Middleware 中间件系统文档

## 1. 概述

中间件是 Go Simple Server 框架中的一个重要概念，用于在 HTTP 请求处理前后执行特定逻辑。中间件可以用于实现日志记录、认证授权、错误处理、响应时间测量等功能。

框架的中间件系统设计简洁而灵活，支持多个中间件的组合使用，以及自定义中间件的开发。

## 2. 中间件基础

### 2.1 中间件类型定义

在框架中，中间件被定义为一种函数类型：

```go
// Middleware 定义了中间件函数类型
type Middleware func(http.HandlerFunc) http.HandlerFunc
```

中间件接收一个 `http.HandlerFunc` 类型的函数作为输入，并返回一个新的 `http.HandlerFunc` 类型的函数。通过这种方式，中间件可以在调用原始处理函数前后执行自定义逻辑。

### 2.2 中间件应用机制

框架提供了 `Apply` 函数用于应用中间件：

```go
// Apply 将一个或多个中间件应用到处理函数
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

注意：代码中有一个潜在问题，中间件被应用了两次（通过两个循环）。在实际使用中，这可能会导致中间件逻辑被执行两次。

## 3. 内置中间件

框架提供了两个内置的中间件：

### 3.1 日志中间件（Logging）

日志中间件用于记录 HTTP 请求的信息，包括请求方法、路径和处理时间：

```go
// Logging 中间件记录请求信息和处理时间
func Logging(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next(w, r) // 调用下一个处理函数
        fmt.Printf("[%s] %s %s (%s)\n", time.Now().Format("15:04:05"), r.Method, r.URL.Path, time.Since(start))
    }
}
```

### 3.2 认证中间件（Auth）

认证中间件基于令牌（token）进行简单的身份验证。它检查请求头中的 `X-Token` 字段是否与环境变量 `AUTH_TOKEN` 匹配：

```go
// Auth 中间件（需要请求头：X-Token: secret）
func Auth(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        token := r.Header.Get("X-Token")
        authToken := os.Getenv("AUTH_TOKEN")
        if authToken != "" && token != authToken {
            w.WriteHeader(http.StatusUnauthorized)
            w.Write([]byte(`{"error":"unauthorized"}`))
            return
        }
        next(w, r) // 认证通过，调用下一个处理函数
    }
}
```

## 4. 应用中间件

### 4.1 通过 App 应用中间件

在使用框架的 `App` 组件时，可以通过 `Use` 方法应用中间件：

```go
app := foundation.New()

// 应用单个中间件
app.Use(app.Logging())

// 应用多个中间件（注意：中间件的顺序很重要）
app.Use(app.Logging(), app.Auth())
```

`App.Use` 方法会将中间件应用到所有通过路由器处理的请求上：

```go
// Use 将中间件应用到路由器
func (a *App) Use(middlewares ...middleware.Middleware) {
    // 将中间件应用到路由器的 ServeHTTP 方法
    handler := a.router.ServeHTTP
    for i := len(middlewares) - 1; i >= 0; i-- {
        handler = middleware.Apply(handler, middlewares[i])
    }
    a.mux.HandleFunc("/", handler)
}
```

### 4.2 直接应用中间件

在不使用 `App` 组件的情况下，可以直接使用 `middleware.Apply` 函数应用中间件：

```go
// 定义处理函数
originalHandler := func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("Hello, World!"))
}

// 应用中间件
handlerWithMiddleware := middleware.Apply(originalHandler, middleware.Logging, middleware.Auth)

// 注册处理函数
http.HandleFunc("/", handlerWithMiddleware)
```

## 5. 开发自定义中间件

### 5.1 基本中间件开发

开发自定义中间件非常简单，只需按照 `Middleware` 类型定义实现相应的函数即可：

```go
// 自定义中间件示例：添加响应头
func AddResponseHeader(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 请求处理前：添加响应头
        w.Header().Set("X-Powered-By", "Go Simple Server")
        
        // 调用下一个处理函数
        next(w, r)
        
        // 请求处理后：可以在这里添加后续处理逻辑
    }
}
```

### 5.2 带配置的中间件

对于需要配置的中间件，可以使用闭包函数：

```go
// 带配置的中间件：基本认证
func BasicAuth(username, password string) middleware.Middleware {
    return func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            // 获取 Authorization 头
            auth := r.Header.Get("Authorization")
            if auth == "" {
                w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
                w.WriteHeader(http.StatusUnauthorized)
                w.Write([]byte("Unauthorized"))
                return
            }
            
            // 解码并验证凭证
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
            
            // 认证通过，调用下一个处理函数
            next(w, r)
        }
    }
}

// 使用带配置的中间件
authMiddleware := BasicAuth("admin", "password123")
app.Use(authMiddleware)
```

## 6. 中间件链

中间件可以链式组合使用，形成一个处理管道。中间件的执行顺序遵循后进先出（LIFO）原则：

```go
// 应用多个中间件
app.Use(Middleware1, Middleware2, Middleware3)
```

执行顺序：
1. Middleware1 的前置逻辑
2. Middleware2 的前置逻辑
3. Middleware3 的前置逻辑
4. 实际处理函数
5. Middleware3 的后置逻辑
6. Middleware2 的后置逻辑
7. Middleware1 的后置逻辑

## 7. 中间件最佳实践

### 7.1 中间件设计原则

- **单一职责**：每个中间件只负责一项功能
- **可组合**：中间件应该可以与其他中间件组合使用
- **无状态**：尽量避免在中间件中存储请求间的状态
- **错误处理**：适当处理中间件中的错误，避免影响后续处理

### 7.2 常见中间件用例

- **日志记录**：记录请求和响应信息
- **认证授权**：验证用户身份和权限
- **CORS 处理**：设置跨域资源共享头
- **压缩**：压缩响应内容
- **缓存**：缓存响应结果
- **限流**：限制请求频率
- **超时处理**：设置请求处理超时
- **恢复**：捕获并处理处理函数中的 panic

### 7.3 中间件使用建议

- 按照逻辑顺序组织中间件（例如，认证中间件应该在日志中间件之前）
- 避免过度使用中间件，每个中间件都会增加请求处理的开销
- 对于只应用于特定路由的逻辑，考虑直接在处理函数中实现，而不是创建中间件
- 为复杂的中间件编写单元测试

## 8. 中间件示例集

### 8.1 CORS 中间件

```go
// CORS 中间件处理跨域请求
func CORS(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 设置 CORS 头
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Token")
        
        // 处理 OPTIONS 请求
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        
        // 调用下一个处理函数
        next(w, r)
    }
}
```

### 8.2 错误处理中间件

```go
// ErrorHandler 中间件捕获并处理处理函数中的错误
func ErrorHandler(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 创建一个响应写入器包装器来捕获状态码
        rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
        
        // 使用 defer 和 recover 捕获 panic
        defer func() {
            if err := recover(); err != nil {
                // 记录错误信息（实际应用中应使用日志库）
                fmt.Printf("Error: %v\n", err)
                
                // 返回 500 错误
                w.WriteHeader(http.StatusInternalServerError)
                w.Write([]byte(`{"error":"Internal server error"}`))
            }
        }()
        
        // 调用下一个处理函数
        next(rw, r)
    }
}

// responseWriter 是 http.ResponseWriter 的包装器，用于捕获状态码
type responseWriter struct {
    http.ResponseWriter
    statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
    rw.statusCode = code
    rw.ResponseWriter.WriteHeader(code)
}
```

### 8.3 JSON 中间件

```go
// JSONMiddleware 中间件自动设置 Content-Type 为 application/json
func JSONMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 设置 Content-Type 头
        w.Header().Set("Content-Type", "application/json")
        
        // 调用下一个处理函数
        next(w, r)
    }
}
```

## 9. 总结

中间件是构建 Web 应用的强大工具，可以帮助分离关注点，提高代码的可复用性和可维护性。Go Simple Server 框架提供了简洁而灵活的中间件系统，支持自定义中间件的开发和组合使用。

通过本文档的介绍，您应该已经了解了框架中间件系统的基本概念、使用方法和最佳实践。在实际项目中，合理使用中间件可以使您的代码更加清晰、模块化和易于测试。