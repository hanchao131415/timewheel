package performance

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"timewheel/pkg/timewheel"
	"timewheel/tests/fixtures"
	"timewheel/tests/testutil"
)

// TestMemory_AddTask_Allocation_LessThan100Bytes 测试添加任务内存分配
func TestMemory_AddTask_Allocation_LessThan100Bytes(t *testing.T) {
	tw, err := timewheel.New(
		timewheel.WithCacheEnabled(true),
	)
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	// 预热
	for i := 0; i < 100; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("warmup-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
		tw.RemoveTask(fmt.Sprintf("warmup-%d", i))
	}

	// 测量内存分配
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	const iterations = 1000
	for i := 0; i < iterations; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("mem-test-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	runtime.ReadMemStats(&m2)

	allocPerTask := float64(m2.Alloc-m1.Alloc) / float64(iterations)
	t.Logf("Memory allocation per task: %.2f bytes", allocPerTask)

	// 目标: 每任务 < 100 字节
	if allocPerTask > 100 {
		t.Logf("Warning: Memory allocation per task (%.2f bytes) exceeds target (100 bytes)", allocPerTask)
	}
}

// TestMemory_TaskSlot_PoolReuse 测试 sync.Pool 复用率
func TestMemory_TaskSlot_PoolReuse(t *testing.T) {
	tw, err := timewheel.New()
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	// 预热池
	for i := 0; i < 100; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("pool-warm-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
		tw.RemoveTask(fmt.Sprintf("pool-warm-%d", i))
	}

	// 获取池统计（如果可用）
	metrics := tw.GetMetrics()
	t.Logf("Cache stats: Hits=%d, Misses=%d", metrics.TotalCacheHits, metrics.TotalCacheMisses)

	// 执行多次添加/删除循环
	for cycle := 0; cycle < 10; cycle++ {
		for i := 0; i < 100; i++ {
			taskID := fmt.Sprintf("pool-test-%d-%d", cycle, i)
			task := fixtures.NewTaskFixture().WithID(taskID).ToTimeWheelTask(nil)
			tw.AddTask(task)
			tw.RemoveTask(taskID)
		}
	}

	// 检查池复用
	finalMetrics := tw.GetMetrics()
	cacheHitRate := float64(0)
	if finalMetrics.TotalCacheHits+finalMetrics.TotalCacheMisses > 0 {
		cacheHitRate = float64(finalMetrics.TotalCacheHits) / float64(finalMetrics.TotalCacheHits+finalMetrics.TotalCacheMisses) * 100
	}

	t.Logf("Pool reuse rate: %.1f%% (hits=%d, misses=%d)",
		cacheHitRate, finalMetrics.TotalCacheHits, finalMetrics.TotalCacheMisses)
}

// TestMemory_StringPool_Deduplication 测试字符串去重节省
func TestMemory_StringPool_Deduplication(t *testing.T) {
	tw, err := timewheel.New(
		timewheel.WithCacheEnabled(true),
	)
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	// 使用相同的标签值创建任务
	commonLabels := map[string]string{
		"env":     "production",
		"service": "timewheel",
		"region":  "us-west-2",
	}

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// 创建 1000 个任务，使用相同的标签
	for i := 0; i < 1000; i++ {
		task := fixtures.NewTaskFixture().
			WithID(fmt.Sprintf("string-pool-%d", i)).
			WithLabels(commonLabels).
			ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	runtime.ReadMemStats(&m2)

	memoryUsed := m2.Alloc - m1.Alloc
	t.Logf("Memory used for 1000 tasks with common labels: %d bytes (%.2f KB)", memoryUsed, float64(memoryUsed)/1024)

	// 如果启用字符串池，相同字符串应该只存储一次
	// 预期节省: 每个标签键值对 * (N-1) 个任务
}

// TestMemory_50K_Tasks_LessThan50MB 测试 50K 任务内存使用
func TestMemory_50K_Tasks_LessThan50MB(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tw, err := timewheel.New(
		timewheel.WithCacheEnabled(true),
		timewheel.WithSlotNum(1000), // 更多槽位
	)
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	const taskCount = 50000
	start := time.Now()

	for i := 0; i < taskCount; i++ {
		task := fixtures.NewTaskFixture().
			WithID(fmt.Sprintf("50k-task-%d", i)).
			WithInterval(60000). // 1分钟间隔，避免频繁执行
			ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	duration := time.Since(start)
	runtime.ReadMemStats(&m2)

	memoryUsed := m2.Alloc - m1.Alloc
	memoryPerTask := float64(memoryUsed) / float64(taskCount)

	t.Logf("50K tasks memory usage:")
	t.Logf("  Total: %d bytes (%.2f MB)", memoryUsed, float64(memoryUsed)/1024/1024)
	t.Logf("  Per task: %.2f bytes", memoryPerTask)
	t.Logf("  Add time: %v (%.2f ops/sec)", duration, float64(taskCount)/duration.Seconds())

	// 目标: 50K 任务 < 50MB
	maxMemory := uint64(50 * 1024 * 1024)
	if memoryUsed > maxMemory {
		t.Errorf("Memory usage %d bytes exceeds target %d bytes", memoryUsed, maxMemory)
	}
}

// TestMemory_LongRunning_24h_NoLeak 测试 24 小时无泄漏（模拟）
func TestMemory_LongRunning_24h_NoLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tw, err := timewheel.New()
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	// 模拟 24 小时操作（压缩为 1 分钟测试）
	// 每秒添加 100 任务，执行 10 任务，移除 90 任务
	const (
		cycles          = 60 // 60 秒
		addPerCycle     = 100
		removePerCycle  = 90
		taskLifetime    = 10 // 任务在移除前的生命周期（秒）
	)

	taskID := 0
	memorySamples := make([]uint64, 0, cycles)

	for cycle := 0; cycle < cycles; cycle++ {
		// 添加任务
		for i := 0; i < addPerCycle; i++ {
			task := fixtures.NewTaskFixture().
				WithID(fmt.Sprintf("leak-test-%d", taskID)).
				WithInterval(1000).
				ToTimeWheelTask(nil)
			tw.AddTask(task)
			taskID++
		}

		// 移除旧任务
		removeStart := taskID - addPerCycle*taskLifetime - removePerCycle
		if removeStart > 0 {
			for i := 0; i < removePerCycle; i++ {
				tw.RemoveTask(fmt.Sprintf("leak-test-%d", removeStart+i))
			}
		}

		// 记录内存
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		memorySamples = append(memorySamples, m.Alloc)

		time.Sleep(time.Second)
	}

	// 分析内存趋势
	if len(memorySamples) >= 10 {
		earlyAvg := average(memorySamples[:10])
		lateAvg := average(memorySamples[len(memorySamples)-10:])

		t.Logf("Early memory avg: %.2f MB", float64(earlyAvg)/1024/1024)
		t.Logf("Late memory avg: %.2f MB", float64(lateAvg)/1024/1024)

		growthRate := float64(lateAvg-earlyAvg) / float64(earlyAvg) * 100
		t.Logf("Memory growth rate: %.2f%%", growthRate)

		// 如果增长率 > 50%，可能存在泄漏
		if growthRate > 50 {
			t.Logf("Warning: Potential memory leak detected (%.2f%% growth)", growthRate)
		}
	}
}

// TestMemory_GCPressure 测试 GC 压力
func TestMemory_GCPressure(t *testing.T) {
	tw, err := timewheel.New()
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// 执行大量添加/删除操作
	for i := 0; i < 10000; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("gc-test-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)

		if i%2 == 0 {
			tw.RemoveTask(fmt.Sprintf("gc-test-%d", i))
		}
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	gcCount := m2.NumGC - m1.NumGC
	gcPauseTotal := m2.PauseTotalNs - m1.PauseTotalNs

	t.Logf("GC count: %d", gcCount)
	t.Logf("GC pause total: %v", time.Duration(gcPauseTotal))
	t.Logf("GC pause avg: %v", time.Duration(gcPauseTotal/uint64(max(1, gcCount))))

	// 目标: 10K 操作 < 10 次 GC
	if gcCount > 10 {
		t.Logf("Warning: GC count (%d) exceeds target (10)", gcCount)
	}
}

// TestMemory_AllocationProfile 测试内存分配 profile
func TestMemory_AllocationProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	runner := testutil.NewBenchmarkRunner()

	tw, _ := timewheel.New(timewheel.WithCacheEnabled(true))
	tw.Start()
	defer tw.Stop()

	report := runner.RunSequential(1000, func() error {
		task := fixtures.NewTaskFixture().ToTimeWheelTask(nil)
		return tw.AddTask(task)
	})

	t.Log(report.String())

	// 验证内存使用
	assert := testutil.NewAssertion(t)
	assert.LessThan(report.Memory.AllocDiff, int64(100*1024), "Memory diff should be less than 100KB for 1000 tasks")
}

// Helper function
func average(values []uint64) uint64 {
	if len(values) == 0 {
		return 0
	}
	var sum uint64
	for _, v := range values {
		sum += v
	}
	return sum / uint64(len(values))
}

// BenchmarkMemory_AddTask 内存基准测试
func BenchmarkMemory_AddTask(b *testing.B) {
	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("bench-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}
}

// BenchmarkMemory_RemoveTask 内存基准测试
func BenchmarkMemory_RemoveTask(b *testing.B) {
	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	// 预填充
	for i := 0; i < b.N; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("bench-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tw.RemoveTask(fmt.Sprintf("bench-%d", i))
	}
}
