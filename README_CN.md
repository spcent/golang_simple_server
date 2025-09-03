# 轻量级HTTP服务骨架

一个基于Go标准库的轻量级HTTP服务骨架，包含动态路由、中间件、优雅关闭等核心功能，适合快速搭建简单API服务或学习HTTP服务开发。

## 功能特性
- **动态路由**：支持`/hello/{name}`格式的路径参数匹配
- **中间件系统**：内置日志（Logging）和鉴权（Auth）中间件，支持自定义扩展
- **优雅关闭**：接收中断信号后5秒内完成连接清理
- **日志**：支持glog日志库，支持日志级别配置
- **env配置**：支持env文件配置，包括日志级别等
- **测试覆盖**：包含路由匹配、中间件、404处理等核心功能测试
- **开发工具链**：提供Makefile和dev.sh脚本简化构建/运行/测试流程

## 快速开始

### 环境要求
- Go 1.18+

### 构建运行
```bash
# 使用Makefile
make run  # 构建并启动服务（默认端口:8080）

# 或使用dev.sh脚本
./dev.sh run
```

### 自定义端口
```bash
./golang_simple_server -addr :9090  # 指定9090端口启动
```

## 路由定义
通过 router.(Get|Post|Delete|Put) 函数注册带参数的路由（示例在 main.go ）：

```go
router.Get("/hello/{name}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
    // 使用params["name"]获取路径参数
})
router.Get("/users/{id}/posts/{postID}", func(...) {
    // 使用params["id"]和params["postID"]
})
```

## 路由测试
启动服务后，通过curl测试路由：

```bash
curl http://localhost:8080/ping # pong
curl http://localhost:8080/hello # {"message":"Hello, World!"}
curl -H "X-Token: secret" http://localhost:8080/hello/Alice # {"message":"Hello, Alice!"}
```

## 中间件说明
- LoggingMiddleware ：记录请求耗时（格式： [时间] 方法 路径 (耗时) ）
- AuthMiddleware ：校验 X-Token 请求头
- CorsMiddleware ：允许跨域请求

中间件组合使用（示例在 main.go ）：
```go
mux.HandleFunc("/", middleware.Apply(routerHandler, LoggingMiddleware, AuthMiddleware))
```

## 运行测试
```bash
make test       # 运行所有测试
make coverage   # 生成覆盖率报告（输出到coverage.html）
```

## 许可
MIT