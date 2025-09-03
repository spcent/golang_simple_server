package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	// 命令行参数解析
	addr := flag.String("addr", ":8080", "Server address to listen on")
	env := flag.String("env", ".env", "Path to .env file")
	flag.Parse()

	// 如果env文件存在，加载环境变量
	if _, err := os.Stat(*env); err == nil {
		fmt.Println("加载.env 文件:", *env)
		err := LoadEnv(*env, true)
		if err != nil {
			fmt.Println("加载 .env 失败:", err)
			return
		}
	}

	AddRoute("/hello/{name}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		name := params["name"]
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"message":"Hello, %s!"}`, name)))
	})
	AddRoute("/users/{id}/posts/{postID}", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		id := params["id"]
		postID := params["postID"]
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"user": "%s", "post": "%s"}`, id, postID)))
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/", ApplyMiddleware(routerHandler, LoggingMiddleware, AuthMiddleware))
	mux.HandleFunc("/ping", pingHandler)
	mux.HandleFunc("/hello", ApplyMiddleware(helloHandler, LoggingMiddleware))

	server := &http.Server{
		Addr:    *addr,
		Handler: mux,
	}

	// 优雅关闭
	idleConnsClosed := make(chan struct{})
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig

		fmt.Println("\nShutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			fmt.Println("Error shutting down:", err)
		}
		close(idleConnsClosed)
	}()

	fmt.Printf("Server running on %s\n", *addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		fmt.Println("Server error:", err)
	}

	<-idleConnsClosed
	fmt.Println("Server stopped gracefully")
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message":"pong"}`))
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/hello" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message":"Hello, World!"}`))
}

// LoadEnv 从指定路径加载 .env 文件到环境变量
// 如果 overwrite=true，则覆盖已有环境变量
func LoadEnv(filepath string, overwrite bool) error {
	file, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 按第一个 '=' 分割 key/value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // 忽略不合法的行
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// 去掉可能的引号
		value = strings.Trim(value, `"'`)

		// 设置环境变量
		if overwrite || os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}

	return scanner.Err()
}
