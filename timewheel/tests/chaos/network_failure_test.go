package chaos

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"timewheel/internal/config"
	"timewheel/pkg/webhook"
	"timewheel/tests/fixtures"
	"timewheel/tests/testutil"
)

// TestNetworkFailure_WebhookTimeout 测试 Webhook 超时
func TestNetworkFailure_WebhookTimeout(t *testing.T) {
	// 创建慢速 Mock 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second) // 10 秒延迟
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 创建带短超时的客户端
	cfg := &config.WebhookConfig{
		URL:     server.URL,
		Timeout: 1 * time.Second, // 1 秒超时
		Headers: map[string]string{},
		Retry: config.RetryConfig{
			MaxAttempts:     1,
			InitialInterval: 100 * time.Millisecond,
			MaxInterval:     1 * time.Second,
			Multiplier:      2.0,
		},
	}

	client := webhook.NewClient(cfg, nil)
	ctx := context.Background()

	payload := &webhook.AlertPayload{
		TaskID:    "timeout-test",
		TaskName:  "Timeout Test",
		State:     "firing",
		Value:     85.0,
		Threshold: 80.0,
		Severity:  "warning",
		Timestamp: time.Now(),
	}

	start := time.Now()
	err := client.Send(ctx, payload)
	duration := time.Since(start)

	assert := testutil.NewAssertion(t)
	assert.Error(err, "Should timeout")
	assert.LessThan(duration.Milliseconds(), int64(2000), "Should fail within 2 seconds")

	t.Logf("Webhook timeout after %v (expected: ~1s)", duration)
}

// TestNetworkFailure_RedisDisconnection 测试 Redis 断连
func TestNetworkFailure_RedisDisconnection(t *testing.T) {
	// 使用 Mock Redis 模拟断连
	mockRedis := fixtures.NewMockRedisServer()

	// 模拟正常操作
	mockRedis.Set("lock:task-1", "locked")
	mockRedis.Set("lock:task-2", "locked")

	assert := testutil.NewAssertion(t)

	// 验证正常操作
	val, ok := mockRedis.Get("lock:task-1")
	assert.True(ok, "Should have lock")
	assert.Equal("locked", val, "Lock value should match")

	// 模拟断连（清空数据）
	mockRedis.Clear()

	// 验证断连后状态
	_, ok = mockRedis.Get("lock:task-1")
	assert.False(ok, "Should not have lock after disconnect")

	t.Log("Redis disconnect simulation passed")
}

// TestNetworkFailure_MySQLConnectionLost 测试 MySQL 连接丢失
func TestNetworkFailure_MySQLConnectionLost(t *testing.T) {
	// 创建数据库固件
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// 正常创建任务
	task := fixtures.NewTaskFixture().ToModel()
	err = dbFixture.DB.Create(task).Error
	assert.NoError(err, "Create should succeed")

	// 模拟连接丢失（关闭数据库）
	dbFixture.Close()

	// 尝试操作（应该失败）
	var count int64
	err = dbFixture.DB.Model(task).Count(&count).Error
	assert.Error(err, "Should fail after connection lost")

	t.Log("MySQL connection lost simulation passed")
}

// TestNetworkFailure_PartialConnectivity 测试部分连通性
func TestNetworkFailure_PartialConnectivity(t *testing.T) {
	// 创建两个 Mock 服务器：一个正常，一个失败
	goodServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer goodServer.Close()

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badServer.Close()

	assert := testutil.NewAssertion(t)

	// 测试正常服务器
	resp, err := http.Get(goodServer.URL)
	assert.NoError(err, "Good server should respond")
	assert.Equal(http.StatusOK, resp.StatusCode, "Should return 200")

	// 测试失败服务器
	resp, err = http.Get(badServer.URL)
	assert.NoError(err, "Bad server should respond")
	assert.Equal(http.StatusInternalServerError, resp.StatusCode, "Should return 500")

	t.Log("Partial connectivity test passed")
}

// TestNetworkFailure_RetryWithBackoff 测试指数退避重试
func TestNetworkFailure_RetryWithBackoff(t *testing.T) {
	var attemptCount atomic.Int32

	// 创建会失败几次后成功的 Mock 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attemptCount.Add(1)

		if attempt < 3 {
			// 前 2 次失败
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// 第 3 次成功
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.WebhookConfig{
		URL:     server.URL,
		Timeout: 5 * time.Second,
		Retry: config.RetryConfig{
			MaxAttempts:     5,
			InitialInterval: 100 * time.Millisecond,
			MaxInterval:     1 * time.Second,
			Multiplier:      2.0,
		},
	}

	client := webhook.NewClient(cfg, nil)
	ctx := context.Background()

	start := time.Now()
	err := client.SendWithRetry(ctx, &webhook.AlertPayload{
		TaskID:    "retry-test",
		TaskName:  "Retry Test",
		State:     "firing",
		Timestamp: time.Now(),
	})
	duration := time.Since(start)

	assert := testutil.NewAssertion(t)
	assert.NoError(err, "Should succeed after retries")
	assert.Equal(int32(3), attemptCount.Load(), "Should have 3 attempts")

	// 验证指数退避：总时间应该 > 100ms + 200ms = 300ms
	assert.GreaterThan(duration.Milliseconds(), int64(250), "Should have retry delays")

	t.Logf("Retry succeeded after %v with %d attempts", duration, attemptCount.Load())
}

// TestNetworkFailure_CircuitBreaker 测试熔断器模式
func TestNetworkFailure_CircuitBreaker(t *testing.T) {
	// 简化的熔断器实现
	circuitBreaker := &CircuitBreaker{
		maxFailures:   3,
		resetTimeout:  1 * time.Second,
		failureCount:  0,
		state:         "closed",
		lastFailTime:  time.Time{},
	}

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	// 模拟多次失败，触发熔断
	for i := 0; i < 10; i++ {
		if circuitBreaker.Allow() {
			resp, _ := http.Get(server.URL)
			if resp.StatusCode >= 500 {
				circuitBreaker.RecordFailure()
			}
		}
	}

	// 验证熔断后请求被阻止
	assert := testutil.NewAssertion(t)
	assert.LessThan(int(requestCount.Load()), 10, "Circuit breaker should stop requests after failures")
	assert.Equal("open", circuitBreaker.state, "Circuit should be open")

	t.Logf("Circuit breaker test: %d actual requests (expected < 10)", requestCount.Load())
}

// CircuitBreaker 简化的熔断器
type CircuitBreaker struct {
	maxFailures  int
	resetTimeout time.Duration
	failureCount int
	state        string
	lastFailTime time.Time
	mu           sync.Mutex
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == "open" {
		if time.Since(cb.lastFailTime) > cb.resetTimeout {
			cb.state = "half-open"
			return true
		}
		return false
	}

	return true
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailTime = time.Now()

	if cb.failureCount >= cb.maxFailures {
		cb.state = "open"
	}
}

// TestNetworkFailure_TimeoutChain 测试超时链传播
func TestNetworkFailure_TimeoutChain(t *testing.T) {
	// 创建带层级超时的上下文链
	rootCtx := context.Background()

	// 第一层：5 秒超时
	ctx1, cancel1 := context.WithTimeout(rootCtx, 5*time.Second)
	defer cancel1()

	// 第二层：2 秒超时
	ctx2, cancel2 := context.WithTimeout(ctx1, 2*time.Second)
	defer cancel2()

	// 第三层：1 秒超时
	ctx3, cancel3 := context.WithTimeout(ctx2, 1*time.Second)
	defer cancel3()

	start := time.Now()

	select {
	case <-time.After(10 * time.Second):
		t.Log("Timer completed (should not happen)")
	case <-ctx3.Done():
		duration := time.Since(start)
		assert := testutil.NewAssertion(t)
		assert.LessThan(duration.Milliseconds(), int64(1200), "Should timeout at ~1s")
		t.Logf("Context chain timeout after %v", duration)
	}
}

// TestNetworkFailure_GracefulDegradation 测试优雅降级
func TestNetworkFailure_GracefulDegradation(t *testing.T) {
	// 创建 Mock 服务器
	serverUp := atomic.Bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !serverUp.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.WebhookConfig{
		URL:     server.URL,
		Timeout: 1 * time.Second,
		Retry: config.RetryConfig{
			MaxAttempts:     2,
			InitialInterval: 50 * time.Millisecond,
			Multiplier:      2.0,
		},
	}

	client := webhook.NewClient(cfg, nil)
	ctx := context.Background()

	var successCount, failureCount atomic.Int32

	// 模拟服务不可用期间的请求
	for i := 0; i < 10; i++ {
		err := client.Send(ctx, &webhook.AlertPayload{TaskID: fmt.Sprintf("task-%d", i)})
		if err != nil {
			failureCount.Add(1)
		} else {
			successCount.Add(1)
		}
	}

	// 恢复服务
	serverUp.Store(true)

	// 验证恢复后正常工作
	for i := 0; i < 5; i++ {
		err := client.Send(ctx, &webhook.AlertPayload{TaskID: fmt.Sprintf("recovery-%d", i)})
		assert := testutil.NewAssertion(t)
		assert.NoError(err, "Should succeed after recovery")
	}

	t.Logf("Graceful degradation: %d failures, %d successes during outage",
		failureCount.Load(), successCount.Load())
}

// Import encoding/json for the test
import "encoding/json"
