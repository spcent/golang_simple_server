# App 应用核心文档

## 1. 概述

App 是 Go Simple Server 框架的核心组件，负责初始化和管理整个 HTTP 服务器应用程序。它提供了统一的接口来配置和启动 Web 服务，协调路由、中间件和其他框架组件的工作。

主要功能：
- 应用程序初始化和配置
- 路由管理和注册
- 中间件应用
- HTTP 服务器启动和优雅关闭
- 环境变量管理
- 日志系统集成

## 2. App 结构体

App 结构体是框架应用的核心：

```go
// App 表示一个 HTTP 服务器应用程序
type App struct {
    addr    string         // 绑定地址
    envFile string         // .env 文件路径
    mux     *http.ServeMux // HTTP 服务器多路复用器
    router  *router.Router // HTTP 请求路由器
}
```

## 3. 创建应用实例

框架提供了 `New` 函数来创建 App 实例，支持函数选项模式进行配置：

```go
// New 创建一个新的 App 实例
func New(options ...Option) *App {
    app := &App{
        // 设置默认值
        mux:     http.NewServeMux(),
        router:  router.NewRouter(),
        addr:    ":8080",
        envFile: ".env",
    }

    // 应用所有提供的选项
    for _, opt := range options {
        opt(app)
    }

    return app
}
```

### 3.1 函数选项模式

框架使用函数选项模式来配置 App 实例，提供了以下选项函数：

```go
// WithRouter 设置路由器
func WithRouter(router *router.Router) Option

// WithAddr 设置服务器地址
func WithAddr(address string) Option

// WithEnvPath 设置 .env 文件路径
func WithEnvPath(path string) Option
```

### 3.2 创建应用示例

```go
// 使用默认配置创建应用
app := foundation.New()

// 使用自定义配置创建应用
app := foundation.New(
    foundation.WithAddr(":9090"),           // 自定义监听地址
    foundation.WithEnvPath("./config.env"), // 自定义环境变量文件路径
)
```

## 4. 路由管理

### 4.1 直接注册路由

App 提供了方法直接注册路由处理函数，这是对标准 `http.ServeMux` 的包装：

```go
// HandleFunc 注册处理函数到指定路径
func (a *App) HandleFunc(pattern string, handler http.HandlerFunc)

// Handle 注册处理器到指定路径
func (a *App) Handle(pattern string, handler http.Handler)
```

示例：

```go
app.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("pong"))
})
```

### 4.2 使用框架路由器

对于更复杂的路由需求，可以获取框架的路由器并使用其高级功能：

```go
// Router 返回底层路由器用于高级配置
func (a *App) Router() *router.Router
```

示例：

```go
r := app.Router()

// 注册带参数的路由
r.Get("/users/:id", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
    id := params["id"]
    w.Write([]byte(`{"user_id":"` + id + `"}`))
})

// 注册路由注册器
r.Register(&UserHandler{}, &PostHandler{})
```

## 5. 中间件应用

App 提供了 `Use` 方法来应用中间件：

```go
// Use 应用中间件到路由器
func (a *App) Use(middlewares ...middleware.Middleware) {
    // 将中间件应用到路由器的 ServeHTTP 方法
    handler := a.router.ServeHTTP
    for i := len(middlewares) - 1; i >= 0; i-- {
        handler = middleware.Apply(handler, middlewares[i])
    }
    a.mux.HandleFunc("/", handler)
}
```

示例：

```go
// 应用单个中间件
app.Use(app.Logging())

// 应用多个中间件
app.Use(app.Logging(), app.Auth())
```

### 5.1 获取内置中间件

App 提供了方法来获取内置中间件：

```go
// Logging 返回日志中间件
func (a *App) Logging() middleware.Middleware

// Auth 返回认证中间件
func (a *App) Auth() middleware.Middleware
```

## 6. 应用启动

`Boot` 方法是启动应用的核心方法，它初始化各个组件并启动 HTTP 服务器：

```go
// Boot 初始化并启动应用程序
func (a *App) Boot() {
    // 设置路由器与多路复用器
    glog.Init()
    defer glog.Flush()
    defer glog.Close()

    // 加载 .env 文件（如果存在）
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
    // 中间件通过 Use() 方法应用
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

    // 当收到 SIGTERM 信号时优雅关闭服务器
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

### 6.1 优雅关闭

`Boot` 方法实现了服务器的优雅关闭功能，当收到 `SIGTERM` 或 `SIGINT` 信号时，服务器会：

1. 停止接受新的连接
2. 在指定的超时时间内（默认为 5 秒）等待活跃连接处理完成
3. 关闭所有连接并退出

这确保了在应用程序关闭时不会中断正在进行的请求处理。

## 7. 环境变量管理

App 支持通过 `.env` 文件加载环境变量，这对于不同环境（开发、测试、生产）的配置管理非常有用。

### 7.1 加载 .env 文件

在 `Boot` 方法中，应用程序会检查并加载 `.env` 文件（如果存在）：

```go
// 加载 .env 文件（如果存在）
if _, err := os.Stat(a.envFile); err == nil {
    glog.Infof("Load .env file: %s", a.envFile)
    err := config.LoadEnv(a.envFile, true)
    if err != nil {
        glog.Fatalf("Load .env failed: %v", err)
    }
}
```

### 7.2 命令行参数

App 支持通过命令行参数覆盖配置：

```go
var (
    addr = flag.String("addr", "", "Server address to listen on")
    env  = flag.String("env", "", "Path to .env file")
)
```

这些参数可以在启动应用时指定：

```bash
./server -addr=:9090 -env=./config.env
```

## 8. 日志系统集成

App 集成了日志系统，在应用启动、运行和关闭过程中记录关键信息：

```go
// 初始化日志系统
glog.Init()
// 确保在应用退出时刷新和关闭日志
defer glog.Flush()
defer glog.Close()

// 记录服务器启动信息
glog.Infof("Server running on %s", a.addr)

// 记录服务器关闭信息
glog.Info("SIGTERM received, shutting down...")
// ...
glog.Info("Server stopped gracefully")

// 记录错误信息
glog.Errorf("Server shutdown error: %v", err)
```

## 9. 完整应用示例

以下是一个使用 App 组件构建完整应用的示例：

```go
package main

import (
    "fmt"
    "net/http"
    "github.com/spcent/golang_simple_server/pkg/foundation"
    "github.com/spcent/golang_simple_server/pkg/router"
)

// 定义路由注册器
type UserHandler struct{}

func (h *UserHandler) Register(r *router.Router) {
    r.Get("/users", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
        w.Header().Set("Content-Type", "application/json")
        fmt.Fprintln(w, `{"users": [{"id": 1, "name": "Alice"}, {"id": 2, "name": "Bob"}]}`)
    })
    
    r.Get("/users/:id", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
        id := params["id"]
        w.Header().Set("Content-Type", "application/json")
        fmt.Fprintf(w, `{"user": {"id": %s, "name": "User %s"}}`, id, id)
    })
}

func main() {
    // 创建应用实例
    app := foundation.New(
        foundation.WithAddr(":8080"),         // 监听 8080 端口
        foundation.WithEnvPath("./.env"),     // 从当前目录加载 .env 文件
    )
    
    // 获取路由器
    r := app.Router()
    
    // 注册路由注册器
    r.Register(&UserHandler{})
    
    // 直接注册路由
    app.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("pong"))
    })
    
    // 应用中间件
    app.Use(app.Logging())
    
    // 启动应用
    app.Boot()
}
```

## 10. 最佳实践

### 10.1 应用组织

- 将路由、处理函数和中间件分离到不同的包中
- 使用路由注册器组织相关路由
- 在 main 函数中保持应用初始化代码简洁

### 10.2 配置管理

- 使用 `.env` 文件管理环境特定的配置
- 将敏感信息（如密码、API 密钥）存储在环境变量中，而不是代码中
- 为不同环境（开发、测试、生产）创建不同的 `.env` 文件

### 10.3 错误处理

- 使用中间件统一处理错误
- 在关键操作中使用适当的错误日志记录
- 为用户提供有意义的错误消息，同时避免泄露内部系统信息

### 10.4 性能优化

- 避免在中间件中执行耗时操作
- 使用路由分组减少重复代码
- 对于高流量应用，考虑使用连接池和缓存

## 11. 总结

App 组件是 Go Simple Server 框架的核心，提供了完整的 Web 服务器生命周期管理功能。通过本文档的介绍，您应该已经了解了 App 的主要功能、使用方法和最佳实践。

在实际项目中，合理使用 App 组件可以帮助您快速构建功能完善、性能优良的 Web 应用程序，同时保持代码的可维护性和可扩展性。