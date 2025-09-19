# Go Simple Server 使用文档

## 1. 框架概述

Go Simple Server 是一个轻量级的 HTTP 服务器框架，提供了简洁的 API 用于快速构建 Web 应用程序。

主要特点：
- 基于 Trie 树的高性能路由系统
- 灵活的中间件支持
- 优雅关闭功能
- 环境配置管理
- 路由参数支持
- 路由分组功能

## 2. 项目结构

```
├── main.go              # 应用程序入口
├── handlers/            # HTTP 处理器
│   ├── handler.go       # 路由注册
│   ├── health.go        # 健康检查相关路由
│   └── user.go          # 用户相关路由
├── pkg/                 # 框架核心包
│   ├── foundation/      # 应用程序核心
│   ├── router/          # 路由系统
│   ├── middleware/      # 中间件
│   └── glog/            # 日志系统
└── docs/                # 文档目录
```

## 3. 快速开始

### 3.1 创建一个基本应用

```go
package main

import (
    "net/http"
    "github.com/spcent/golang_simple_server/pkg/foundation"
)

func main() {
    // 创建应用实例
    app := foundation.New()
    
    // 注册路由
    app.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("pong"))
    })
    
    // 启动应用
    app.Boot()
}
```

### 3.2 使用路由系统

```go
package main

import (
    "net/http"
    "github.com/spcent/golang_simple_server/pkg/foundation"
)

func main() {
    app := foundation.New()
    
    // 直接在 App 上注册基础路由
    app.Get("/hello", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
        w.Write([]byte(`{"message":"Hello, World!"}`))
    })

    // 注册带参数的路由
    app.Get("/hello/:name", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
        name := params["name"]
        w.Write([]byte(`{"message":"Hello, ` + name + `!"}`))
    })
    
    // 应用中间件
    app.Use(app.Logging(), app.Auth())
    
    // 启动应用
    app.Boot()
}
```

## 4. 核心组件详解

### 4.1 App 应用核心

App 是框架的核心组件，负责初始化和管理整个 HTTP 服务器。

#### 4.1.1 创建 App

```go
// 使用默认配置创建
app := foundation.New()

// 使用选项模式自定义配置
app := foundation.New(
    foundation.WithAddr(":9090"),         // 自定义监听地址
    foundation.WithEnvPath("./config.env"), // 自定义环境变量文件路径
)
```

#### 4.1.2 主要方法

- `Get/Post/Put/Delete/Patch/Any(path string, handler foundation.Handler)`: 注册支持路径参数的 HTTP 方法处理器
- `Group(prefix string) foundation.RouteRegister`: 创建子路由组
- `Register(registrars ...foundation.RouteRegistrar)`: 注册路由注册器，无需引入 router 包
- `Resource(path string, controller foundation.ResourceController)`: 快速绑定 RESTful 资源
- `HandleFunc(pattern string, handler http.HandlerFunc)`: 注册处理函数（标准 http.HandlerFunc）
- `Handle(pattern string, handler http.Handler)`: 注册处理器（标准 http.Handler）
- `Use(middlewares ...middleware.Middleware)`: 应用中间件
- `Router() *router.Router`: 获取框架路由器（高级用法）
- `Boot()`: 初始化并启动 HTTP 服务器
- `Logging() middleware.Middleware`: 获取日志中间件
- `Auth() middleware.Middleware`: 获取认证中间件

### 4.2 Router 路由系统

Router 是框架的路由组件，基于 Trie 树实现高性能的路由匹配。

#### 4.2.1 路由注册方法

- `Get(path string, handler Handler)`: 注册 GET 请求路由
- `Post(path string, handler Handler)`: 注册 POST 请求路由  
- `Put(path string, handler Handler)`: 注册 PUT 请求路由
- `Delete(path string, handler Handler)`: 注册 DELETE 请求路由
- `Patch(path string, handler Handler)`: 注册 PATCH 请求路由
- `Any(path string, handler Handler)`: 注册匹配所有 HTTP 方法的路由
- `Resource(path string, controller ResourceController)`: 注册 RESTful 资源路由
- `Group(prefix string)`: 创建路由分组
- `Register(registrars ...RouteRegistrar)`: 注册路由注册器

#### 4.2.2 路由参数

框架支持路径参数，以 `:` 前缀标识，如 `/users/:id`。

```go
r.Get("/users/:id", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
    id := params["id"]
    w.Write([]byte(`{"user_id":"` + id + `"}`))
})
```

#### 4.2.3 路由分组

可以通过分组功能组织相关路由：

```go
api := r.Group("/api")
api.Get("/users", userListHandler)
api.Get("/posts", postListHandler)
// 实际路由为 /api/users 和 /api/posts
```

#### 4.2.4 RESTful 资源路由

框架提供了便捷的 RESTful 资源路由注册方法：

```go
// 定义资源控制器
type UserController struct{}

func (c *UserController) Index(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // GET /users - 列出所有用户
}

func (c *UserController) Create(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // POST /users - 创建新用户
}

func (c *UserController) Show(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // GET /users/:id - 获取单个用户
}

func (c *UserController) Update(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // PUT /users/:id - 更新用户
}

func (c *UserController) Delete(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // DELETE /users/:id - 删除用户
}

func (c *UserController) Patch(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // PATCH /users/:id - 部分更新用户
}

// 注册资源路由
r.Resource("/users", &UserController{})
```

### 4.3 Middleware 中间件系统

中间件用于在请求处理前后执行特定逻辑，如日志记录、认证等。

#### 4.3.1 内置中间件

- `Logging`: 记录请求信息和处理时间
- `Auth`: 基于令牌的认证（检查 X-Token 请求头）

#### 4.3.2 应用中间件

```go
app.Use(app.Logging(), app.Auth())
```

#### 4.3.3 自定义中间件

```go
// 定义中间件
func CustomMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 请求处理前的逻辑
        next(w, r) // 调用下一个处理器
        // 请求处理后的逻辑
    }
}

// 应用自定义中间件
app.Use(CustomMiddleware)
```

## 5. 处理器（Handlers）

处理器负责处理具体的 HTTP 请求并返回响应。框架支持两种注册处理器的方式：

### 5.1 直接注册处理函数

```go
r.Get("/hello", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
    w.Write([]byte(`{"message":"Hello, World!"}`))
})
```

### 5.2 使用 RouteRegistrar 接口

```go
// 定义处理器结构体
type UserHandler struct{}

// 实现 Register 方法
func (h *UserHandler) Register(r *router.Router) {
    r.Get("/users", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
        w.Write([]byte("User List"))
    })
    
    r.Post("/users", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
        w.Write([]byte("Create User"))
    })
}

// 注册处理器
r.Register(&UserHandler{})
```

## 6. 环境配置

框架支持通过 `.env` 文件加载环境变量：

### 6.1 创建 .env 文件

```
APP_DEBUG=true
AUTH_TOKEN=my-secret-token
SERVER_PORT=8080
```

### 6.2 自定义 .env 文件路径

```go
app := foundation.New(foundation.WithEnvPath("./custom.env"))
```

### 6.3 命令行参数

框架支持通过命令行参数覆盖配置：

```
./server -addr=:9090 -env=./config.env
```

## 7. 日志系统

框架集成了日志系统，自动记录应用启动、请求处理和关闭等信息。

## 8. 优雅关闭

框架支持优雅关闭功能，当收到 SIGTERM 信号时，会等待活跃连接处理完成后再退出。

## 9. 实战示例

### 9.1 完整应用示例

```go
package main

import (
    "fmt"
    "net/http"

    "github.com/spcent/golang_simple_server/pkg/foundation"
)

// 定义路由注册器
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
    // 创建应用实例
    app := foundation.New(
        foundation.WithAddr(":8080"),
    )
    
    // 注册处理器
    app.Register(&UserHandler{})

    // 注册直接路由
    app.Get("/", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
        w.Write([]byte("Welcome to Go Simple Server!"))
    })
    
    // 应用中间件
    app.Use(app.Logging())
    
    // 启动应用
    app.Boot()
}
```

### 9.2 测试 API

启动应用后，可以使用 curl 测试 API：

```bash
# 测试根路径
curl http://localhost:8080/

# 测试用户列表
curl http://localhost:8080/users

# 测试单个用户
curl http://localhost:8080/users/123

# 测试带认证的路由（如果设置了 AUTH_TOKEN）
curl -H "X-Token: my-secret-token" http://localhost:8080/users
```

## 10. 配置与部署

### 10.1 构建应用

```bash
go build -o server main.go
```

### 10.2 运行应用

```bash
# 使用默认配置运行
./server

# 使用自定义端口
./server -addr=:9090

# 使用自定义环境变量文件
./server -env=./config/prod.env
```

### 10.3 生产环境建议

- 设置适当的 AUTH_TOKEN 保护 API
- 关闭 APP_DEBUG 模式
- 使用 systemd 或其他进程管理工具管理服务
- 配置日志轮转

## 11. 总结

Go Simple Server 是一个轻量级但功能完善的 HTTP 服务器框架，适合快速开发小型到中型的 Web 应用程序和 API 服务。它提供了直观的 API、高性能的路由系统和灵活的中间件支持，可以满足大多数 Web 开发需求。

通过本文档的介绍，您应该已经了解了框架的核心组件和使用方法，可以开始构建自己的 Web 应用了。