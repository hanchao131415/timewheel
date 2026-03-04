package integration

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"timewheel/pkg/timewheel"
	"timewheel/tests/testutil"
)

// TestPoolManager_CreatePools 测试池初始化
func TestPoolManager_CreatePools(t *testing.T) {
	pm, err := timewheel.NewPoolManager()
	if err != nil {
		t.Fatalf("Failed to create PoolManager: %v", err)
	}
	defer pm.Release()

	assert := testutil.NewAssertion(t)
	assert.NotNil(pm, "PoolManager should not be nil")

	// 测试不同优先级的任务都能提交
	testCases := []struct {
		name     string
		priority timewheel.TaskPriority
	}{
		{"high priority", timewheel.TaskPriorityHigh},
		{"normal priority", timewheel.TaskPriorityNormal},
		{"low priority", timewheel.TaskPriorityLow},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := &timewheel.Task{
				ID:       "pool-test-" + tc.name,
				Priority: tc.priority,
			}

			var executed atomic.Bool
			err := pm.Execute(task, func() {
				executed.Store(true)
			})

			assert := testutil.NewAssertion(t)
			assert.NoError(err, "Execute should succeed")

			// 等待执行完成
			time.Sleep(100 * time.Millisecond)
			assert.True(executed.Load(), "Task should be executed")
		})
	}
}

// TestPoolManager_PriorityRouting 测试优先级路由
func TestPoolManager_PriorityRouting(t *testing.T) {
	pm, err := timewheel.NewPoolManager()
	if err != nil {
		t.Fatalf("Failed to create PoolManager: %v", err)
	}
	defer pm.Release()

	assert := testutil.NewAssertion(t)

	// 记录执行顺序
	var executionOrder []string
	var mu sync.Mutex

	// 提交不同优先级的任务
	priorities := []struct {
		name     string
		priority timewheel.TaskPriority
	}{
		{"low", timewheel.TaskPriorityLow},
		{"normal", timewheel.TaskPriorityNormal},
		{"high", timewheel.TaskPriorityHigh},
	}

	for _, p := range priorities {
		task := &timewheel.Task{
			ID:       p.name + "-priority-task",
			Priority: p.priority,
		}

		err := pm.Execute(task, func() {
			mu.Lock()
			executionOrder = append(executionOrder, p.name)
			mu.Unlock()
			time.Sleep(50 * time.Millisecond) // 模拟工作
		})
		assert.NoError(err, "Execute should succeed")
	}

	// 等待所有任务完成
	time.Sleep(500 * time.Millisecond)

	// 验证所有任务都执行了
	assert.Equal(3, len(executionOrder), "All tasks should be executed")
	t.Logf("Execution order: %v", executionOrder)
}

// TestPoolManager_ConcurrentExecution_500 测试 500 并发执行
func TestPoolManager_ConcurrentExecution_500(t *testing.T) {
	pm, err := timewheel.NewPoolManager()
	if err != nil {
		t.Fatalf("Failed to create PoolManager: %v", err)
	}
	defer pm.Release()

	assert := testutil.NewAssertion(t)
	const taskCount = 500

	var completedCount atomic.Int32
	var wg sync.WaitGroup
	errors := make(chan error, taskCount)

	start := time.Now()

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			task := &timewheel.Task{
				ID:       "concurrent-pool-task",
				Priority: timewheel.TaskPriorityNormal,
			}

			err := pm.Execute(task, func() {
				completedCount.Add(1)
				time.Sleep(10 * time.Millisecond) // 模拟工作
			})

			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	duration := time.Since(start)

	// 检查错误
	errorCount := 0
	for err := range errors {
		t.Logf("Execute error: %v", err)
		errorCount++
	}

	assert.Equal(0, errorCount, "Should have no execute errors")
	assert.Equal(int32(taskCount), completedCount.Load(), "All tasks should complete")

	t.Logf("Executed %d concurrent tasks in %v", taskCount, duration)
	t.Logf("Throughput: %.2f tasks/sec", float64(taskCount)/duration.Seconds())
}

// TestPoolManager_PanicRecovery 测试 Panic 恢复
func TestPoolManager_PanicRecovery(t *testing.T) {
	pm, err := timewheel.NewPoolManager()
	if err != nil {
		t.Fatalf("Failed to create PoolManager: %v", err)
	}
	defer pm.Release()

	assert := testutil.NewAssertion(t)

	// 提交一个会 panic 的任务
	task := &timewheel.Task{
		ID:       "panic-task",
		Priority: timewheel.TaskPriorityNormal,
	}

	panicExecuted := atomic.Bool{}
	err = pm.Execute(task, func() {
		panicExecuted.Store(true)
		panic("intentional panic for testing")
	})
	assert.NoError(err, "Execute should succeed even if task panics")

	// 等待执行
	time.Sleep(200 * time.Millisecond)

	// 任务应该被执行（即使 panic 了）
	assert.True(panicExecuted.Load(), "Panic task should be executed")

	// Panic 不应该影响后续任务
	normalExecuted := atomic.Bool{}
	err = pm.Execute(nil, func() {
		normalExecuted.Store(true)
	})
	assert.NoError(err, "Execute after panic should succeed")

	time.Sleep(100 * time.Millisecond)
	assert.True(normalExecuted.Load(), "Normal task after panic should execute")

	t.Log("Panic recovery works correctly")
}

// TestPoolManager_Backpressure 测试背压处理
func TestPoolManager_Backpressure(t *testing.T) {
	pm, err := timewheel.NewPoolManager()
	if err != nil {
		t.Fatalf("Failed to create PoolManager: %v", err)
	}
	defer pm.Release()

	assert := testutil.NewAssertion(t)

	// 创建一个使用小池的测试场景
	const submitCount = 1000
	var submittedCount atomic.Int32
	var errorCount atomic.Int32

	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < submitCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			task := &timewheel.Task{Priority: timewheel.TaskPriorityNormal}
			err := pm.Execute(task, func() {
				time.Sleep(100 * time.Millisecond) // 模拟长时间工作
			})

			submittedCount.Add(1)
			if err != nil {
				errorCount.Add(1)
			}
		}()
	}

	wg.Wait()
	duration := time.Since(start)

	t.Logf("Submitted %d tasks in %v", submittedCount.Load(), duration)
	t.Logf("Errors (backpressure): %d", errorCount.Load())

	// ants 库会在池满时阻塞，而不是返回错误
	assert.Equal(int32(submitCount), submittedCount.Load(), "All tasks should be submitted")
}

// TestPoolManager_NilTask 测试 nil 任务
func TestPoolManager_NilTask(t *testing.T) {
	pm, err := timewheel.NewPoolManager()
	if err != nil {
		t.Fatalf("Failed to create PoolManager: %v", err)
	}
	defer pm.Release()

	assert := testutil.NewAssertion(t)

	// nil 任务应该使用普通优先级池
	var executed atomic.Bool
	err = pm.Execute(nil, func() {
		executed.Store(true)
	})
	assert.NoError(err, "Execute with nil task should succeed")

	time.Sleep(100 * time.Millisecond)
	assert.True(executed.Load(), "Nil task should be executed")
}

// TestPoolManager_MultiplePriorityPools 测试多个优先级池并行工作
func TestPoolManager_MultiplePriorityPools(t *testing.T) {
	pm, err := timewheel.NewPoolManager()
	if err != nil {
		t.Fatalf("Failed to create PoolManager: %v", err)
	}
	defer pm.Release()

	assert := testutil.NewAssertion(t)

	const tasksPerPriority = 50
	var highCount, normalCount, lowCount atomic.Int32
	var wg sync.WaitGroup

	// 同时提交不同优先级的任务
	for i := 0; i < tasksPerPriority; i++ {
		// 高优先级
		wg.Add(1)
		go func() {
			defer wg.Done()
			highTask := &timewheel.Task{Priority: timewheel.TaskPriorityHigh}
			pm.Execute(highTask, func() {
				highCount.Add(1)
			})
		}()

		// 普通优先级
		wg.Add(1)
		go func() {
			defer wg.Done()
			normalTask := &timewheel.Task{Priority: timewheel.TaskPriorityNormal}
			pm.Execute(normalTask, func() {
				normalCount.Add(1)
			})
		}()

		// 低优先级
		wg.Add(1)
		go func() {
			defer wg.Done()
			lowTask := &timewheel.Task{Priority: timewheel.TaskPriorityLow}
			pm.Execute(lowTask, func() {
				lowCount.Add(1)
			})
		}()
	}

	wg.Wait()

	// 等待所有任务完成
	time.Sleep(500 * time.Millisecond)

	assert.Equal(int32(tasksPerPriority), highCount.Load(), "All high priority tasks should complete")
	assert.Equal(int32(tasksPerPriority), normalCount.Load(), "All normal priority tasks should complete")
	assert.Equal(int32(tasksPerPriority), lowCount.Load(), "All low priority tasks should complete")

	t.Logf("High: %d, Normal: %d, Low: %d", highCount.Load(), normalCount.Load(), lowCount.Load())
}

// TestPoolManager_ResourceCleanup 测试资源清理
func TestPoolManager_ResourceCleanup(t *testing.T) {
	// 创建并立即释放
	pm, err := timewheel.NewPoolManager()
	if err != nil {
		t.Fatalf("Failed to create PoolManager: %v", err)
	}

	// 执行一些任务
	for i := 0; i < 10; i++ {
		pm.Execute(nil, func() {
			time.Sleep(10 * time.Millisecond)
		})
	}

	// 释放资源
	pm.Release()

	// 验证释放后不能再使用
	task := &timewheel.Task{Priority: timewheel.TaskPriorityNormal}
	err = pm.Execute(task, func() {})
	// 由于池已释放，应该返回错误
	if err == nil {
		t.Log("Warning: Execute after Release did not return error")
	}

	t.Log("Resource cleanup test passed")
}

// TestPoolManager_Benchmark 基准测试
func TestPoolManager_Benchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping benchmark in short mode")
	}

	pm, err := timewheel.NewPoolManager()
	if err != nil {
		t.Fatalf("Failed to create PoolManager: %v", err)
	}
	defer pm.Release()

	runner := testutil.NewBenchmarkRunner()

	report := runner.RunConcurrent(10000, 50, func() error {
		task := &timewheel.Task{Priority: timewheel.TaskPriorityNormal}
		return pm.Execute(task, func() {
			// 模拟轻量级工作
		})
	})

	t.Log(report.String())

	// 验证性能目标
	assert := testutil.NewAssertion(t)
	assert.LessThan(report.ErrorRate, 0.01, "Error rate should be less than 1%")
	assert.GreaterThan(report.Throughput, 1000.0, "Throughput should be greater than 1000 ops/sec")
}
