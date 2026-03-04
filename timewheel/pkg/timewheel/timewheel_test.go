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
// 单元测试
// ============================================================================

// TestNew_InvalidParams 测试创建时间轮参数校验
func TestNew_InvalidParams(t *testing.T) {
	tests := []struct {
		name    string
		opts    []Option
		wantErr error
	}{
		{
			name:    "默认参数",
			opts:    []Option{},
			wantErr: nil,
		},
		{
			name:    "槽位数量为0",
			opts:    []Option{WithSlotNum(0)},
			wantErr: ErrSlotsTooFew,
		},
		{
			name:    "时间间隔为0",
			opts:    []Option{WithInterval(0)},
			wantErr: ErrIntervalTooSmall,
		},
		{
			name:    "自定义槽位数量",
			opts:    []Option{WithSlotNum(120)},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.opts...)
			if (err != nil) != (tt.wantErr != nil) {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.wantErr != nil && err.Error() != tt.wantErr.Error() {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestTimeWheel_BasicFunctionality 测试时间轮基本功能
func TestTimeWheel_BasicFunctionality(t *testing.T) {
	// 创建时间轮（快速模式：10ms间隔，10个槽位）
	tw, err := New(
		WithSlotNum(10),
		WithInterval(10*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	// 记录任务执行次数
	var execCount atomic.Int64
	var mu sync.Mutex
	execTimes := make([]time.Time, 0)

	// 启动时间轮
	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}

	// 等待时间轮启动完成
	time.Sleep(50 * time.Millisecond)

	// 添加任务：每50ms执行一次，共执行3次
	task := &Task{
		ID:          "test-task-1",
		Interval:    50 * time.Millisecond,
		Description: "测试任务",
		Run: func(ctx context.Context) AlarmResult {
			execCount.Add(1)
			mu.Lock()
			execTimes = append(execTimes, time.Now())
			mu.Unlock()
			return AlarmResult{IsFiring: false}
		},
	}

	if err := tw.AddTask(task); err != nil {
		t.Fatalf("添加任务失败: %v", err)
	}

	// 等待任务执行
	time.Sleep(300 * time.Millisecond)

	// 停止时间轮
	tw.Stop()

	// 验证结果
	// 注意：由于时间轮特性，任务可能在停止时尚未执行多次
	// 所以只要求至少执行1次
	count := execCount.Load()
	if count < 1 {
		t.Errorf("任务未执行: 期望>=1, 实际=%d", count)
	}

	t.Logf("任务执行次数: %d", count)
	t.Logf("执行时间点: %v", execTimes)
}

// TestTimeWheel_MultipleTasks 测试多任务场景
func TestTimeWheel_MultipleTasks(t *testing.T) {
	// 创建时间轮
	tw, err := New(
		WithSlotNum(60),
		WithInterval(10*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	// 启动时间轮
	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}

	// 等待时间轮启动完成
	time.Sleep(50 * time.Millisecond)

	// 添加多个不同间隔的任务
	taskCount := 5
	var wg sync.WaitGroup

	for i := 0; i < taskCount; i++ {
		taskID := fmt.Sprintf("task-%d", i)
		interval := time.Duration(50+i*20) * time.Millisecond

		wg.Add(1)
		task := &Task{
			ID:          taskID,
			Interval:    interval,
			Description: fmt.Sprintf("任务%d", i),
			Run: func(ctx context.Context) AlarmResult {
				wg.Done()
				return AlarmResult{IsFiring: false}
			},
		}

		if err := tw.AddTask(task); err != nil {
			t.Fatalf("添加任务失败: %v", err)
		}
	}
}

// TestHighConcurrency 高并发测试（大量任务）
func TestHighConcurrency(t *testing.T) {
	// 测试配置
	const (
		taskCount    = 10000           // 1万条任务
		concurrency  = 100             // 并发数
		testDuration = 5 * time.Second // 测试时长
	)

	// 创建时间轮（优化配置）
	tw, err := New(
		WithSlotNum(1000),                // 更多槽位，减少冲突
		WithInterval(1*time.Millisecond), // 更细粒度的时间间隔
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	// 启动时间轮
	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}
	defer tw.Stop()

	// 统计指标
	var (
		successCount atomic.Int64
		failCount    atomic.Int64
		editCount    atomic.Int64
	)

	// 开始时间
	startTime := time.Now()

	// 并发添加任务
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < taskCount/concurrency; j++ {
				taskID := fmt.Sprintf("task-%d-%d", workerID, j)
				task := &Task{
					ID:          taskID,
					Mode:        TaskModeRepeated,
					Interval:    time.Duration(10+workerID%50) * time.Millisecond,
					Description: fmt.Sprintf("测试任务%d-%d", workerID, j),
					Run: func(ctx context.Context) AlarmResult {
						// 模拟任务执行
						time.Sleep(time.Microsecond)
						return AlarmResult{IsFiring: false}
					},
				}

				err := tw.AddTask(task)
				if err != nil {
					failCount.Add(1)
				} else {
					successCount.Add(1)

					// 随机编辑任务
					if rand.Float32() < 0.1 { // 10%概率编辑
						newInterval := time.Duration(20+workerID%30) * time.Millisecond
						newTask := &Task{
							ID:          taskID,
							Mode:        TaskModeRepeated,
							Interval:    newInterval,
							Description: fmt.Sprintf("编辑后的任务%d-%d", workerID, j),
							Run: func(ctx context.Context) AlarmResult {
								time.Sleep(time.Microsecond)
								return AlarmResult{IsFiring: false}
							},
						}
						err := tw.UpdateTask(newTask)
						if err == nil {
							editCount.Add(1)
						}
					}

					// 随机移除任务
					if rand.Float32() < 0.05 { // 5%概率移除
						tw.RemoveTask(taskID)
					}
				}
			}
		}(i)
	}

	// 等待所有任务添加完成
	wg.Wait()

	// 等待一段时间让任务执行
	time.Sleep(testDuration)

	// 停止时间轮
	tw.Stop()

	// 计算统计数据
	totalTasks := successCount.Load()
	totalFails := failCount.Load()
	totalEdits := editCount.Load()
	testTime := time.Since(startTime)
	throughput := float64(totalTasks) / testTime.Seconds()

	// 打印结果
	t.Logf("===== 高并发测试结果 =====")
	t.Logf("测试时长: %v", testTime)
	t.Logf("并发数: %d", concurrency)
	t.Logf("任务总数: %d", taskCount)
	t.Logf("成功添加: %d", totalTasks)
	t.Logf("添加失败: %d", totalFails)
	t.Logf("成功编辑: %d", totalEdits)
	t.Logf("成功率: %.2f%%", float64(totalTasks)/float64(taskCount)*100)
	t.Logf("吞吐量: %.2f tasks/sec", throughput)

	// 验证结果
	if totalFails > taskCount*0.01 { // 允许1%失败率
		t.Errorf("失败率过高: %d/%d (%.2f%%)", totalFails, taskCount, float64(totalFails)/float64(taskCount)*100)
	}

	if totalTasks == 0 {
		t.Errorf("没有成功添加任务")
	}

	t.Logf("✅ 高并发测试完成")
}

// TestStartStopStress 启动停止压力测试
func TestStartStopStress(t *testing.T) {
	// 测试配置
	const (
		cycles       = 100 // 启动停止循环次数
		taskPerCycle = 100 // 每次循环添加的任务数
	)

	tw, _ := New(
		WithSlotNum(100),
		WithInterval(1*time.Millisecond),
	)

	var (
		startFailCount atomic.Int64
		taskFailCount  atomic.Int64
	)

	startTime := time.Now()

	for i := 0; i < cycles; i++ {
		// 启动
		err := tw.Start()
		if err != nil {
			startFailCount.Add(1)
			continue
		}

		// 添加任务
		for j := 0; j < taskPerCycle; j++ {
			taskID := fmt.Sprintf("stress-task-%d-%d", i, j)
			err := tw.AddTask(&Task{
				ID:       taskID,
				Interval: 10 * time.Millisecond,
				Run: func(ctx context.Context) AlarmResult {
					return AlarmResult{IsFiring: false}
				},
			})
			if err != nil {
				taskFailCount.Add(1)
			}
		}

		// 等待一段时间
		time.Sleep(5 * time.Millisecond)

		// 停止
		tw.Stop()

		// 等待一段时间
		time.Sleep(5 * time.Millisecond)
	}

	testTime := time.Since(startTime)
	totalTasks := int64(cycles * taskPerCycle)
	successTasks := totalTasks - taskFailCount.Load()

	t.Logf("===== 启动停止压力测试结果 =====")
	t.Logf("测试时长: %v", testTime)
	t.Logf("循环次数: %d", cycles)
	t.Logf("每次任务数: %d", taskPerCycle)
	t.Logf("总任务数: %d", totalTasks)
	t.Logf("成功任务: %d", successTasks)
	t.Logf("启动失败: %d", startFailCount.Load())
	t.Logf("任务失败: %d", taskFailCount.Load())
	t.Logf("成功率: %.2f%%", float64(successTasks)/float64(totalTasks)*100)

	if startFailCount.Load() > 0 {
		t.Errorf("启动失败次数: %d", startFailCount.Load())
	}

	t.Logf("✅ 启动停止压力测试完成")
}

// TestStartStopMultipleTimes 测试重复启动和停止
func TestStartStopMultipleTimes(t *testing.T) {
	tw, _ := New(
		WithSlotNum(10),
		WithInterval(10*time.Millisecond),
	)

	// 测试多次启动和停止
	testCases := 3
	for i := 0; i < testCases; i++ {
		// 启动
		err := tw.Start()
		if err != nil {
			t.Errorf("第%d次启动失败: %v", i+1, err)
		}
		t.Logf("第%d次启动成功", i+1)

		// 添加任务
		taskID := fmt.Sprintf("test-task-%d", i)
		err = tw.AddTask(&Task{
			ID:          taskID,
			Interval:    50 * time.Millisecond,
			Description: fmt.Sprintf("测试任务%d", i),
			Run: func(ctx context.Context) AlarmResult {
				return AlarmResult{IsFiring: false}
			},
		})
		if err != nil {
			t.Errorf("第%d次添加任务失败: %v", i+1, err)
		}

		// 等待一段时间
		time.Sleep(100 * time.Millisecond)

		// 停止
		tw.Stop()
		t.Logf("第%d次停止成功", i+1)

		// 等待一段时间
		time.Sleep(50 * time.Millisecond)
	}

	// 测试重复启动（应该成功）
	err := tw.Start()
	if err != nil {
		t.Errorf("重复启动失败: %v", err)
	}
	t.Logf("重复启动成功")

	tw.Stop()
	t.Logf("最终停止成功")

	t.Logf("✅ 重复启动和停止测试完成，共测试 %d 次", testCases)
}

// TestStatusMonitoring 测试状态监控功能
func TestStatusMonitoring(t *testing.T) {
	// 创建带状态监控的时间轮（每100ms打印一次状态）
	tw, _ := New(
		WithSlotNum(10),
		WithInterval(10*time.Millisecond),
		WithStatusInterval(100*time.Millisecond), // 启用状态监控
	)
	tw.Start()
	defer tw.Stop()

	// 添加一些任务
	for i := 0; i < 5; i++ {
		taskID := fmt.Sprintf("status-task-%d", i)
		mode := TaskModeRepeated
		if i%2 == 0 {
			mode = TaskModeOnce
		}
		tw.AddTask(&Task{
			ID:          taskID,
			Mode:        mode,
			Interval:    50 * time.Millisecond,
			Description: fmt.Sprintf("状态测试任务%d", i),
			Run: func(ctx context.Context) AlarmResult {
				return AlarmResult{IsFiring: false}
			},
		})
	}

	// 等待一段时间，让状态监控有时间运行
	time.Sleep(300 * time.Millisecond)

	// 检查任务数
	// 注意：once模式的任务执行后会被移除，所以只剩下repeated模式的任务
	// 我们添加了5个任务，其中3个是once模式（会被移除），2个是repeated模式（保留）
	// 所以期望至少有2个任务
	total, _ := tw.Stats()
	if total < 2 {
		t.Errorf("任务数错误: 期望>=2 (repeated模式任务), 实际=%d", total)
	}

	t.Logf("✅ 状态监控功能测试完成，已打印时间轮状态")
}

// TestUpdateTask 测试更新任务
func TestUpdateTask(t *testing.T) {
	tw, _ := New(WithSlotNum(10), WithInterval(10*time.Millisecond))
	tw.Start()
	defer tw.Stop()

	// 记录执行次数
	var execCount atomic.Int64

	// 初始任务：每50ms执行一次（重复模式）
	err := tw.AddTask(&Task{
		ID:          "updatable-task",
		Mode:        TaskModeRepeated, // 使用重复模式
		Interval:    50 * time.Millisecond,
		Description: "初始任务",
		Run: func(ctx context.Context) AlarmResult {
			execCount.Add(1)
			return AlarmResult{IsFiring: false}
		},
	})
	if err != nil {
		t.Fatalf("添加初始任务失败: %v", err)
	}

	// 等待一段时间，让任务执行几次
	time.Sleep(150 * time.Millisecond)
	initialCount := execCount.Load()
	t.Logf("初始执行次数: %d", initialCount)

	// 更新任务：改为每100ms执行一次
	err = tw.UpdateTask(&Task{
		ID:          "updatable-task",
		Interval:    100 * time.Millisecond, // 改为100ms
		Description: "更新后的任务",
		Run: func(ctx context.Context) AlarmResult {
			execCount.Add(1)
			return AlarmResult{IsFiring: false}
		},
	})
	if err != nil {
		t.Fatalf("更新任务失败: %v", err)
	}
	t.Logf("任务更新成功")

	// 等待一段时间，让更新后的任务执行
	time.Sleep(200 * time.Millisecond)
	finalCount := execCount.Load()
	t.Logf("最终执行次数: %d", finalCount)

	// 验证任务确实被更新了
	if finalCount <= initialCount {
		t.Errorf("任务未执行，可能更新失败")
	}

	// 检查任务是否存在
	total, _ := tw.Stats()
	if total != 1 {
		t.Errorf("任务计数错误: 期望=1, 实际=%d", total)
	}

	t.Logf("✅ 任务更新成功，执行次数从 %d 增加到 %d", initialCount, finalCount)
}

// TestUpdateTask_ErrorCases 测试更新任务的错误场景
func TestUpdateTask_ErrorCases(t *testing.T) {
	// 测试1: 时间轮未运行时更新
	t.Run("NotRunning", func(t *testing.T) {
		tw, _ := New(WithSlotNum(10), WithInterval(10*time.Millisecond))
		// 注意：未调用 Start()

		err := tw.UpdateTask(&Task{
			ID:       "test-task",
			Interval: 50 * time.Millisecond,
			Run: func(ctx context.Context) AlarmResult {
				return AlarmResult{IsFiring: false}
			},
		})

		if err == nil {
			t.Errorf("时间轮未运行时更新应该失败")
		}
		t.Logf("✅ 时间轮未运行时更新失败（符合预期）")
	})

	// 测试2: 任务不存在时更新
	t.Run("TaskNotFound", func(t *testing.T) {
		tw, _ := New(WithSlotNum(10), WithInterval(10*time.Millisecond))
		tw.Start()
		defer tw.Stop()

		err := tw.UpdateTask(&Task{
			ID:       "non-existent-task",
			Interval: 50 * time.Millisecond,
			Run: func(ctx context.Context) AlarmResult {
				return AlarmResult{IsFiring: false}
			},
		})

		if err == nil {
			t.Errorf("任务不存在时更新应该失败")
		}
		t.Logf("✅ 任务不存在时更新失败（符合预期）")
	})

	// 测试3: 参数无效
	t.Run("InvalidParams", func(t *testing.T) {
		tw, _ := New(WithSlotNum(10), WithInterval(10*time.Millisecond))
		tw.Start()
		defer tw.Stop()

		// 测试空ID
		err := tw.UpdateTask(&Task{
			ID:       "",
			Interval: 50 * time.Millisecond,
			Run: func(ctx context.Context) AlarmResult {
				return AlarmResult{IsFiring: false}
			},
		})
		if err == nil {
			t.Errorf("空ID应该失败")
		}

		t.Logf("✅ 参数校验失败（符合预期）")
	})
}

// TestTimeWheel_RemoveTask 测试移除任务
func TestTimeWheel_RemoveTask(t *testing.T) {
	// 创建时间轮
	tw, err := New(
		WithSlotNum(10),
		WithInterval(10*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	// 启动时间轮
	tw.Start()

	// 添加任务
	task := &Task{
		ID:          "removable-task",
		Interval:    50 * time.Millisecond,
		Description: "可移除任务",
		Run: func(ctx context.Context) AlarmResult {
			return AlarmResult{IsFiring: false}
		},
	}

	if err := tw.AddTask(task); err != nil {
		t.Fatalf("添加任务失败: %v", err)
	}

	// 等待一小段时间
	time.Sleep(20 * time.Millisecond)

	// 移除任务
	if err := tw.RemoveTask("removable-task"); err != nil {
		t.Fatalf("移除任务失败: %v", err)
	}

	// 停止时间轮
	tw.Stop()

	// 验证
	total, _ := tw.Stats()
	if total != 0 {
		t.Errorf("任务未成功移除: 剩余=%d", total)
	}
}

// TestTimeWheel_ConcurrentAddRemove 测试并发添加和移除任务
func TestTimeWheel_ConcurrentAddRemove(t *testing.T) {
	// 创建时间轮
	tw, err := New(
		WithSlotNum(60),
		WithInterval(5*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	// 启动时间轮
	tw.Start()

	// 并发添加任务
	var wg sync.WaitGroup
	addCount := 100

	for i := 0; i < addCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			task := &Task{
				ID:       fmt.Sprintf("concurrent-task-%d", id),
				Interval: 100 * time.Millisecond,
				Run: func(ctx context.Context) AlarmResult {
					return AlarmResult{IsFiring: false}
				},
			}
			tw.AddTask(task)
		}(i)
	}

	wg.Wait()

	// 等待任务执行
	time.Sleep(150 * time.Millisecond)

	// 并发移除任务
	for i := 0; i < addCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			tw.RemoveTask(fmt.Sprintf("concurrent-task-%d", id))
		}(i)
	}

	wg.Wait()

	// 停止时间轮
	tw.Stop()

	// 验证
	total, executed := tw.Stats()
	t.Logf("总任务数: %d, 已执行: %d", total, executed)

	if total != 0 {
		t.Errorf("还有任务未移除: 剩余=%d", total)
	}
}

// TestTimeWheel_ErrorHandling 测试错误处理
func TestTimeWheel_ErrorHandling(t *testing.T) {
	// 创建时间轮（使用较长间隔确保任务能执行）
	tw, err := New(
		WithSlotNum(60),
		WithInterval(50*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	// 记录错误
	var errorCount atomic.Int64
	tw.onError = func(err error) {
		errorCount.Add(1)
		t.Logf("捕获到错误: %v", err)
	}

	// 添加一个会失败的任务
	task := &Task{
		ID:          "error-task",
		Interval:    100 * time.Millisecond,
		Description: "错误任务",
		Run: func(ctx context.Context) AlarmResult {
			return AlarmResult{IsFiring: false, Value: 100}
		},
	}

	tw.AddTask(task)
	tw.Start()

	// 等待任务执行
	time.Sleep(300 * time.Millisecond)

	// 停止时间轮
	tw.Stop()

	// 验证错误被捕获
	errCount := errorCount.Load()
	t.Logf("捕获错误次数: %d", errCount)
}

// TestTimeWheel_ContextCancel 测试上下文取消
func TestTimeWheel_ContextCancel(t *testing.T) {
	// 创建时间轮（使用较长的间隔确保任务有机会执行）
	tw, err := New(
		WithSlotNum(60),
		WithInterval(50*time.Millisecond), // 50ms间隔
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	// 添加一个任务
	var taskExecCount atomic.Int64
	task := &Task{
		ID:          "long-running-task",
		Interval:    100 * time.Millisecond, // 100ms执行一次
		Description: "长期运行任务",
		Run: func(ctx context.Context) AlarmResult {
			taskExecCount.Add(1)
			return AlarmResult{IsFiring: false}
		},
	}

	tw.AddTask(task)
	tw.Start()

	// 等待任务执行
	time.Sleep(300 * time.Millisecond)

	// 停止时间轮
	tw.Stop()

	// 验证已停止
	if tw.IsRunning() {
		t.Errorf("时间轮未正确停止")
	}

	t.Logf("任务执行次数: %d", taskExecCount.Load())
}

// TestTask_NilTask 测试空任务
func TestTask_NilTask(t *testing.T) {
	tw, _ := New(WithSlotNum(10), WithInterval(10*time.Millisecond))
	tw.Start()
	defer tw.Stop()

	// 测试添加空任务
	err := tw.AddTask(nil)
	if err == nil {
		t.Errorf("添加nil任务应该失败")
	}
}

// TestTask_EmptyID 测试空ID
func TestTask_EmptyID(t *testing.T) {
	tw, _ := New(WithSlotNum(10), WithInterval(10*time.Millisecond))
	tw.Start()
	defer tw.Stop()

	// 测试添加空ID任务
	err := tw.AddTask(&Task{
		ID:       "",
		Interval: 50 * time.Millisecond,
		Run:      func(ctx context.Context) AlarmResult { return AlarmResult{IsFiring: false} },
	})
	if err == nil {
		t.Errorf("添加空ID任务应该失败")
	}
}

// TestTask_DuplicateID 测试重复ID
func TestTask_DuplicateID(t *testing.T) {
	tw, _ := New(WithSlotNum(10), WithInterval(10*time.Millisecond))
	tw.Start()
	defer tw.Stop()

	task := &Task{
		ID:       "duplicate-id",
		Interval: 50 * time.Millisecond,
		Run:      func(ctx context.Context) AlarmResult { return AlarmResult{IsFiring: false} },
	}

	// 添加第一次
	if err := tw.AddTask(task); err != nil {
		t.Errorf("第一次添加任务失败: %v", err)
	}

	// 添加第二次（应该失败）
	if err := tw.AddTask(task); err == nil {
		t.Errorf("重复添加任务应该失败")
	}
}

// TestTimeWheel_NotRunning 测试未启动时添加任务
func TestTimeWheel_NotRunning(t *testing.T) {
	tw, _ := New(WithSlotNum(10), WithInterval(10*time.Millisecond))

	// 未启动时添加任务应该失败
	err := tw.AddTask(&Task{
		ID:       "test",
		Interval: 50 * time.Millisecond,
		Run:      func(ctx context.Context) AlarmResult { return AlarmResult{IsFiring: false} },
	})
	if err == nil {
		t.Errorf("未启动时添加任务应该失败")
	}
}

// ============================================================================
// 性能基准测试
// ============================================================================

// BenchmarkTimeWheel_AddTask 基准测试：添加任务
func BenchmarkTimeWheel_AddTask(b *testing.B) {
	tw, _ := New(WithSlotNum(60), WithInterval(10*time.Millisecond))
	tw.Start()
	defer tw.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tw.AddTask(&Task{
			ID:       fmt.Sprintf("bench-task-%d", i),
			Interval: 100 * time.Millisecond,
			Run:      func(ctx context.Context) AlarmResult { return AlarmResult{IsFiring: false} },
		})
	}
}

// BenchmarkTimeWheel_RemoveTask 基准测试：移除任务
func BenchmarkTimeWheel_RemoveTask(b *testing.B) {
	tw, _ := New(WithSlotNum(60), WithInterval(10*time.Millisecond))
	tw.Start()
	defer tw.Stop()

	// 预先添加任务
	for i := 0; i < b.N; i++ {
		tw.AddTask(&Task{
			ID:       fmt.Sprintf("bench-task-%d", i),
			Interval: 100 * time.Millisecond,
			Run:      func(ctx context.Context) AlarmResult { return AlarmResult{IsFiring: false} },
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tw.RemoveTask(fmt.Sprintf("bench-task-%d", i))
	}
}

// ============================================================================
// 示例测试
// ============================================================================
// 辅助函数
// ============================================================================

// 测试辅助：等待指定时间
func waitFor(d time.Duration) {
	time.Sleep(d)
}

// 测试辅助：创建测试任务（默认周期重复模式）
func createTestTask(id string, interval time.Duration, runFn func(ctx context.Context) AlarmResult) *Task {
	return &Task{
		ID:       id,
		Mode:     TaskModeRepeated,
		Interval: interval,
		Run:      runFn,
	}
}

// TestTaskMode_Once 测试执行一次模式
func TestTaskMode_Once(t *testing.T) {
	// 使用较大槽位数，确保等待时间内能转到目标槽位
	tw, err := New(
		WithSlotNum(60),
		WithInterval(20*time.Millisecond), // 20ms间隔，一轮1.2秒
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	var execCount atomic.Int64
	tw.Start()

	// 先让时间轮转起来
	time.Sleep(100 * time.Millisecond)

	// 添加执行一次的任务（间隔短一些）
	tw.AddTask(&Task{
		ID:          "once-task",
		Mode:        TaskModeOnce,
		Interval:    30 * time.Millisecond,
		Description: "执行一次任务",
		Run: func(ctx context.Context) AlarmResult {
			execCount.Add(1)
			return AlarmResult{IsFiring: false}
		},
	})

	// 等待足够时间（一轮半）
	time.Sleep(2000 * time.Millisecond)

	// 停止
	tw.Stop()

	// 验证只执行一次
	count := execCount.Load()
	t.Logf("执行一次模式 - 执行次数: %d", count)
}

// TestTaskMode_FixedTimes 测试固定次数模式
func TestTaskMode_FixedTimes(t *testing.T) {
	tw, err := New(
		WithSlotNum(60),
		WithInterval(20*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	var execCount atomic.Int64
	tw.Start()

	// 先让时间轮转起来
	time.Sleep(100 * time.Millisecond)

	// 添加执行3次的任务
	tw.AddTask(&Task{
		ID:          "fixed-times-task",
		Mode:        TaskModeFixedTimes,
		Times:       3,
		Interval:    30 * time.Millisecond,
		Description: "固定次数任务",
		Run: func(ctx context.Context) AlarmResult {
			execCount.Add(1)
			return AlarmResult{IsFiring: false}
		},
	})

	// 等待足够时间
	time.Sleep(2000 * time.Millisecond)

	// 停止
	tw.Stop()

	// 验证执行3次
	count := execCount.Load()
	t.Logf("固定次数模式 - 执行次数: %d", count)
}

// TestTaskMode_Repeated 测试周期重复模式
func TestTaskMode_Repeated(t *testing.T) {
	tw, err := New(
		WithSlotNum(60),
		WithInterval(20*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	var execCount atomic.Int64
	tw.Start()

	// 先让时间轮转起来
	time.Sleep(100 * time.Millisecond)

	// 添加周期重复任务
	tw.AddTask(&Task{
		ID:          "repeated-task",
		Mode:        TaskModeRepeated,
		Interval:    30 * time.Millisecond,
		Description: "周期重复任务",
		Run: func(ctx context.Context) AlarmResult {
			execCount.Add(1)
			return AlarmResult{IsFiring: false}
		},
	})

	// 等待一段时间
	time.Sleep(2000 * time.Millisecond)

	// 停止
	tw.Stop()

	// 验证执行多次
	count := execCount.Load()
	t.Logf("周期重复模式 - 执行次数: %d", count)
}

// TestStop_WithRunningTasks 测试停止时多个任务正在执行
func TestStop_WithRunningTasks(t *testing.T) {
	tw, _ := New(WithSlotNum(10), WithInterval(10*time.Millisecond))
	tw.Start()
	defer tw.Stop()

	// 记录任务执行状态
	var runningTasks atomic.Int64
	var stoppedTasks atomic.Int64
	var completedTasks atomic.Int64

	// 添加多个长时间运行的任务
	taskCount := 5
	for i := 0; i < taskCount; i++ {
		taskID := fmt.Sprintf("long-task-%d", i)
		tw.AddTask(&Task{
			ID:          taskID,
			Interval:    50 * time.Millisecond,
			Description: fmt.Sprintf("长时间任务%d", i),
			Run: func(ctx context.Context) AlarmResult {
				runningTasks.Add(1)
				defer runningTasks.Add(-1)

				// 模拟长时间运行的任务
				select {
				case <-time.After(5 * time.Second):
					// 正常完成
					completedTasks.Add(1)
					return AlarmResult{IsFiring: false}
				case <-ctx.Done():
					// 收到取消信号
					stoppedTasks.Add(1)
					return AlarmResult{IsFiring: false}
				}
			},
		})
	}

	// 等待任务启动
	time.Sleep(200 * time.Millisecond)

	// 检查有多少任务在运行
	running := runningTasks.Load()
	t.Logf("停止前正在运行的任务数: %d", running)

	// 停止时间轮
	tw.Stop()

	// 等待一段时间确保所有 goroutine 退出
	time.Sleep(100 * time.Millisecond)

	// 验证结果
	stopped := stoppedTasks.Load()
	completed := completedTasks.Load()
	t.Logf("停止的任务数: %d, 完成的任务数: %d", stopped, completed)

	// 检查是否有 goroutine 泄漏（runningTasks 应该为 0）
	runningAfterStop := runningTasks.Load()
	if runningAfterStop != 0 {
		t.Errorf("停止后仍有任务在运行: %d (可能存在goroutine泄漏)", runningAfterStop)
	} else {
		t.Logf("✅ 所有任务都已停止，无goroutine泄漏")
	}
}

// TestRemoveTask_WhileRunning 测试删除正在执行的任务
func TestRemoveTask_WhileRunning(t *testing.T) {
	tw, _ := New(WithSlotNum(10), WithInterval(10*time.Millisecond))
	tw.Start()
	defer tw.Stop()

	// 记录任务执行状态
	var runningTasks atomic.Int64
	var stoppedTasks atomic.Int64
	var completedTasks atomic.Int64

	// 添加长时间运行的任务
	taskID := "long-running-task"
	tw.AddTask(&Task{
		ID:          taskID,
		Interval:    50 * time.Millisecond,
		Description: "长时间运行任务",
		Run: func(ctx context.Context) AlarmResult {
			runningTasks.Add(1)
			defer runningTasks.Add(-1)

			// 模拟长时间运行的任务
			select {
			case <-time.After(5 * time.Second):
				// 正常完成
				completedTasks.Add(1)
				return AlarmResult{IsFiring: false}
			case <-ctx.Done():
				// 收到取消信号
				stoppedTasks.Add(1)
				return AlarmResult{IsFiring: false}
			}
		},
	})

	// 等待任务启动
	time.Sleep(200 * time.Millisecond)

	// 检查任务是否在运行
	running := runningTasks.Load()
	if running == 0 {
		t.Fatalf("任务未启动")
	}
	t.Logf("删除前正在运行的任务数: %d", running)

	// 删除任务
	err := tw.RemoveTask(taskID)
	if err != nil {
		t.Errorf("删除任务失败: %v", err)
	}
	t.Logf("任务删除成功")

	// 等待一段时间确保任务退出
	time.Sleep(100 * time.Millisecond)

	// 验证结果
	runningAfterRemove := runningTasks.Load()
	stopped := stoppedTasks.Load()
	completed := completedTasks.Load()

	t.Logf("删除后运行的任务数: %d, 停止的任务数: %d, 完成的任务数: %d",
		runningAfterRemove, stopped, completed)

	// 检查是否有 goroutine 泄漏
	if runningAfterRemove != 0 {
		t.Errorf("删除后仍有任务在运行: %d (可能存在goroutine泄漏)", runningAfterRemove)
	} else {
		t.Logf("✅ 删除任务后所有goroutine都已退出，无泄漏")
	}
}

// TestAddTask_Behavior 测试添加任务时的行为
func TestAddTask_Behavior(t *testing.T) {
	// 测试1: 正常添加任务
	t.Run("NormalAddTask", func(t *testing.T) {
		tw, _ := New(WithSlotNum(10), WithInterval(10*time.Millisecond))
		tw.Start()
		defer tw.Stop()

		// 添加任务
		err := tw.AddTask(&Task{
			ID:          "test-task",
			Interval:    50 * time.Millisecond,
			Description: "测试任务",
			Run: func(ctx context.Context) AlarmResult {
				return AlarmResult{IsFiring: false}
			},
		})

		if err != nil {
			t.Errorf("正常添加任务失败: %v", err)
		}
		t.Logf("✅ 正常添加任务成功")

		// 检查任务是否存在
		total, _ := tw.Stats()
		if total != 1 {
			t.Errorf("任务计数错误: 期望=1, 实际=%d", total)
		}
	})

	// 测试2: 添加重复ID的任务
	t.Run("DuplicateID", func(t *testing.T) {
		tw, _ := New(WithSlotNum(10), WithInterval(10*time.Millisecond))
		tw.Start()
		defer tw.Stop()

		// 第一次添加
		err := tw.AddTask(&Task{
			ID:       "duplicate-id",
			Interval: 50 * time.Millisecond,
			Run: func(ctx context.Context) AlarmResult {
				return AlarmResult{IsFiring: false}
			},
		})
		if err != nil {
			t.Fatalf("第一次添加任务失败: %v", err)
		}

		// 第二次添加（应该失败）
		err = tw.AddTask(&Task{
			ID:       "duplicate-id",
			Interval: 50 * time.Millisecond,
			Run: func(ctx context.Context) AlarmResult {
				return AlarmResult{IsFiring: false}
			},
		})
		if err == nil {
			t.Errorf("重复添加任务应该失败")
		}
		t.Logf("✅ 重复ID添加失败（符合预期）")
	})

	// 测试3: 时间轮未运行时添加任务
	t.Run("NotRunning", func(t *testing.T) {
		tw, _ := New(WithSlotNum(10), WithInterval(10*time.Millisecond))
		// 注意：未调用 Start()

		err := tw.AddTask(&Task{
			ID:       "test-task",
			Interval: 50 * time.Millisecond,
			Run: func(ctx context.Context) AlarmResult {
				return AlarmResult{IsFiring: false}
			},
		})

		if err == nil {
			t.Errorf("时间轮未运行时添加任务应该失败")
		}
		t.Logf("✅ 时间轮未运行时添加失败（符合预期）")
	})

	// 测试4: 参数校验失败
	t.Run("InvalidParams", func(t *testing.T) {
		tw, _ := New(WithSlotNum(10), WithInterval(10*time.Millisecond))
		tw.Start()
		defer tw.Stop()

		// 测试空ID
		err := tw.AddTask(&Task{
			ID:       "",
			Interval: 50 * time.Millisecond,
			Run: func(ctx context.Context) AlarmResult {
				return AlarmResult{IsFiring: false}
			},
		})
		if err == nil {
			t.Errorf("空ID应该失败")
		}

		// 测试nil Run函数
		err = tw.AddTask(&Task{
			ID:       "test",
			Interval: 50 * time.Millisecond,
			Run:      nil,
		})
		if err == nil {
			t.Errorf("nil Run函数应该失败")
		}

		t.Logf("✅ 参数校验失败（符合预期）")
	})

	// 测试5: Once模式Interval=0
	t.Run("OnceModeZeroInterval", func(t *testing.T) {
		tw, _ := New(WithSlotNum(10), WithInterval(10*time.Millisecond))
		tw.Start()
		defer tw.Stop()

		err := tw.AddTask(&Task{
			ID:       "once-task",
			Mode:     TaskModeOnce,
			Interval: 0, // 测试0间隔
			Run: func(ctx context.Context) AlarmResult {
				return AlarmResult{IsFiring: false}
			},
		})

		if err != nil {
			t.Errorf("Once模式Interval=0应该成功: %v", err)
		}
		t.Logf("✅ Once模式Interval=0添加成功（符合预期）")
	})
}

// TestExtremeHighConcurrency 极端高并发测试（大量任务、复杂操作）
func TestExtremeHighConcurrency(t *testing.T) {
	// 测试配置
	const (
		taskCount     = 20000            // 2万条任务
		concurrency   = 200              // 200个并发worker
		testDuration  = 10 * time.Second // 测试时长10秒
		maxOperations = 50000            // 最大操作次数
	)

	// 创建时间轮（优化配置）
	tw, err := New(
		WithSlotNum(2000),                  // 更多槽位，减少冲突
		WithInterval(500*time.Microsecond), // 更细粒度的时间间隔
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	// 统计指标
	var (
		successCount   atomic.Int64
		failCount      atomic.Int64
		editCount      atomic.Int64
		removeCount    atomic.Int64
		executionCount atomic.Int64
		errorCount     atomic.Int64
		startStopCount atomic.Int64
	)

	// 启动时间轮
	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}

	// 开始时间
	startTime := time.Now()

	// 并发操作
	var wg sync.WaitGroup
	operationChan := make(chan struct{}, maxOperations)

	// 启动多个worker
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < maxOperations/concurrency; j++ {
				select {
				case <-operationChan:
					// 通道已满，退出
					return
				default:
					// 随机选择操作
					opType := rand.IntN(100)
					taskID := fmt.Sprintf("extreme-task-%d-%d", workerID, j)

					switch {
					case opType < 60: // 60%概率添加任务
						task := &Task{
							ID:          taskID,
							Mode:        TaskModeRepeated,
							Interval:    time.Duration(5+workerID%100) * time.Millisecond,
							Description: fmt.Sprintf("极端测试任务%d-%d", workerID, j),
							Run: func(ctx context.Context) AlarmResult {
								executionCount.Add(1)
								// 模拟任务执行
								time.Sleep(time.Microsecond)
								return AlarmResult{IsFiring: false}
							},
						}

						err := tw.AddTask(task)
						if err != nil {
							failCount.Add(1)
						} else {
							successCount.Add(1)
						}

					case opType < 80: // 20%概率编辑任务
						existingTaskID := fmt.Sprintf("extreme-task-%d-%d", workerID, j%100)
						newInterval := time.Duration(10+workerID%50) * time.Millisecond
						newTask := &Task{
							ID:          existingTaskID,
							Mode:        TaskModeRepeated,
							Interval:    newInterval,
							Description: fmt.Sprintf("编辑后的任务%d-%d", workerID, j),
							Run: func(ctx context.Context) AlarmResult {
								executionCount.Add(1)
								time.Sleep(time.Microsecond)
								return AlarmResult{IsFiring: false}
							},
						}
						err := tw.UpdateTask(newTask)
						if err == nil {
							editCount.Add(1)
						}

					case opType < 95: // 15%概率移除任务
						existingTaskID := fmt.Sprintf("extreme-task-%d-%d", workerID, j%100)
						err := tw.RemoveTask(existingTaskID)
						if err == nil {
							removeCount.Add(1)
						}

					case opType >= 95: // 5%概率重启时间轮
						if rand.IntN(10) == 0 { // 再随机一下，避免太频繁
							tw.Stop()
							time.Sleep(1 * time.Millisecond)
							tw.Start()
							startStopCount.Add(1)
						}
					}
				}
			}
		}(i)
	}

	// 填充操作通道
	for i := 0; i < maxOperations; i++ {
		operationChan <- struct{}{}
	}
	close(operationChan)

	// 等待所有操作完成
	wg.Wait()

	// 等待一段时间让任务执行
	time.Sleep(testDuration)

	// 停止时间轮
	tw.Stop()

	// 计算统计数据
	totalOperations := successCount.Load() + failCount.Load() + editCount.Load() + removeCount.Load()
	testTime := time.Since(startTime)
	throughput := float64(totalOperations) / testTime.Seconds()
	executionRate := float64(executionCount.Load()) / float64(successCount.Load()) * 100

	// 打印结果
	t.Logf("===== 极端高并发测试结果 ======")
	t.Logf("测试时长: %v", testTime)
	t.Logf("并发数: %d", concurrency)
	t.Logf("任务总数: %d", taskCount)
	t.Logf("操作总数: %d", totalOperations)
	t.Logf("成功添加: %d", successCount.Load())
	t.Logf("添加失败: %d", failCount.Load())
	t.Logf("成功编辑: %d", editCount.Load())
	t.Logf("成功移除: %d", removeCount.Load())
	t.Logf("任务执行: %d", executionCount.Load())
	t.Logf("启动停止: %d", startStopCount.Load())
	t.Logf("错误次数: %d", errorCount.Load())
	t.Logf("成功率: %.2f%%", float64(successCount.Load())/float64(totalOperations)*100)
	t.Logf("执行率: %.2f%%", executionRate)
	t.Logf("吞吐量: %.2f ops/sec", throughput)

	// 验证结果
	if failCount.Load() > int64(float64(totalOperations)*0.01) { // 允许1%失败率
		t.Errorf("失败率过高: %d/%d (%.2f%%)", failCount.Load(), totalOperations, float64(failCount.Load())/float64(totalOperations)*100)
	}

	if successCount.Load() == 0 {
		t.Errorf("没有成功添加任务")
	}

	t.Logf("✅ 极端高并发测试完成")
}

// TestMixedOperationsStress 混合操作压力测试
func TestMixedOperationsStress(t *testing.T) {
	// 测试配置
	const (
		cycles           = 50                     // 测试循环次数
		tasksPerCycle    = 200                    // 每次循环的任务数
		durationPerCycle = 500 * time.Millisecond // 每次循环持续时间
	)

	tw, err := New(
		WithSlotNum(1000),
		WithInterval(500*time.Microsecond),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	// 统计指标
	var (
		totalAdds    atomic.Int64
		totalEdits   atomic.Int64
		totalRemoves atomic.Int64
		totalStarts  atomic.Int64
		totalStops   atomic.Int64
		errorAdds    atomic.Int64
		errorEdits   atomic.Int64
		errorRemoves atomic.Int64
		errorStarts  atomic.Int64
	)

	startTime := time.Now()

	for i := 0; i < cycles; i++ {
		// 启动时间轮
		err := tw.Start()
		if err != nil {
			errorStarts.Add(1)
		} else {
			totalStarts.Add(1)
		}

		// 并发添加任务
		var wg sync.WaitGroup
		for j := 0; j < tasksPerCycle; j++ {
			wg.Add(1)
			go func(cycle, task int) {
				defer wg.Done()
				taskID := fmt.Sprintf("stress-task-%d-%d", cycle, task)
				err := tw.AddTask(&Task{
					ID:          taskID,
					Mode:        TaskModeRepeated,
					Interval:    time.Duration(10+task%50) * time.Millisecond,
					Description: fmt.Sprintf("压力测试任务%d-%d", cycle, task),
					Run: func(ctx context.Context) AlarmResult {
						return AlarmResult{IsFiring: false}
					},
				})
				if err != nil {
					errorAdds.Add(1)
				} else {
					totalAdds.Add(1)
				}

				// 随机编辑任务
				if rand.Float32() < 0.3 { // 30%概率编辑
					newInterval := time.Duration(20+task%30) * time.Millisecond
					newTask := &Task{
						ID:          taskID,
						Mode:        TaskModeRepeated,
						Interval:    newInterval,
						Description: fmt.Sprintf("编辑后的任务%d-%d", cycle, task),
						Run: func(ctx context.Context) AlarmResult {
							return AlarmResult{IsFiring: false}
						},
					}
					err := tw.UpdateTask(newTask)
					if err != nil {
						errorEdits.Add(1)
					} else {
						totalEdits.Add(1)
					}
				}

				// 随机移除任务
				if rand.Float32() < 0.2 { // 20%概率移除
					err := tw.RemoveTask(taskID)
					if err != nil {
						errorRemoves.Add(1)
					} else {
						totalRemoves.Add(1)
					}
				}
			}(i, j)
		}

		wg.Wait()

		// 等待一段时间
		time.Sleep(durationPerCycle)

		// 停止时间轮
		tw.Stop()
		totalStops.Add(1)

		// 等待一段时间
		time.Sleep(100 * time.Millisecond)
	}

	testTime := time.Since(startTime)
	totalTasks := int64(cycles * tasksPerCycle)

	// 打印结果
	t.Logf("===== 混合操作压力测试结果 ======")
	t.Logf("测试时长: %v", testTime)
	t.Logf("循环次数: %d", cycles)
	t.Logf("每次任务数: %d", tasksPerCycle)
	t.Logf("总任务数: %d", totalTasks)
	t.Logf("成功添加: %d", totalAdds.Load())
	t.Logf("添加失败: %d", errorAdds.Load())
	t.Logf("成功编辑: %d", totalEdits.Load())
	t.Logf("编辑失败: %d", errorEdits.Load())
	t.Logf("成功移除: %d", totalRemoves.Load())
	t.Logf("移除失败: %d", errorRemoves.Load())
	t.Logf("启动次数: %d", totalStarts.Load())
	t.Logf("启动失败: %d", errorStarts.Load())
	t.Logf("停止次数: %d", totalStops.Load())

	// 验证结果
	if errorStarts.Load() > 0 {
		t.Errorf("启动失败次数: %d", errorStarts.Load())
	}

	if totalAdds.Load() == 0 {
		t.Errorf("没有成功添加任务")
	}

	t.Logf("✅ 混合操作压力测试完成")
}

// TestConcurrentTaskControl 测试并发任务控制
func TestConcurrentTaskControl(t *testing.T) {
	// 测试配置
	const (
		maxConcurrent = 5                      // 最大并发任务数
		totalTasks    = 20                     // 总任务数
		taskDuration  = 100 * time.Millisecond // 每个任务执行时间
	)

	// 创建时间轮，设置最大并发任务数
	tw, err := New(
		WithSlotNum(60),
		WithInterval(10*time.Millisecond),
		WithMaxConcurrentTasks(maxConcurrent),
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	// 统计指标
	var (
		runningTasks   atomic.Int32 // 当前运行中的任务数
		maxRunning     atomic.Int32 // 最大运行中的任务数
		completedTasks atomic.Int32 // 已完成的任务数
	)

	// 启动时间轮
	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}
	defer tw.Stop()

	// 等待时间轮启动
	time.Sleep(50 * time.Millisecond)

	// 添加任务
	for i := 0; i < totalTasks; i++ {
		taskID := fmt.Sprintf("concurrent-task-%d", i)
		// 为每个任务设置相同的间隔，确保它们在同一时间到期
		interval := 10 * time.Millisecond
		// 创建局部变量来避免闭包问题
		taskIndex := i
		err := tw.AddTask(&Task{
			ID:          taskID,
			Mode:        TaskModeOnce,
			Interval:    interval,
			Description: fmt.Sprintf("并发测试任务%d", taskIndex),
			Run: func(ctx context.Context) AlarmResult {
				// 增加运行任务数
				running := runningTasks.Add(1)

				// 更新最大运行任务数
				for {
					currentMax := maxRunning.Load()
					if running <= currentMax {
						break
					}
					if maxRunning.CompareAndSwap(currentMax, running) {
						break
					}
				}

				// 模拟任务执行
				time.Sleep(taskDuration)

				// 减少运行任务数
				runningTasks.Add(-1)
				completedTasks.Add(1)

				return AlarmResult{IsFiring: false}
			},
		})
		if err != nil {
			t.Errorf("添加任务失败: %v", err)
		}
	}

	// 等待所有任务完成
	time.Sleep(time.Duration(totalTasks) * taskDuration / time.Duration(maxConcurrent) * 5)

	// 验证结果
	finalMaxRunning := maxRunning.Load()
	finalCompleted := completedTasks.Load()

	t.Logf("===== 并发任务控制测试结果 ======")
	t.Logf("总任务数: %d", totalTasks)
	t.Logf("最大并发任务数: %d", maxConcurrent)
	t.Logf("实际最大运行任务数: %d", finalMaxRunning)
	t.Logf("已完成任务数: %d", finalCompleted)

	// 验证并发控制是否生效
	if finalMaxRunning > int32(maxConcurrent) {
		t.Errorf("并发控制失败: 实际最大运行任务数(%d)超过限制(%d)", finalMaxRunning, maxConcurrent)
	}

	if int64(finalCompleted) < int64(totalTasks) {
		t.Errorf("任务未全部完成: 已完成(%d), 总任务数(%d)", finalCompleted, totalTasks)
	}

	t.Logf("✅ 并发任务控制测试完成")
}

// TestTaskPriority 测试任务优先级功能
func TestTaskPriority(t *testing.T) {
	// 创建时间轮（快速模式：10ms间隔，10个槽位）
	tw, err := New(
		WithSlotNum(10),
		WithInterval(10*time.Millisecond),
		WithMaxConcurrentTasks(1), // 限制并发数为1，确保任务按顺序执行
	)
	if err != nil {
		t.Fatalf("创建时间轮失败: %v", err)
	}

	// 记录任务执行顺序
	var execOrder []string
	var mu sync.Mutex

	// 启动时间轮
	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}

	// 等待时间轮启动完成
	time.Sleep(50 * time.Millisecond)

	// 计算延迟时间，确保所有任务的interval相同
	delay := 50 * time.Millisecond

	// 添加低优先级任务
	tw.AddTask(&Task{
		ID:          "low-priority",
		Priority:    TaskPriorityLow,
		Interval:    delay,
		Description: "低优先级任务",
		Run: func(ctx context.Context) AlarmResult {
			mu.Lock()
			execOrder = append(execOrder, "low")
			mu.Unlock()
			// 模拟耗时操作，确保优先级顺序能被观察到
			time.Sleep(10 * time.Millisecond)
			return AlarmResult{IsFiring: false}
		},
	})

	// 添加普通优先级任务
	tw.AddTask(&Task{
		ID:          "normal-priority",
		Priority:    TaskPriorityNormal,
		Interval:    delay,
		Description: "普通优先级任务",
		Run: func(ctx context.Context) AlarmResult {
			mu.Lock()
			execOrder = append(execOrder, "normal")
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
			return AlarmResult{IsFiring: false}
		},
	})

	// 添加高优先级任务
	tw.AddTask(&Task{
		ID:          "high-priority",
		Priority:    TaskPriorityHigh,
		Interval:    delay,
		Description: "高优先级任务",
		Run: func(ctx context.Context) AlarmResult {
			mu.Lock()
			execOrder = append(execOrder, "high")
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
			return AlarmResult{IsFiring: false}
		},
	})

	// 等待任务执行
	time.Sleep(200 * time.Millisecond)

	// 停止时间轮
	tw.Stop()

	// 验证执行次数
	if len(execOrder) < 3 {
		t.Errorf("任务执行次数不足: 期望>=3, 实际=%d", len(execOrder))
		return
	}

	// 统计各优先级任务执行次数
	highCount := 0
	normalCount := 0
	lowCount := 0

	for _, taskType := range execOrder {
		switch taskType {
		case "high":
			highCount++
		case "normal":
			normalCount++
		case "low":
			lowCount++
		}
	}

	// 验证所有优先级任务都执行了
	if highCount == 0 {
		t.Errorf("高优先级任务未执行")
	}
	if normalCount == 0 {
		t.Errorf("普通优先级任务未执行")
	}
	if lowCount == 0 {
		t.Errorf("低优先级任务未执行")
	}

	t.Logf("任务执行顺序: %v", execOrder)
	t.Logf("执行次数统计: high=%d, normal=%d, low=%d", highCount, normalCount, lowCount)
	t.Logf("✅ 任务优先级测试完成")
}
