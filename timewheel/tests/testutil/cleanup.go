package testutil

import (
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Cleanup 清理助手
type Cleanup struct {
	t       *testing.T
	cleanup []func()
	mu      sync.Mutex
}

// NewCleanup 创建清理助手
func NewCleanup(t *testing.T) *Cleanup {
	return &Cleanup{
		t:       t,
		cleanup: make([]func(), 0),
	}
}

// Add 添加清理函数
func (c *Cleanup) Add(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanup = append(c.cleanup, fn)
}

// Run 执行所有清理函数
func (c *Cleanup) Run() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 逆序执行
	for i := len(c.cleanup) - 1; i >= 0; i-- {
		fn := c.cleanup[i]
		func() {
			defer func() {
				if r := recover(); r != nil {
					c.t.Logf("Cleanup panic: %v", r)
				}
			}()
			fn()
		}()
	}
	c.cleanup = c.cleanup[:0]
}

// TempDir 创建临时目录
func (c *Cleanup) TempDir(prefix string) string {
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		c.t.Fatalf("Failed to create temp dir: %v", err)
	}
	c.Add(func() {
		os.RemoveAll(dir)
	})
	return dir
}

// TempFile 创建临时文件
func (c *Cleanup) TempFile(pattern string) *os.File {
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		c.t.Fatalf("Failed to create temp file: %v", err)
	}
	c.Add(func() {
		file.Close()
		os.Remove(file.Name())
	})
	return file
}

// CleanupRegistry 全局清理注册表
type CleanupRegistry struct {
	cleanups []func()
	mu       sync.Mutex
}

var globalCleanup = &CleanupRegistry{
	cleanups: make([]func(), 0),
}

// RegisterCleanup 注册全局清理函数
func RegisterCleanup(fn func()) {
	globalCleanup.mu.Lock()
	defer globalCleanup.mu.Unlock()
	globalCleanup.cleanups = append(globalCleanup.cleanups, fn)
}

// RunGlobalCleanups 执行所有全局清理函数
func RunGlobalCleanups() {
	globalCleanup.mu.Lock()
	defer globalCleanup.mu.Unlock()

	for i := len(globalCleanup.cleanups) - 1; i >= 0; i-- {
		fn := globalCleanup.cleanups[i]
		func() {
			defer func() {
				if r := recover(); r != nil {
					// 忽略清理时的 panic
				}
			}()
			fn()
		}()
	}
	globalCleanup.cleanups = globalCleanup.cleanups[:0]
}

// Timeout 超时助手
type Timeout struct {
	timeout  time.Duration
	start    time.Time
	onFinish func()
}

// NewTimeout 创建超时助手
func NewTimeout(timeout time.Duration, onFinish func()) *Timeout {
	return &Timeout{
		timeout:  timeout,
		start:    time.Now(),
		onFinish: onFinish,
	}
}

// Check 检查是否超时
func (t *Timeout) Check() bool {
	return time.Since(t.start) > t.timeout
}

// Remaining 剩余时间
func (t *Timeout) Remaining() time.Duration {
	remaining := t.timeout - time.Since(t.start)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Finish 完成并调用回调
func (t *Timeout) Finish() {
	if t.onFinish != nil {
		t.onFinish()
	}
}

// ResourceMonitor 资源监控器
type ResourceMonitor struct {
	t           *testing.T
	maxMemoryMB uint64
	maxGoroutine int
	interval    time.Duration
	stop        chan struct{}
	wg          sync.WaitGroup
}

// NewResourceMonitor 创建资源监控器
func NewResourceMonitor(t *testing.T, maxMemoryMB uint64, maxGoroutine int) *ResourceMonitor {
	return &ResourceMonitor{
		t:            t,
		maxMemoryMB:  maxMemoryMB,
		maxGoroutine: maxGoroutine,
		interval:     time.Second,
		stop:         make(chan struct{}),
	}
}

// Start 开始监控
func (m *ResourceMonitor) Start() {
	m.wg.Add(1)
	go m.run()
}

// Stop 停止监控
func (m *ResourceMonitor) Stop() {
	close(m.stop)
	m.wg.Wait()
}

func (m *ResourceMonitor) run() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.check()
		}
	}
}

func (m *ResourceMonitor) check() {
	// 检查内存
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memMB := memStats.Alloc / 1024 / 1024
	if memMB > m.maxMemoryMB {
		m.t.Logf("Warning: Memory usage %d MB exceeds limit %d MB", memMB, m.maxMemoryMB)
	}

	// 检查 goroutine
	goroutineCount := runtime.NumGoroutine()
	if goroutineCount > m.maxGoroutine {
		m.t.Logf("Warning: Goroutine count %d exceeds limit %d", goroutineCount, m.maxGoroutine)
	}
}

// TestContext 测试上下文
type TestContext struct {
	T        *testing.T
	Cleanup  *Cleanup
	Assert   *Assertion
	Timeout  *Timeout
	Monitor  *ResourceMonitor
}

// NewTestContext 创建测试上下文
func NewTestContext(t *testing.T, timeout time.Duration) *TestContext {
	cleanup := NewCleanup(t)
	t.Cleanup(cleanup.Run)

	ctx := &TestContext{
		T:       t,
		Cleanup: cleanup,
		Assert:  NewAssertion(t),
	}

	if timeout > 0 {
		ctx.Timeout = NewTimeout(timeout, func() {
			t.Logf("Test timeout after %v", timeout)
		})
	}

	return ctx
}

// WithResourceMonitor 启用资源监控
func (c *TestContext) WithResourceMonitor(maxMemoryMB uint64, maxGoroutine int) *TestContext {
	c.Monitor = NewResourceMonitor(c.T, maxMemoryMB, maxGoroutine)
	c.Cleanup.Add(c.Monitor.Stop)
	c.Monitor.Start()
	return c
}

// Parallel 标记测试可并行
func (c *TestContext) Parallel() {
	c.T.Parallel()
}

// Skip 跳过测试
func (c *TestContext) Skip(args ...interface{}) {
	c.T.Skip(args...)
}

// Skipf 跳过测试（格式化）
func (c *TestContext) Skipf(format string, args ...interface{}) {
	c.T.Skipf(format, args...)
}

// Fatal 致命错误
func (c *TestContext) Fatal(args ...interface{}) {
	c.T.Fatal(args...)
}

// Fatalf 致命错误（格式化）
func (c *TestContext) Fatalf(format string, args ...interface{}) {
	c.T.Fatalf(format, args...)
}

// Error 错误
func (c *TestContext) Error(args ...interface{}) {
	c.T.Error(args...)
}

// Errorf 错误（格式化）
func (c *TestContext) Errorf(format string, args ...interface{}) {
	c.T.Errorf(format, args...)
}

// Log 日志
func (c *TestContext) Log(args ...interface{}) {
	c.T.Log(args...)
}

// Logf 日志（格式化）
func (c *TestContext) Logf(format string, args ...interface{}) {
	c.T.Logf(format, args...)
}

// Name 获取测试名称
func (c *TestContext) Name() string {
	return c.T.Name()
}

// Run 运行子测试
func (c *TestContext) Run(name string, f func(t *testing.T)) bool {
	return c.T.Run(name, f)
}
