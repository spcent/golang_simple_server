package glog

import (
	"bytes"
	"io"
	stdlog "log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// 初始化日志
	Init()
}

// 测试辅助函数

// captureOutput 捕获输出到缓冲区
func captureOutput(f func()) string {
	var buf bytes.Buffer
	std.mu.Lock()
	oldOutput := std.output
	std.output = &buf
	std.mu.Unlock()

	f()
	std.Flush()

	std.mu.Lock()
	std.output = oldOutput
	std.mu.Unlock()

	return buf.String()
}

// createTempDir 创建临时目录
func createTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "glog_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	return dir
}

// cleanupTempDir 清理临时目录
func cleanupTempDir(t *testing.T, dir string) {
	if err := os.RemoveAll(dir); err != nil {
		t.Errorf("Failed to cleanup temp dir: %v", err)
	}
}

// resetGlobalLogger 重置全局logger到默认状态
func resetGlobalLogger() {
	std.mu.Lock()
	defer std.mu.Unlock()

	// 关闭所有文件
	for _, file := range std.logFiles {
		if file != nil {
			file.Close()
		}
	}

	// 重置到默认状态
	std.level = INFO
	std.output = os.Stderr
	std.toStderr = true
	std.alsoToStderr = false
	std.verbosity = 0
	std.vmodulePatterns = nil
	std.logBacktraceAt = ""
	std.logFiles = make(map[Level]*os.File)
	std.logDir = ""
	std.program = filepath.Base(os.Args[0])
}

// TestBasicLogging 测试基本日志功能
func TestBasicLogging(t *testing.T) {
	resetGlobalLogger()

	tests := []struct {
		name     string
		logFunc  func()
		expected string
	}{
		{
			name:     "Info",
			logFunc:  func() { Info("test info message") },
			expected: "test info message",
		},
		{
			name:     "Infof",
			logFunc:  func() { Infof("test %s message", "info") },
			expected: "test info message",
		},
		{
			name:     "Warning",
			logFunc:  func() { Warning("test warning message") },
			expected: "test warning message",
		},
		{
			name:     "Warningf",
			logFunc:  func() { Warningf("test %s message", "warning") },
			expected: "test warning message",
		},
		{
			name:     "Error",
			logFunc:  func() { Error("test error message") },
			expected: "test error message",
		},
		{
			name:     "Errorf",
			logFunc:  func() { Errorf("test %s message", "error") },
			expected: "test error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureOutput(tt.logFunc)
			if !strings.Contains(output, tt.expected) {
				t.Errorf("Expected output to contain %q, got %q", tt.expected, output)
			}

			// 验证日志格式
			if !regexp.MustCompile(`[IWEF]\d{4} \d{2}:\d{2}:\d{2}\.\d{6} +\d+ \w+:\d+\]`).MatchString(output) {
				t.Errorf("Log format is incorrect: %q", output)
			}
		})
	}
}

// TestLogLevels 测试日志级别过滤
func TestLogLevels(t *testing.T) {
	resetGlobalLogger()

	tests := []struct {
		name      string
		setLevel  Level
		logLevel  Level
		logFunc   func()
		shouldLog bool
	}{
		{"INFO level allows INFO", INFO, INFO, func() { Info("test") }, true},
		{"INFO level allows WARNING", INFO, WARNING, func() { Warning("test") }, true},
		{"INFO level allows ERROR", INFO, ERROR, func() { Error("test") }, true},
		{"WARNING level blocks INFO", WARNING, INFO, func() { Info("test") }, false},
		{"WARNING level allows WARNING", WARNING, WARNING, func() { Warning("test") }, true},
		{"WARNING level allows ERROR", WARNING, ERROR, func() { Error("test") }, true},
		{"ERROR level blocks INFO", ERROR, INFO, func() { Info("test") }, false},
		{"ERROR level blocks WARNING", ERROR, WARNING, func() { Warning("test") }, false},
		{"ERROR level allows ERROR", ERROR, ERROR, func() { Error("test") }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			std.SetLevel(tt.setLevel)
			output := captureOutput(tt.logFunc)

			hasOutput := len(strings.TrimSpace(output)) > 0
			if hasOutput != tt.shouldLog {
				t.Errorf("Expected shouldLog=%v, got output=%q", tt.shouldLog, output)
			}
		})
	}
}

// TestVerboseLogging 测试详细日志
func TestVerboseLogging(t *testing.T) {
	resetGlobalLogger()

	tests := []struct {
		name      string
		verbosity int
		logLevel  int
		shouldLog bool
	}{
		{"V(0) with verbosity 0", 0, 0, true},
		{"V(1) with verbosity 0", 0, 1, false},
		{"V(1) with verbosity 1", 1, 1, true},
		{"V(2) with verbosity 1", 1, 2, false},
		{"V(2) with verbosity 2", 2, 2, true},
		{"V(5) with verbosity 10", 10, 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			std.SetVerbose(tt.verbosity)

			if V(tt.logLevel) != tt.shouldLog {
				t.Errorf("V(%d) with verbosity %d: expected %v", tt.logLevel, tt.verbosity, tt.shouldLog)
			}

			// 测试VLog
			output := captureOutput(func() {
				VLog(tt.logLevel, "verbose test message")
			})

			hasOutput := len(strings.TrimSpace(output)) > 0
			if hasOutput != tt.shouldLog {
				t.Errorf("VLog(%d) with verbosity %d: expected shouldLog=%v, got output=%q",
					tt.logLevel, tt.verbosity, tt.shouldLog, output)
			}
		})
	}
}

// TestVmodule 测试vmodule功能
func TestVmodule(t *testing.T) {
	resetGlobalLogger()

	// 测试vmodule解析
	std.parseVmodule("glog_test=2,other=1,pattern*=3")

	expected := []vmodulePattern{
		{"glog_test", 2},
		{"other", 1},
		{"pattern*", 3},
	}

	if len(std.vmodulePatterns) != len(expected) {
		t.Fatalf("Expected %d patterns, got %d", len(expected), len(std.vmodulePatterns))
	}

	for i, pattern := range std.vmodulePatterns {
		if pattern != expected[i] {
			t.Errorf("Pattern %d: expected %+v, got %+v", i, expected[i], pattern)
		}
	}

	// 测试文件匹配
	testFile := "glog_test.go"
	verbosity := std.getVerbosityForFile(testFile)
	if verbosity != 2 {
		t.Errorf("Expected verbosity 2 for %s, got %d", testFile, verbosity)
	}

	// 测试不匹配的文件使用全局verbosity
	std.verbosity = 5
	testFile = "unmatched.go"
	verbosity = std.getVerbosityForFile(testFile)
	if verbosity != 5 {
		t.Errorf("Expected verbosity 5 for %s, got %d", testFile, verbosity)
	}
}

// TestLogBacktrace 测试堆栈跟踪
func TestLogBacktrace(t *testing.T) {
	resetGlobalLogger()

	// 设置backtrace位置（使用当前文件的某一行）
	std.logBacktraceAt = "glog_test.go:999" // 使用不存在的行号，避免实际触发

	if !std.shouldLogBacktrace("glog_test.go", 999) {
		t.Error("Should log backtrace for matching file:line")
	}

	if std.shouldLogBacktrace("glog_test.go", 1000) {
		t.Error("Should not log backtrace for non-matching line")
	}

	if std.shouldLogBacktrace("other.go", 999) {
		t.Error("Should not log backtrace for non-matching file")
	}
}

// TestFileOutput 测试文件输出
func TestFileOutput(t *testing.T) {
	resetGlobalLogger()

	tempDir := createTempDir(t)
	defer cleanupTempDir(t, tempDir)

	// 设置日志目录
	std.logDir = tempDir
	std.program = "testapp"

	err := std.initLogFiles()
	if err != nil {
		t.Fatalf("Failed to initialize log files: %v", err)
	}
	defer std.Close()

	// 测试不同级别的日志
	Info("info message")
	Warning("warning message")
	Error("error message")

	std.Flush()

	// 验证文件是否创建
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to read temp dir: %v", err)
	}

	var logFiles []string
	var symlinks []string

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".log") {
			logFiles = append(logFiles, file.Name())
		} else if strings.Contains(file.Name(), "testapp.") {
			symlinks = append(symlinks, file.Name())
		}
	}

	// 应该有3个日志文件（INFO、WARN、ERROR）
	if len(logFiles) < 3 {
		t.Errorf("Expected at least 3 log files, got %d: %v", len(logFiles), logFiles)
	}

	// 应该有3个软链接
	if len(symlinks) < 3 {
		t.Errorf("Expected at least 3 symlinks, got %d: %v", len(symlinks), symlinks)
	}

	// 验证INFO文件包含所有日志
	infoFile := ""
	for _, file := range logFiles {
		if strings.Contains(file, "INFO") {
			infoFile = filepath.Join(tempDir, file)
			break
		}
	}

	if infoFile == "" {
		t.Fatal("INFO log file not found")
	}

	content, err := os.ReadFile(infoFile)
	if err != nil {
		t.Fatalf("Failed to read INFO log file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "info message") {
		t.Error("INFO file should contain info message")
	}
	if !strings.Contains(contentStr, "warning message") {
		t.Error("INFO file should contain warning message")
	}
	if !strings.Contains(contentStr, "error message") {
		t.Error("INFO file should contain error message")
	}
}

// TestConcurrentLogging 测试并发日志记录
func TestConcurrentLogging(t *testing.T) {
	resetGlobalLogger()

	var buf bytes.Buffer
	std.SetOutput(&buf)

	const numGoroutines = 10
	const numLogs = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numLogs; j++ {
				Infof("goroutine %d log %d", id, j)
			}
		}(i)
	}

	wg.Wait()

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// 验证日志数量
	expectedLines := numGoroutines * numLogs
	if len(lines) != expectedLines {
		t.Errorf("Expected %d log lines, got %d", expectedLines, len(lines))
	}

	// 验证每行都是正确格式
	for i, line := range lines {
		if !strings.Contains(line, "goroutine") || !strings.Contains(line, "log") {
			t.Errorf("Line %d has incorrect format: %s", i, line)
		}
	}
}

// TestLoggerInstance 测试自定义Logger实例
func TestLoggerInstance(t *testing.T) {
	logger := New()

	var buf bytes.Buffer
	logger.SetOutput(&buf)
	logger.SetLevel(WARNING)

	// INFO级别应该被过滤
	logger.Info("should not appear")
	if buf.Len() > 0 {
		t.Error("INFO log should be filtered when level is WARNING")
	}

	// WARNING级别应该出现
	logger.Warning("should appear")
	if buf.Len() == 0 {
		t.Error("WARNING log should not be filtered")
	}

	output := buf.String()
	if !strings.Contains(output, "should appear") {
		t.Errorf("Expected warning message in output: %q", output)
	}
}

// TestCopyStandardLogTo 测试标准log重定向
func TestCopyStandardLogTo(t *testing.T) {
	resetGlobalLogger()

	var buf bytes.Buffer
	std.SetOutput(&buf)

	CopyStandardLogTo(INFO)

	// 使用标准log包记录日志

	stdlog.Print("standard log message")

	output := buf.String()
	if !strings.Contains(output, "standard log message") {
		t.Errorf("Standard log message not found in output: %q", output)
	}
}

// TestFlagIntegration 测试flag集成
func TestFlagIntegration(t *testing.T) {
	// 保存原始值
	origLogDir := *logDir
	origAlsoLogToStderr := *alsoLogToStderr
	origLogToStderr := *logToStderr
	origVerbosity := *verbosity
	origVmodule := *vmodule
	origLogBacktraceAt := *logBacktraceAt

	defer func() {
		*logDir = origLogDir
		*alsoLogToStderr = origAlsoLogToStderr
		*logToStderr = origLogToStderr
		*verbosity = origVerbosity
		*vmodule = origVmodule
		*logBacktraceAt = origLogBacktraceAt
	}()

	// 设置flag值
	tempDir := createTempDir(t)
	defer cleanupTempDir(t, tempDir)

	*logDir = tempDir
	*alsoLogToStderr = true
	*logToStderr = false
	*verbosity = 2
	*vmodule = "test=3"
	*logBacktraceAt = "test.go:123"

	// 重新初始化
	resetGlobalLogger()
	Init()
	defer std.Close()

	// 验证设置是否生效
	if std.logDir != tempDir {
		t.Errorf("Expected logDir %s, got %s", tempDir, std.logDir)
	}

	if !std.alsoToStderr {
		t.Error("Expected alsoToStderr to be true")
	}

	if std.verbosity != 2 {
		t.Errorf("Expected verbosity 2, got %d", std.verbosity)
	}

	if len(std.vmodulePatterns) == 0 {
		t.Error("Expected vmodule patterns to be parsed")
	}

	if std.logBacktraceAt != "test.go:123" {
		t.Errorf("Expected logBacktraceAt 'test.go:123', got %s", std.logBacktraceAt)
	}
}

// TestMultiWriter 测试多重写入器
func TestMultiWriter(t *testing.T) {
	resetGlobalLogger()

	tempDir := createTempDir(t)
	defer cleanupTempDir(t, tempDir)

	// 设置同时输出到文件和stderr
	std.logDir = tempDir
	std.alsoToStderr = true
	std.program = "testapp"

	err := std.initLogFiles()
	if err != nil {
		t.Fatalf("Failed to initialize log files: %v", err)
	}
	defer std.Close()

	// 捕获stderr输出
	var stderrBuf bytes.Buffer
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	go func() {
		io.Copy(&stderrBuf, r)
	}()

	Info("test multi writer message")
	std.Flush()

	w.Close()
	os.Stderr = oldStderr

	time.Sleep(100 * time.Millisecond) // 等待goroutine完成

	// 验证stderr有输出
	stderrOutput := stderrBuf.String()
	if !strings.Contains(stderrOutput, "test multi writer message") {
		t.Errorf("Message not found in stderr: %q", stderrOutput)
	}

	// 验证文件也有输出
	files, _ := os.ReadDir(tempDir)
	var infoFile string
	for _, file := range files {
		if strings.Contains(file.Name(), "INFO") && strings.HasSuffix(file.Name(), ".log") {
			infoFile = filepath.Join(tempDir, file.Name())
			break
		}
	}

	if infoFile == "" {
		t.Fatal("INFO log file not found")
	}

	fileContent, err := os.ReadFile(infoFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(fileContent), "test multi writer message") {
		t.Errorf("Message not found in log file: %q", string(fileContent))
	}
}

// BenchmarkLogging 性能基准测试
func BenchmarkLogging(b *testing.B) {
	resetGlobalLogger()
	std.SetOutput(io.Discard) // 丢弃输出以专注于测试性能

	b.ResetTimer()

	b.Run("Info", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			Info("benchmark test message")
		}
	})

	b.Run("Infof", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			Infof("benchmark test message %d", i)
		}
	})

	b.Run("VLog", func(b *testing.B) {
		std.SetVerbose(1)
		for i := 0; i < b.N; i++ {
			VLog(1, "benchmark verbose message")
		}
	})

	b.Run("VLogFiltered", func(b *testing.B) {
		std.SetVerbose(0) // 这样VLog(1)会被过滤
		for i := 0; i < b.N; i++ {
			VLog(1, "benchmark filtered message")
		}
	})
}

// BenchmarkConcurrentLogging 并发性能测试
func BenchmarkConcurrentLogging(b *testing.B) {
	resetGlobalLogger()
	std.SetOutput(io.Discard)

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Info("concurrent benchmark message")
		}
	})
}

// TestEdgeCases 测试边界情况
func TestEdgeCases(t *testing.T) {
	resetGlobalLogger()

	// 测试空消息
	output := captureOutput(func() {
		Info("")
	})
	if !strings.Contains(output, "I") { // 至少应该有级别标识
		t.Error("Empty message should still produce log header")
	}

	// 测试非常长的消息
	longMessage := strings.Repeat("a", 10000)
	output = captureOutput(func() {
		Info(longMessage)
	})
	if !strings.Contains(output, longMessage) {
		t.Error("Long message should be logged completely")
	}

	// 测试特殊字符
	specialMessage := "测试中文\n\t特殊字符"
	output = captureOutput(func() {
		Info(specialMessage)
	})
	if !strings.Contains(output, specialMessage) {
		t.Error("Special characters should be logged correctly")
	}
}

// TestErrorHandling 测试错误处理
func TestErrorHandling(t *testing.T) {
	resetGlobalLogger()

	// 测试无效目录
	std.logDir = "/invalid/nonexistent/directory"
	err := std.initLogFiles()
	if err == nil {
		t.Error("Should get error for invalid directory")
	}

	// 测试无效vmodule
	std.parseVmodule("invalid=abc,=123,normal=1")

	// 应该只解析出valid的部分
	validCount := 0
	for _, pattern := range std.vmodulePatterns {
		if pattern.pattern == "normal" && pattern.level == 1 {
			validCount++
		}
	}

	if validCount != 1 {
		t.Error("Should parse only valid vmodule patterns")
	}
}
