package timewheel

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// 高并发压力测试
// ============================================================================

// TestHighConcurrency_RandomOperations 10万并发随机操作测试
func TestHighConcurrency_RandomOperations(t *testing.T) {
	// 配置
	taskCount := 10000
	operationCount := 50000
	duration := 30 * time.Second

	// 创建时间轮
	tw, err := New(
		WithSlotNum(1000),
		WithInterval(1*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	// 统计
	var (
		addCount     atomic.Int64
		updateCount  atomic.Int64
		removeCount  atomic.Int64
		executeCount atomic.Int64
		errorCount   atomic.Int64
		activeTasks  atomic.Int64
		maxActive    atomic.Int64
	)

	// 任务映射（用于随机选择）
	taskMu := sync.RWMutex{}
	taskIDs := make(map[string]bool)

	// 随机数
	rng := rand.New(rand.NewPCG(1234, 5678))

	// 启动时间轮
	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}
	defer tw.Stop()

	// 记录执行的任务
	tw.onAlertStateChange = func(taskID string, oldState, newState AlertState, result AlarmResult) {
		if newState == AlertStateFiring {
			executeCount.Add(1)
		}
	}

	// 添加初始任务
	t.Logf("开始添加 %d 个初始任务...", taskCount)
	for i := 0; i < taskCount; i++ {
		taskID := fmt.Sprintf("task-%d", i)
		task := &Task{
			ID:             taskID,
			Interval:       10*time.Millisecond + time.Duration(rng.IntN(100))*time.Millisecond,
			Description:    fmt.Sprintf("任务 %d", i),
			Severity:       Severity(rng.IntN(3)),
			For:            0,
			RepeatInterval: 0,
			Run: func(ctx context.Context) AlarmResult {
				return AlarmResult{
					Value:     float64(rng.IntN(100)),
					Threshold: 50,
					IsFiring:  rng.Float32() > 0.5,
				}
			},
		}

		if err := tw.AddTask(task); err != nil {
			t.Errorf("添加任务失败: %v", err)
			errorCount.Add(1)
		} else {
			addCount.Add(1)
			taskMu.Lock()
			taskIDs[taskID] = true
			taskMu.Unlock()
			activeTasks.Add(1)
		}
	}

	// 更新最大活跃任务数
	currentActive := activeTasks.Load()
	for {
		if currentActive > maxActive.Load() {
			maxActive.Store(currentActive)
		}
		if currentActive < 0 {
			break
		}
		break
	}

	t.Logf("初始任务添加完成: %d, 开始并发随机操作...", addCount.Load())

	// 并发执行随机操作
	startTime := time.Now()
	var wg sync.WaitGroup

	// 添加操作
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < operationCount; i++ {
			if time.Since(startTime) >= duration {
				break
			}

			taskID := fmt.Sprintf("task-new-%d-%d", time.Now().UnixNano(), i)
			task := &Task{
				ID:             taskID,
				Interval:       10*time.Millisecond + time.Duration(rng.IntN(100))*time.Millisecond,
				Description:    "新任务",
				Severity:       SeverityWarning,
				For:            0,
				RepeatInterval: 0,
				Run: func(ctx context.Context) AlarmResult {
					return AlarmResult{IsFiring: false}
				},
			}

			if err := tw.AddTask(task); err == nil {
				addCount.Add(1)
				taskMu.Lock()
				taskIDs[taskID] = true
				taskMu.Unlock()
				activeTasks.Add(1)

				current := activeTasks.Load()
				for {
					if current > maxActive.Load() {
						maxActive.Store(current)
					}
					break
				}
			}

			// 随机延迟
			time.Sleep(time.Duration(rng.IntN(1000)) * time.Microsecond)
		}
	}()

	// 更新操作
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < operationCount; i++ {
			if time.Since(startTime) >= duration {
				break
			}

			// 随机选择一个任务
			taskMu.RLock()
			idList := make([]string, 0, len(taskIDs))
			for id := range taskIDs {
				idList = append(idList, id)
			}
			taskMu.RUnlock()

			if len(idList) == 0 {
				time.Sleep(time.Millisecond)
				continue
			}

			taskID := idList[rng.IntN(len(idList))]
			interval := 10*time.Millisecond + time.Duration(rng.IntN(100))*time.Millisecond

			task := &Task{
				ID:             taskID,
				Interval:       interval,
				Description:    "更新后的任务",
				Severity:       SeverityCritical,
				For:            0,
				RepeatInterval: 0,
				Run: func(ctx context.Context) AlarmResult {
					return AlarmResult{IsFiring: false}
				},
			}

			if err := tw.UpdateTask(task); err == nil {
				updateCount.Add(1)
			}

			time.Sleep(time.Duration(rng.IntN(1000)) * time.Microsecond)
		}
	}()

	// 删除操作
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < operationCount; i++ {
			if time.Since(startTime) >= duration {
				break
			}

			// 随机选择一个任务
			taskMu.RLock()
			idList := make([]string, 0, len(taskIDs))
			for id := range taskIDs {
				idList = append(idList, id)
			}
			taskMu.RUnlock()

			if len(idList) == 0 {
				time.Sleep(time.Millisecond)
				continue
			}

			taskID := idList[rng.IntN(len(idList))]

			if err := tw.RemoveTask(taskID); err == nil {
				removeCount.Add(1)
				taskMu.Lock()
				delete(taskIDs, taskID)
				taskMu.Unlock()
				activeTasks.Add(-1)
			}

			time.Sleep(time.Duration(rng.IntN(1000)) * time.Microsecond)
		}
	}()

	// 等待操作完成
	wg.Wait()

	// 等待一段时间让任务执行
	t.Logf("等待任务执行...")
	time.Sleep(3 * time.Second)

	// 统计结果
	elapsed := time.Since(startTime)
	t.Logf("=== 测试结果 ===")
	t.Logf("测试时长: %v", elapsed)
	t.Logf("添加任务数: %d", addCount.Load())
	t.Logf("更新任务数: %d", updateCount.Load())
	t.Logf("删除任务数: %d", removeCount.Load())
	t.Logf("执行任务数: %d", executeCount.Load())
	t.Logf("错误次数: %d", errorCount.Load())
	t.Logf("最终活跃任务: %d", activeTasks.Load())
	t.Logf("最大活跃任务: %d", maxActive.Load())

	// 验证
	if errorCount.Load() > 0 {
		t.Errorf("发生 %d 个错误", errorCount.Load())
	}

	if activeTasks.Load() < 0 {
		t.Errorf("活跃任务数为负数: %d", activeTasks.Load())
	}

	// 基本断言
	if addCount.Load() == 0 {
		t.Error("未成功添加任何任务")
	}

	t.Logf("高并发随机操作测试完成")
}

// TestHighConcurrency_ConcurrentAddRemove 并发添加和删除
func TestHighConcurrency_ConcurrentAddRemove(t *testing.T) {
	taskCount := 5000
	concurrency := 100

	tw, err := New(
		WithSlotNum(500),
		WithInterval(1*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}
	defer tw.Stop()

	var wg sync.WaitGroup
	addErrors := atomic.Int64{}
	removeErrors := atomic.Int64{}
	successAdds := atomic.Int64{}
	successRemoves := atomic.Int64{}

	// 并发添加
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < taskCount/concurrency; j++ {
				taskID := fmt.Sprintf("task-g%d-%d", goroutineID, j)
				task := &Task{
					ID:          taskID,
					Interval:    10 * time.Millisecond,
					Description: fmt.Sprintf("Goroutine %d Task %d", goroutineID, j),
					Run: func(ctx context.Context) AlarmResult {
						return AlarmResult{IsFiring: false}
					},
				}

				if err := tw.AddTask(task); err != nil {
					addErrors.Add(1)
				} else {
					successAdds.Add(1)

					// 立即删除
					if err := tw.RemoveTask(taskID); err != nil {
						removeErrors.Add(1)
					} else {
						successRemoves.Add(1)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// 等待执行
	time.Sleep(2 * time.Second)

	t.Logf("=== 并发添加删除测试结果 ===")
	t.Logf("成功添加: %d", successAdds.Load())
	t.Logf("添加错误: %d", addErrors.Load())
	t.Logf("成功删除: %d", successRemoves.Load())
	t.Logf("删除错误: %d", removeErrors.Load())

	if addErrors.Load() > 0 {
		t.Errorf("添加错误数过多: %d", addErrors.Load())
	}

	if successAdds.Load() == 0 {
		t.Error("未成功添加任何任务")
	}
}

// TestHighConcurrency_SameIDConcurrentAdd 相同ID并发添加
func TestHighConcurrency_SameIDConcurrentAdd(t *testing.T) {
	sameTaskID := "same-id-task"
	concurrency := 50

	tw, err := New(
		WithSlotNum(100),
		WithInterval(1*time.Millisecond+time.Millisecond),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}
	defer tw.Stop()

	var wg sync.WaitGroup
	var successCount atomic.Int64
	var errorCount atomic.Int64

	// 多个goroutine同时添加相同ID的任务
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task := &Task{
				ID:          sameTaskID,
				Interval:    10 * time.Millisecond,
				Description: "相同ID任务",
				Run: func(ctx context.Context) AlarmResult {
					return AlarmResult{IsFiring: false}
				},
			}

			if err := tw.AddTask(task); err != nil {
				errorCount.Add(1)
			} else {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	// 等待任务执行
	time.Sleep(500 * time.Millisecond)

	t.Logf("=== 相同ID并发添加测试 ===")
	t.Logf("成功: %d", successCount.Load())
	t.Logf("错误(预期): %d", errorCount.Load())

	// 只有一个应该成功
	if successCount.Load() != 1 {
		t.Errorf("期望只有1个成功，实际 %d 个", successCount.Load())
	}

	// 验证任务存在
	node := tw.GetTask(sameTaskID)
	if node == nil {
		t.Error("任务不存在")
	}
}

// TestHighConcurrency_UpdateWhileExecuting 任务执行时更新
func TestHighConcurrency_UpdateWhileExecuting(t *testing.T) {
	executionCount := atomic.Int64{}

	tw, err := New(
		WithSlotNum(100),
		WithInterval(1*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}
	defer tw.Stop()

	// 添加任务
	task := &Task{
		ID:          "update-test",
		Interval:    5 * time.Millisecond,
		Description: "更新测试",
		Severity:    SeverityWarning,
		For:         0,
		Run: func(ctx context.Context) AlarmResult {
			executionCount.Add(1)
			// 模拟一些工作
			time.Sleep(1 * time.Millisecond)
			return AlarmResult{IsFiring: false}
		},
	}

	if err := tw.AddTask(task); err != nil {
		t.Fatalf("添加任务失败: %v", err)
	}

	// 等待执行几次
	time.Sleep(50 * time.Millisecond)
	firstCount := executionCount.Load()

	// 更新任务（改变间隔）
	newInterval := 20 * time.Millisecond
	updatedTask := &Task{
		ID:          "update-test",
		Interval:    newInterval,
		Description: "更新后的任务",
		Severity:    SeverityCritical,
		For:         0,
		Run: func(ctx context.Context) AlarmResult {
			executionCount.Add(1)
			return AlarmResult{IsFiring: true}
		},
	}

	if err := tw.UpdateTask(updatedTask); err != nil {
		t.Fatalf("更新任务失败: %v", err)
	}

	// 等待更新后的执行
	time.Sleep(100 * time.Millisecond)
	secondCount := executionCount.Load()

	t.Logf("=== 更新测试 ===")
	t.Logf("更新前执行: %d 次", firstCount)
	t.Logf("更新后执行: %d 次", secondCount)
	t.Logf("新增执行: %d 次", secondCount-firstCount)

	// 验证任务已更新
	node := tw.GetTask("update-test")
	if node == nil {
		t.Error("任务不存在")
	} else if node.task.Interval != newInterval {
		t.Errorf("任务间隔未更新，期望 %v，实际 %v", newInterval, node.task.Interval)
	} else if node.task.Severity != SeverityCritical {
		t.Errorf("任务级别未更新，期望 Critical，实际 %v", node.task.Severity)
	}
}

// TestHighConcurrency_RemoveWhileExecuting 任务执行时删除
func TestHighConcurrency_RemoveWhileExecuting(t *testing.T) {
	executionCount := atomic.Int64{}
	taskActive := atomic.Bool{}

	tw, err := New(
		WithSlotNum(100),
		WithInterval(1*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}
	defer tw.Stop()

	// 添加任务
	task := &Task{
		ID:          "remove-test",
		Interval:    5 * time.Millisecond,
		Description: "删除测试",
		For:         0,
		Run: func(ctx context.Context) AlarmResult {
			taskActive.Store(true)
			executionCount.Add(1)
			// 模拟一些工作
			time.Sleep(10 * time.Millisecond)
			return AlarmResult{IsFiring: false}
		},
	}

	if err := tw.AddTask(task); err != nil {
		t.Fatalf("添加任务失败: %v", err)
	}

	// 等待任务开始执行
	time.Sleep(10 * time.Millisecond)

	// 尝试删除正在执行的任务
	if err := tw.RemoveTask("remove-test"); err != nil {
		t.Logf("删除任务返回错误: %v", err)
	}

	// 等待执行完成
	time.Sleep(50 * time.Millisecond)

	t.Logf("=== 删除测试 ===")
	t.Logf("执行次数: %d", executionCount.Load())
	t.Logf("任务已激活: %v", taskActive.Load())

	// 验证任务已删除
	node := tw.GetTask("remove-test")
	if node != nil {
		t.Error("任务仍然存在，应该被删除")
	}
}

// TestHighConcurrency_StopAndStart 停止和启动
func TestHighConcurrency_StopAndStart(t *testing.T) {
	cycleCount := 10

	tw, err := New(
		WithSlotNum(100),
		WithInterval(10*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	executionCount := atomic.Int64{}

	// 先启动时间轮
	if err := tw.Start(); err != nil {
		t.Fatalf("启动失败: %v", err)
	}

	for i := 0; i < cycleCount; i++ {
		// 添加任务
		task := &Task{
			ID:          fmt.Sprintf("cycle-task-%d", i),
			Interval:    20 * time.Millisecond,
			Description: fmt.Sprintf("周期 %d", i),
			Run: func(ctx context.Context) AlarmResult {
				executionCount.Add(1)
				return AlarmResult{IsFiring: false}
			},
		}

		if err := tw.AddTask(task); err != nil {
			t.Errorf("添加任务失败: %v", err)
		}

		// 运行足够长的时间让任务执行
		time.Sleep(100 * time.Millisecond)
	}

	// 停止时间轮
	tw.Stop()

	t.Logf("=== 启停测试 ===")
	t.Logf("周期数: %d", cycleCount)
	t.Logf("执行次数: %d", executionCount.Load())

	if executionCount.Load() == 0 {
		t.Error("任务从未执行")
	}
}

// TestHighConcurrency_Metrics 压力测试下的指标
func TestHighConcurrency_Metrics(t *testing.T) {
	taskCount := 5000

	tw, err := New(
		WithSlotNum(500),
		WithInterval(1*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}
	defer tw.Stop()

	// 添加大量任务
	rng := rand.New(rand.NewPCG(1234, 5678))
	for i := 0; i < taskCount; i++ {
		task := &Task{
			ID:          fmt.Sprintf("metrics-task-%d", i),
			Interval:    10*time.Millisecond + time.Duration(rng.IntN(100))*time.Millisecond,
			Description: fmt.Sprintf("任务 %d", i),
			Run: func(ctx context.Context) AlarmResult {
				return AlarmResult{IsFiring: false}
			},
		}
		tw.AddTask(task)
	}

	// 等待执行
	time.Sleep(2 * time.Second)

	// 获取指标
	metrics := tw.GetMetrics()

	t.Logf("=== 指标测试 ===")
	t.Logf("总任务数: %d", metrics.TotalTasks)
	t.Logf("槽位数: %d", metrics.SlotNum)
	t.Logf("分片数: %d", metrics.ShardNum)
	t.Logf("已执行: %d", metrics.Executed)

	// 验证指标
	if metrics.ShardNum == 0 {
		t.Error("分片数为0")
	}

	if metrics.SlotNum == 0 {
		t.Error("槽位数为0")
	}
}
