package config

import (
	"os"
	"testing"
)

func TestLoadEnv(t *testing.T) {
	// 创建临时文件模拟 .env
	content := `
# 注释行
DB_HOST=127.0.0.1
DB_USER=root
DB_PASS="secret"
EMPTY_KEY=
QUOTED_KEY='quoted_value'
`
	tmpFile := "test.env"
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile)

	// 清理环境变量，避免影响测试
	os.Clearenv()

	// 设置一个已有变量，确保不会被覆盖
	os.Setenv("DB_USER", "existing_user")

	// 调用 LoadEnv
	err = LoadEnv(tmpFile, false)
	if err != nil {
		t.Fatalf("LoadEnv 执行失败: %v", err)
	}

	// 校验结果
	tests := []struct {
		key      string
		expected string
	}{
		{"DB_HOST", "127.0.0.1"},
		{"DB_USER", "existing_user"}, // 不覆盖已有值
		{"DB_PASS", "secret"},
		{"EMPTY_KEY", ""},
		{"QUOTED_KEY", "quoted_value"},
	}

	for _, tt := range tests {
		got := os.Getenv(tt.key)
		if got != tt.expected {
			t.Errorf("环境变量 %s = %q, 期望 %q", tt.key, got, tt.expected)
		}
	}
}

func TestLoadEnvWithOverwrite(t *testing.T) {
	content := `
DB_HOST=127.0.0.1
DB_USER=root
`
	tmpFile := "test_overwrite.env"
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile)

	// 情况1: 不覆盖已有值
	os.Clearenv()
	os.Setenv("DB_USER", "existing_user")
	err = LoadEnv(tmpFile, false)
	if err != nil {
		t.Fatalf("LoadEnv 执行失败: %v", err)
	}
	if got := os.Getenv("DB_USER"); got != "existing_user" {
		t.Errorf("DB_USER 应该保持为 existing_user, 实际是 %q", got)
	}

	// 情况2: 覆盖已有值
	os.Clearenv()
	os.Setenv("DB_USER", "existing_user")
	err = LoadEnv(tmpFile, true)
	if err != nil {
		t.Fatalf("LoadEnv 执行失败: %v", err)
	}
	if got := os.Getenv("DB_USER"); got != "root" {
		t.Errorf("DB_USER 应该被覆盖为 root, 实际是 %q", got)
	}
}

func TestLoadEnvFileNotFound(t *testing.T) {
	os.Clearenv()
	err := LoadEnv("nonexistent.env", false)
	if err == nil {
		t.Fatal("期望 LoadEnv 返回错误，但没有返回")
	}
}
