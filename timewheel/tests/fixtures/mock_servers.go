package fixtures

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"timewheel/pkg/webhook"
)

// MockWebhookServer Mock Webhook 服务器
type MockWebhookServer struct {
	server     *http.Server
	port       int
	requests   []*webhook.AlertPayload
	mu         sync.Mutex
	shouldFail bool
	delay      time.Duration
	callCount  int64
	started    atomic.Bool
}

// NewMockWebhookServer 创建 Mock Webhook 服务器
func NewMockWebhookServer() *MockWebhookServer {
	return &MockWebhookServer{
		requests: make([]*webhook.AlertPayload, 0),
	}
}

// Start 启动服务器
func (s *MockWebhookServer) Start() error {
	// 找一个可用端口
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return fmt.Errorf("failed to find available port: %w", err)
	}
	s.port = listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", s.handleWebhook)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Handler: mux,
	}

	go func() {
		s.started.Store(true)
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			// 服务器关闭
		}
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)
	return nil
}

// Stop 停止服务器
func (s *MockWebhookServer) Stop() error {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}

func (s *MockWebhookServer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	atomic.AddInt64(&s.callCount, 1)

	// 模拟延迟
	if s.delay > 0 {
		time.Sleep(s.delay)
	}

	// 模拟失败
	if s.shouldFail {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "mock error"})
		return
	}

	var payload webhook.AlertPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	s.requests = append(s.requests, &payload)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *MockWebhookServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// URL 获取服务器 URL
func (s *MockWebhookServer) URL() string {
	return fmt.Sprintf("http://localhost:%d/webhook", s.port)
}

// Port 获取服务器端口
func (s *MockWebhookServer) Port() int {
	return s.port
}

// SetFail 设置是否返回失败
func (s *MockWebhookServer) SetFail(shouldFail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shouldFail = shouldFail
}

// SetDelay 设置响应延迟
func (s *MockWebhookServer) SetDelay(delay time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delay = delay
}

// GetRequests 获取所有请求
func (s *MockWebhookServer) GetRequests() []*webhook.AlertPayload {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*webhook.AlertPayload, len(s.requests))
	copy(result, s.requests)
	return result
}

// GetCallCount 获取调用次数
func (s *MockWebhookServer) GetCallCount() int64 {
	return atomic.LoadInt64(&s.callCount)
}

// ClearRequests 清空请求记录
func (s *MockWebhookServer) ClearRequests() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = make([]*webhook.AlertPayload, 0)
	atomic.StoreInt64(&s.callCount, 0)
}

// LastRequest 获取最后一个请求
func (s *MockWebhookServer) LastRequest() *webhook.AlertPayload {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		return nil
	}
	return s.requests[len(s.requests)-1]
}

// MockHTTPServer 通用 Mock HTTP 服务器
type MockHTTPServer struct {
	server   *http.Server
	port     int
	handlers map[string]http.HandlerFunc
	mu       sync.Mutex
	started  atomic.Bool
}

// NewMockHTTPServer 创建通用 Mock HTTP 服务器
func NewMockHTTPServer() *MockHTTPServer {
	return &MockHTTPServer{
		handlers: make(map[string]http.HandlerFunc),
	}
}

// Handle 注册处理函数
func (s *MockHTTPServer) Handle(path string, handler http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[path] = handler
}

// Start 启动服务器
func (s *MockHTTPServer) Start() error {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return fmt.Errorf("failed to find available port: %w", err)
	}
	s.port = listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	s.mu.Lock()
	for path, handler := range s.handlers {
		mux.HandleFunc(path, handler)
	}
	s.mu.Unlock()

	s.server = &http.Server{
		Handler: mux,
	}

	go func() {
		s.started.Store(true)
		s.server.Serve(listener)
	}()

	time.Sleep(100 * time.Millisecond)
	return nil
}

// Stop 停止服务器
func (s *MockHTTPServer) Stop() error {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}

// URL 获取基础 URL
func (s *MockHTTPServer) URL() string {
	return fmt.Sprintf("http://localhost:%d", s.port)
}

// Port 获取端口
func (s *MockHTTPServer) Port() int {
	return s.port
}

// MockRedisServer Mock Redis 服务器（简化版，仅用于测试）
type MockRedisServer struct {
	data    map[string]string
	mu      sync.RWMutex
	started atomic.Bool
}

// NewMockRedisServer 创建 Mock Redis 服务器
func NewMockRedisServer() *MockRedisServer {
	return &MockRedisServer{
		data: make(map[string]string),
	}
}

// Set 设置值
func (s *MockRedisServer) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

// Get 获取值
func (s *MockRedisServer) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.data[key]
	return val, ok
}

// Del 删除值
func (s *MockRedisServer) Del(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

// Exists 检查键是否存在
func (s *MockRedisServer) Exists(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.data[key]
	return ok
}

// SetNX 设置值（如果不存在）
func (s *MockRedisServer) SetNX(key, value string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[key]; ok {
		return false
	}
	s.data[key] = value
	return true
}

// Clear 清空所有数据
func (s *MockRedisServer) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[string]string)
}

// Count 获取键数量
func (s *MockRedisServer) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.data)
}
