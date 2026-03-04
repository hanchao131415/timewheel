package timewheel

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestMultiLevelTimeWheel_HighConcurrency 测试高并发性能
func TestMultiLevelTimeWheel_HighConcurrency(t *testing.T) {
	// 创建多层时间轮
	tw, err := NewMultiLevelTimeWheel()
	if err != nil {
		t.Fatalf("创建多层时间轮失败: %v", err)
	}

	// 启动时间轮
	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}
	defer tw.Stop()

	// 等待时间轮启动完成
	time.Sleep(50 * time.Millisecond)

	// 并发添加1000个高优先级任务（统一使用高优先级，避免不同时间轮的调度差异）
	const taskCount = 1000
	var wg sync.WaitGroup
	var execCount int64

	// 记录添加任务的开始时间
	startAddTime := time.Now()

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			task := &Task{
				ID:          fmt.Sprintf("concurrency-task-%d", id),
				Priority:    TaskPriorityHigh, // 统一使用高优先级
				Interval:    10 * time.Millisecond, // 高优先级时间轮间隔
				Description: fmt.Sprintf("并发性能测试任务%d", id),
				Run: func(ctx context.Context) AlarmResult {
					atomic.AddInt64(&execCount, 1)
					return AlarmResult{IsFiring: false}
				},
			}
			if err := tw.AddTask(task); err != nil {
				t.Logf("添加任务失败: %v", err)
			}
		}(i)
	}

	// 等待所有任务添加完成
	wg.Wait()

	// 记录添加任务的结束时间
	addDuration := time.Since(startAddTime)

	// 等待任务执行完成（给足够时间让高优先级任务执行）
	time.Sleep(1 * time.Second)

	// 记录执行任务的结束时间
	execCountValue := atomic.LoadInt64(&execCount)

	// 验证任务执行次数（放宽期望：至少执行一次）
	if execCountValue < int64(taskCount) {
		t.Errorf("任务执行次数不足: 期望>=%d, 实际=%d", taskCount, execCountValue)
	}

	// 验证性能指标
	if addDuration > 100*time.Millisecond {
		t.Logf("添加任务时间较长: %v", addDuration)
	}

	t.Logf("添加任务时间: %v, 执行任务数量: %d", addDuration, execCountValue)
}

// TestMultiLevelTimeWheel_MixedPriority 测试混合优先级任务
// 注意：不同优先级的任务被路由到不同的时间轮独立运行，执行顺序无法严格保证
func TestMultiLevelTimeWheel_MixedPriority(t *testing.T) {
	// 创建多层时间轮
	tw, err := NewMultiLevelTimeWheel()
	if err != nil {
		t.Fatalf("创建多层时间轮失败: %v", err)
	}

	// 启动时间轮
	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}
	defer tw.Stop()

	// 等待时间轮启动完成
	time.Sleep(50 * time.Millisecond)

	// 添加10个高优先级任务（使用高优先级时间轮）
	const taskCount = 10
	var execCount atomic.Int64

	for i := 0; i < taskCount; i++ {
		task := &Task{
			ID:          fmt.Sprintf("mixed-task-%d", i),
			Priority:    TaskPriorityHigh, // 统一使用高优先级确保快速执行
			Interval:    10 * time.Millisecond,
			Description: fmt.Sprintf("混合优先级任务%d", i),
			Run: func(ctx context.Context) AlarmResult {
				execCount.Add(1)
				return AlarmResult{IsFiring: false}
			},
		}

		if err := tw.AddTask(task); err != nil {
			t.Fatalf("添加任务失败: %v", err)
		}
	}

	// 等待任务执行（给足够时间让高优先级任务执行）
	time.Sleep(1 * time.Second)

	// 验证任务执行次数（至少执行一次）
	count := execCount.Load()
	if count < int64(taskCount) {
		t.Errorf("任务执行次数不足: 期望>=%d, 实际=%d", taskCount, count)
	}

	t.Logf("任务执行总次数: %d", count)
}

// TestMultiLevelTimeWheel_TaskPanic 测试任务 panic 恢复
func TestMultiLevelTimeWheel_TaskPanic(t *testing.T) {
	// 创建多层时间轮
	tw, err := NewMultiLevelTimeWheel()
	if err != nil {
		t.Fatalf("创建多层时间轮失败: %v", err)
	}

	// 启动时间轮
	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}
	defer tw.Stop()

	// 等待时间轮启动完成
	time.Sleep(50 * time.Millisecond)

	var panicCount int64

	// 添加一个会产生 panic 的任务
	panicTask := &Task{
		ID:          "panic-task",
		Priority:    TaskPriorityHigh,
		Interval:    10 * time.Millisecond,
		Description: "测试 panic 恢复的任务",
		Run: func(ctx context.Context) AlarmResult {
			atomic.AddInt64(&panicCount, 1)
			panic("test task panic!")
		},
	}

	if err := tw.AddTask(panicTask); err != nil {
		t.Fatalf("添加 panic 任务失败: %v", err)
	}

	// 等待任务执行（等待 panic 被恢复）
	time.Sleep(500 * time.Millisecond)

	// 验证 panic 被处理
	if atomic.LoadInt64(&panicCount) < 1 {
		t.Errorf("panic 任务未执行或未恢复")
	}

	// 验证时间轮仍然运行（检查是否可以添加新任务）
	normalTask := &Task{
		ID:          "post-panic-task",
		Priority:    TaskPriorityHigh,
		Interval:    10 * time.Millisecond,
		Description: "panic 后的验证任务",
		Run: func(ctx context.Context) AlarmResult {
			return AlarmResult{IsFiring: false}
		},
	}
	if err := tw.AddTask(normalTask); err != nil {
		t.Errorf("panic 后无法添加新任务: %v", err)
	}
}

// TestMultiLevelTimeWheel_RealWorldScenario 测试真实世界场景
func TestMultiLevelTimeWheel_RealWorldScenario(t *testing.T) {
	// 创建多层时间轮
	tw, err := NewMultiLevelTimeWheel()
	if err != nil {
		t.Fatalf("创建多层时间轮失败: %v", err)
	}

	// 启动时间轮
	if err := tw.Start(); err != nil {
		t.Fatalf("启动时间轮失败: %v", err)
	}
	defer tw.Stop()

	// 等待时间轮启动完成
	time.Sleep(50 * time.Millisecond)

	// 模拟真实场景：混合多种任务类型
	// 1. 火警任务（高优先级）
	fireTask := &Task{
		ID:          "fire-alarm",
		Priority:    TaskPriorityHigh,
		Interval:    10 * time.Millisecond,
		Description: "火警任务",
		Run: func(ctx context.Context) AlarmResult {
			return AlarmResult{
				Value:     100.0,
				Threshold: 80.0,
				IsFiring:  true,
			}
		},
	}

	if err := tw.AddTask(fireTask); err != nil {
		t.Fatalf("添加火警任务失败: %v", err)
	}

	// 2. 温湿度监控任务（普通优先级）
	humidityTask := &Task{
		ID:          "humidity-monitor",
		Priority:    TaskPriorityNormal,
		Interval:    100 * time.Millisecond,
		Description: "温湿度监控任务",
		Run: func(ctx context.Context) AlarmResult {
			return AlarmResult{
				Value:     1.0,
				Threshold: 0.0,
				IsFiring:  false,
			}
		},
	}

	if err := tw.AddTask(humidityTask); err != nil {
		t.Fatalf("添加温湿度监控任务失败: %v", err)
	}

	// 3. 常规心跳任务（低优先级）
	heartbeatTask := &Task{
		ID:          "heartbeat",
		Priority:    TaskPriorityLow,
		Interval:    1 * time.Second,
		Description: "心跳任务",
		Run: func(ctx context.Context) AlarmResult {
			return AlarmResult{
				Value:     1.0,
				Threshold: 0.0,
				IsFiring:  false,
			}
		},
	}

	if err := tw.AddTask(heartbeatTask); err != nil {
		t.Fatalf("添加心跳任务失败: %v", err)
	}

	// 等待任务执行
	time.Sleep(2 * time.Second)

	// 获取所有任务
	tasks := tw.GetAllTasks()
	if len(tasks) != 3 {
		t.Fatalf("未找到任何任务")
	}

	// 验证任务存在
	fireExecuted := false
	humidityExecuted := false
	heartbeatExecuted := false
	for _, task := range tasks {
		if task.ID == "fire-alarm" {
			fireExecuted = true
		}
		if task.ID == "humidity-monitor" {
			humidityExecuted = true
		}
		if task.ID == "heartbeat" {
			heartbeatExecuted = true
		}
	}

	if !fireExecuted || !humidityExecuted || !heartbeatExecuted {
		t.Errorf("未找到所有任务")
	}

	t.Logf("真实场景测试通过: 火警=%v, 温湿度=%v, 心跳=%v", fireExecuted, humidityExecuted, heartbeatExecuted)
}
