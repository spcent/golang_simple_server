# Router 路由系统文档

## 1. 概述

Router 是 Go Simple Server 框架的核心路由组件，基于 Trie 树（前缀树）实现，提供高性能的 HTTP 路由匹配功能。

主要功能：
- 支持 RESTful HTTP 方法（GET、POST、PUT、DELETE、PATCH）
- 支持路径参数（如 `/users/:id`）
- 支持路由分组
- 支持 RESTful 资源路由自动注册
- 高性能路由匹配算法

## 2. 核心类型

### 2.1 Handler 类型

Handler 是路由处理函数的类型定义：

```go
// Handler 定义了处理 HTTP 请求的函数签名
type Handler = http.Handler
type HandlerFunc = http.HandlerFunc
```

处理函数签名与标准库保持一致，直接从 `r.Context()` 读取路由参数：可通过 `Param(r, "id")` 或 `RequestContextFrom(r.Context())` 获取。

### 2.2 RouteRegistrar 接口

RouteRegistrar 是路由注册器接口，用于组织和注册相关路由：

```go
// RouteRegistrar 定义了路由注册器接口
type RouteRegistrar interface {
    Register(r *Router)
}
```

## 3. Router 结构体

Router 是路由系统的核心结构体：

```go
// Router 是基于 radix/trie 的 HTTP 路由器
type Router struct {
    trees      map[string]*node // 每个 HTTP 方法对应一棵 radix 树
    registrars []RouteRegistrar // 路由注册器列表
    routes     map[string][]route // 已注册路由信息（用于调试/打印）
    paramBuf   *sync.Pool // 参数映射池（性能优化）
    prefix     string     // 所有路由的前缀（用于分组）
    mu         sync.Mutex // 用于并发安全的互斥锁
}
```

## 4. 路由注册方法

### 4.1 基本路由注册

Router 提供了一系列方法用于注册不同 HTTP 方法的路由：

```go
// Get 注册 GET 请求路由
Get(path string, handler Handler)

// Post 注册 POST 请求路由
Post(path string, handler Handler)

// Put 注册 PUT 请求路由
Put(path string, handler Handler)

// Delete 注册 DELETE 请求路由
Delete(path string, handler Handler)

// Patch 注册 PATCH 请求路由
Patch(path string, handler Handler)

// Any 注册匹配所有 HTTP 方法的路由
Any(path string, handler Handler)
```

示例：

```go
router.Get("/hello", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
    w.Write([]byte(`{"message":"Hello, World!"}`))
})

router.Post("/submit", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // 处理 POST 请求
})
```

### 4.2 参数路由

框架支持在路由中定义参数，以 `:` 为前缀：

```go
router.Get("/users/:id", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
    id := params["id"]
    w.Write([]byte(`{"user_id":"` + id + `"}`))
})

// 支持多个参数
router.Get("/users/:id/posts/:postId", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
    id := params["id"]
    postId := params["postId"]
    w.Write([]byte(`{"user_id":"` + id + `", "post_id":"` + postId + `"}`))
})
```

### 4.3 路由分组

通过 Group 方法可以创建路由分组，为一组路由添加共同的前缀：

```go
// 创建 API 路由分组
api := router.Group("/api")

// 实际路由为 /api/users
api.Get("/users", userListHandler)

// 实际路由为 /api/posts
api.Get("/posts", postListHandler)

// 支持嵌套分组
v1 := api.Group("/v1")

// 实际路由为 /api/v1/users
v1.Get("/users", userListV1Handler)
```

### 4.4 RESTful 资源路由

Resource 方法提供了快速注册 RESTful 资源路由的功能：

```go
// ResourceController 定义了资源控制器接口
// 注意：这是一个隐式接口，不需要显式声明实现
// 只需实现相应的方法即可
type ResourceController interface {
    Index(context.Context, http.ResponseWriter, *http.Request)  // GET    /resources
    Create(context.Context, http.ResponseWriter, *http.Request) // POST   /resources
    Show(context.Context, http.ResponseWriter, *http.Request)   // GET    /resources/:id
    Update(context.Context, http.ResponseWriter, *http.Request) // PUT    /resources/:id
    Delete(context.Context, http.ResponseWriter, *http.Request) // DELETE /resources/:id
    Patch(context.Context, http.ResponseWriter, *http.Request)  // PATCH  /resources/:id
}

// 注册资源路由
router.Resource("/users", &UserController{})
```

### 4.5 路由注册器

通过实现 RouteRegistrar 接口，可以将相关路由组织在一起：

```go
// 定义用户路由注册器
type UserHandler struct{}

// 实现 Register 方法
func (h *UserHandler) Register(r *router.Router) {
    r.Get("/users", h.List)
    r.Post("/users", h.Create)
    r.Get("/users/:id", h.Get)
    r.Put("/users/:id", h.Update)
    r.Delete("/users/:id", h.Delete)
}

// 实现具体的处理方法
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // 实现用户列表逻辑
}

// 注册路由注册器
router.Register(&UserHandler{})
```

## 5. 初始化和路由打印

### 5.1 初始化路由

在注册完所有路由后，需要调用 Init 方法完成路由器的初始化：

```go
router.Init()
```

注意：在使用框架的 App 组件时，App.Boot() 方法会自动调用 router.Init()，无需手动调用。

### 5.2 打印路由

可以使用 Print 方法打印所有已注册的路由，便于调试：

```go
// 打印到标准输出
router.Print(os.Stdout)

// 打印到文件
file, _ := os.Create("routes.txt")
router.Print(file)
file.Close()
```

## 6. 路由匹配算法

Router 使用 Radix Trie（基数树）实现高效的路由匹配。基数树是一种前缀树的压缩形式，可以减少存储空间并提高查找效率。

### 6.1 路由编译

当注册路由时，框架会将路由路径编译成 segments（段）：

```go
// compileTemplate 将路由字符串预编译为段
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

### 6.2 路由查找

当收到 HTTP 请求时，路由器会根据请求方法和路径查找匹配的处理函数：

```go
// ServeHTTP 根据请求路径匹配正确的方法树
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
    method := req.Method
    tree := r.trees[method]

    // 为 ANY 路由提供回退
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

    // 路由匹配逻辑...
}
```

## 7. 性能优化

Router 实现了多项性能优化措施：

1. **Radix Trie 算法**：提供 O(k) 的查找时间复杂度，其中 k 是路径长度
2. **参数映射池化**：使用 sync.Pool 减少内存分配和垃圾回收压力
3. **路由预编译**：在路由注册时预编译路由模板，加快匹配速度
4. **并发安全**：使用互斥锁确保并发环境下的安全操作

## 8. 最佳实践

### 8.1 路由组织

- 将相关路由组织在一个路由注册器中
- 使用路由分组为 API 版本化
- 遵循 RESTful 设计原则组织路由结构

### 8.2 路由命名约定

- 使用小写字母和连字符（而非下划线）分隔单词
- 对于参数化路由，使用有意义的参数名称
- 保持路由简洁明了

### 8.3 避免常见陷阱

- 避免路由冲突（例如 `/users/new` 和 `/users/:id`）
- 谨慎使用 ANY 路由，仅在必要时使用
- 对于复杂的路由需求，考虑使用多个路由注册器而非一个大的路由注册函数

## 9. 示例

### 9.1 基本路由示例

```go
// 创建路由器
r := router.NewRouter()

// 注册基本路由
r.Get("/", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
    w.Write([]byte("Welcome!"))
})

// 注册带参数的路由
r.Get("/users/:id", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
    w.Write([]byte(`User ID: ` + params["id"]))
})

// 初始化路由器
r.Init()

// 创建 HTTP 服务器
server := &http.Server{
    Addr:    ":8080",
    Handler: r,
}

// 启动服务器
server.ListenAndServe()
```

### 9.2 高级路由示例

```go
// 创建路由器
r := router.NewRouter()

// 创建路由分组
api := r.Group("/api/v1")

// 注册用户路由
users := api.Group("/users")
users.Get("", listUsers)
users.Post("", createUser)
users.Get("/:id", getUser)
users.Put("/:id", updateUser)
users.Delete("/:id", deleteUser)

// 注册文章路由
posts := api.Group("/posts")
posts.Get("", listPosts)
posts.Get("/:id", getPost)

// 使用路由注册器
r.Register(&CommentHandler{}, &TagHandler{})

// 初始化路由器
r.Init()

// 打印所有路由
r.Print(os.Stdout)
```

## 10. 总结

Router 组件是 Go Simple Server 框架的核心部分，提供了高效、灵活的路由功能。通过本文档的介绍，您应该已经了解了路由器的主要功能和使用方法。在实际项目中，合理组织和使用路由可以使您的代码更加清晰和可维护。