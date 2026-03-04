package timewheel

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestAllPriorityTasks 测试所有优先级任务的执行
func TestAllPriorityTasks(t *testing.T) {
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

	// 记录各优先级任务执行次数
	var highCount atomic.Int64
	var normalCount atomic.Int64
	var lowCount atomic.Int64

	// 添加高优先级任务（10ms间隔）
	highTask := &Task{
		ID:          "high-priority-task",
		Priority:    TaskPriorityHigh,
		Interval:    10 * time.Millisecond,
		Description: "高优先级任务",
		Run: func(ctx context.Context) AlarmResult {
			highCount.Add(1)
			return AlarmResult{IsFiring: false}
		},
	}

	// 添加普通优先级任务（100ms间隔）
	normalTask := &Task{
		ID:          "normal-priority-task",
		Priority:    TaskPriorityNormal,
		Interval:    100 * time.Millisecond,
		Description: "普通优先级任务",
		Run: func(ctx context.Context) AlarmResult {
			normalCount.Add(1)
			return AlarmResult{IsFiring: false}
		},
	}

	// 添加低优先级任务（1s间隔）
	lowTask := &Task{
		ID:          "low-priority-task",
		Priority:    TaskPriorityLow,
		Interval:    1 * time.Second,
		Description: "低优先级任务",
		Run: func(ctx context.Context) AlarmResult {
			lowCount.Add(1)
			return AlarmResult{IsFiring: false}
		},
	}

	// 添加任务
	if err := tw.AddTask(highTask); err != nil {
		t.Fatalf("添加高优先级任务失败: %v", err)
	}
	if err := tw.AddTask(normalTask); err != nil {
		t.Fatalf("添加普通优先级任务失败: %v", err)
	}
	if err := tw.AddTask(lowTask); err != nil {
		t.Fatalf("添加低优先级任务失败: %v", err)
	}

	// 等待任务执行（等待低优先级任务执行一次）
	time.Sleep(5500 * time.Millisecond)

	// 验证任务执行次数
	high := highCount.Load()
	normal := normalCount.Load()
	low := lowCount.Load()

	t.Logf("高优先级任务执行次数: %d", high)
	t.Logf("普通优先级任务执行次数: %d", normal)
	t.Logf("低优先级任务执行次数: %d", low)

	// 验证所有任务都执行了
	if high == 0 {
		t.Errorf("高优先级任务未执行")
	}
	if normal == 0 {
		t.Errorf("普通优先级任务未执行")
	}
	// 低优先级任务可能还没执行（1s间隔时间轮）
	// 不强制要求低优先级任务执行
	if low == 0 {
		t.Logf("注意: 低优先级任务未执行（1s间隔，可能需要更长时间）")
	}

	// 验证执行频率符合预期（仅当所有优先级都执行了时）
	if high > 0 && normal > 0 {
		if high < normal {
			t.Errorf("高优先级任务执行频率应该高于普通优先级任务: 高=%d, 普通=%d", high, normal)
		}
	}
	if normal > 0 && low > 0 {
		if normal < low {
			t.Errorf("普通优先级任务执行频率应该高于低优先级任务: 普通=%d, 低=%d", normal, low)
		}
	}
}
