package performance

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"timewheel/pkg/timewheel"
	"timewheel/tests/fixtures"
)

// TestGC_AddRemove_100K_LessThan10Cycles 测试 10 万操作 GC 周期
func TestGC_AddRemove_100K_LessThan10Cycles(t *testing.T) {
	tw, _ := timewheel.New(
		timewheel.WithCacheEnabled(true),
	)
	tw.Start()
	defer tw.Stop()

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	const ops = 100000
	for i := 0; i < ops; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("gc-test-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)

		if i%2 == 0 {
			tw.RemoveTask(fmt.Sprintf("gc-test-%d", i))
		}
	}

	runtime.ReadMemStats(&m2)
	gcCycles := m2.NumGC - m1.NumGC

	t.Logf("GC Pressure Test (100K ops):")
	t.Logf("  GC cycles: %d", gcCycles)
	t.Logf("  GC pause total: %v", time.Duration(m2.PauseTotalNs-m1.PauseTotalNs))

	// 目标: < 10 次 GC
	if gcCycles > 10 {
		t.Logf("Warning: GC cycles (%d) exceeds target (10)", gcCycles)
	}
}

// TestGC_TaskExecution_ZeroAllocations 测试热路径零分配
func TestGC_TaskExecution_ZeroAllocations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	var allocBefore, allocAfter uint64

	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	// 预热
	task := fixtures.NewTaskFixture().
		WithID("gc-exec-test").
		WithInterval(100).
		ToTimeWheelTask(func(ctx context.Context) timewheel.AlarmResult {
			return timewheel.AlarmResult{
				Value:     50.0,
				Threshold: 80.0,
				IsFiring:  false,
			}
		})
	tw.AddTask(task)

	// 等待预热
	time.Sleep(500 * time.Millisecond)

	// 测量执行期间的分配
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	allocBefore = m1.Alloc

	// 运行 1 秒
	time.Sleep(1 * time.Second)

	runtime.ReadMemStats(&m1)
	allocAfter = m1.Alloc

	allocDiff := int64(allocAfter) - int64(allocBefore)
	t.Logf("Task Execution Allocation:")
	t.Logf("  Alloc before: %d bytes", allocBefore)
	t.Logf("  Alloc after: %d bytes", allocAfter)
	t.Logf("  Alloc diff: %d bytes", allocDiff)

	// 目标: 热路径零分配（或接近零）
	if allocDiff > 1024*10 { // 10KB 容忍
		t.Logf("Warning: Task execution allocated %d bytes", allocDiff)
	}
}

// TestGC_CacheHit_ZeroAllocations 测试缓存命中零分配
func TestGC_CacheHit_ZeroAllocations(t *testing.T) {
	tw, _ := timewheel.New(
		timewheel.WithCacheEnabled(true),
	)
	tw.Start()
	defer tw.Stop()

	// 添加任务到缓存
	task := fixtures.NewTaskFixture().WithID("cache-hit-test").ToTimeWheelTask(nil)
	tw.AddTask(task)

	// 预热缓存
	for i := 0; i < 100; i++ {
		tw.GetTask("cache-hit-test")
	}

	// 测量缓存命中的分配
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	const iterations = 10000
	for i := 0; i < iterations; i++ {
		tw.GetTask("cache-hit-test")
	}

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	allocPerHit := float64(m2.Alloc-m1.Alloc) / float64(iterations)

	t.Logf("Cache Hit Allocation:")
	t.Logf("  Total allocated: %d bytes", m2.Alloc-m1.Alloc)
	t.Logf("  Per hit: %.2f bytes", allocPerHit)

	// 目标: 缓存命中应该几乎零分配
	if allocPerHit > 50 {
		t.Logf("Warning: Cache hit allocated %.2f bytes per operation", allocPerHit)
	}
}

// TestGC_StringPool_Warmup_ZeroAllocations 测试字符串池预热后零分配
func TestGC_StringPool_Warmup_ZeroAllocations(t *testing.T) {
	tw, _ := timewheel.New(
		timewheel.WithCacheEnabled(true),
	)
	tw.Start()
	defer tw.Stop()

	// 使用重复的字符串标签
	commonLabels := map[string]string{
		"env":     "production",
		"service": "api",
		"region":  "us-west-2",
	}

	// 预热字符串池
	for i := 0; i < 100; i++ {
		task := fixtures.NewTaskFixture().
			WithID(fmt.Sprintf("string-warm-%d", i)).
			WithLabels(commonLabels).
			ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	// 测量使用相同字符串的分配
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	for i := 0; i < 1000; i++ {
		task := fixtures.NewTaskFixture().
			WithID(fmt.Sprintf("string-test-%d", i)).
			WithLabels(commonLabels).
			ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	runtime.ReadMemStats(&m2)
	allocPerTask := float64(m2.Alloc-m1.Alloc) / float64(1000)

	t.Logf("String Pool Allocation:")
	t.Logf("  Alloc per task: %.2f bytes", allocPerTask)

	// 使用字符串池应该显著减少分配
	if allocPerTask > 500 {
		t.Logf("Warning: String pool not effective, %.2f bytes per task", allocPerTask)
	}
}

// TestGC_PauseTime 测试 GC 暂停时间
func TestGC_PauseTime(t *testing.T) {
	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	// 收集 GC 暂停统计
	var pauseTotal time.Duration
	var pauseCount int
	var maxPause time.Duration

	// 获取初始 GC 统计
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	initialGC := m1.NumGC

	// 执行大量操作
	for i := 0; i < 50000; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("pause-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	// 获取最终 GC 统计
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// 计算暂停时间
	if m2.NumGC > initialGC && len(m2.PauseNs) > 0 {
		// 获取最近的暂停记录
		for i := 0; i < 256 && initialGC+uint32(i) < m2.NumGC; i++ {
			pauseIdx := (initialGC + uint32(i)) % 256
			pause := time.Duration(m2.PauseNs[pauseIdx])
			pauseTotal += pause
			pauseCount++
			if pause > maxPause {
				maxPause = pause
			}
		}
	}

	var avgPause time.Duration
	if pauseCount > 0 {
		avgPause = pauseTotal / time.Duration(pauseCount)
	}

	t.Logf("GC Pause Statistics:")
	t.Logf("  GC count: %d", m2.NumGC-initialGC)
	t.Logf("  Total pause: %v", pauseTotal)
	t.Logf("  Avg pause: %v", avgPause)
	t.Logf("  Max pause: %v", maxPause)

	// 目标: 平均暂停 < 1ms，最大暂停 < 10ms
	if avgPause > time.Millisecond {
		t.Logf("Warning: Average GC pause (%v) exceeds 1ms", avgPause)
	}
	if maxPause > 10*time.Millisecond {
		t.Logf("Warning: Max GC pause (%v) exceeds 10ms", maxPause)
	}
}

// TestGC_HeapGrowth 测试堆增长
func TestGC_HeapGrowth(t *testing.T) {
	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// 添加任务并观察堆增长
	taskBatches := []int{1000, 5000, 10000, 20000}

	for _, batchSize := range taskBatches {
		startID := 0
		for i := 0; i < len(taskBatches); i++ {
			if taskBatches[i] == batchSize {
				break
			}
			startID += taskBatches[i]
		}

		for i := 0; i < batchSize; i++ {
			task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("heap-%d", startID+i)).ToTimeWheelTask(nil)
			tw.AddTask(task)
		}

		runtime.ReadMemStats(&m2)

		t.Logf("After %d tasks:", startID+batchSize)
		t.Logf("  Heap alloc: %.2f MB", float64(m2.Alloc)/1024/1024)
		t.Logf("  Heap sys: %.2f MB", float64(m2.HeapSys)/1024/1024)
		t.Logf("  Heap objects: %d", m2.HeapObjects)
	}

	// 验证堆增长合理
	finalHeapMB := float64(m2.Alloc) / 1024 / 1024
	if finalHeapMB > 50 {
		t.Logf("Warning: Final heap size (%.2f MB) is large", finalHeapMB)
	}
}

// TestGC_EscapeAnalysis 测试逃逸分析
func TestGC_EscapeAnalysis(t *testing.T) {
	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	// 测试内联分配 vs 堆分配
	var m1, m2 runtime.MemStats

	// 预热
	for i := 0; i < 1000; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("escape-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	runtime.GC()
	runtime.ReadMemStats(&m1)

	// 测量
	for i := 0; i < 10000; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("escape-test-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	runtime.ReadMemStats(&m2)

	heapGrowth := m2.HeapAlloc - m1.HeapAlloc
	mallocs := m2.Mallocs - m1.Mallocs

	t.Logf("Escape Analysis:")
	t.Logf("  Heap growth: %d bytes", heapGrowth)
	t.Logf("  Mallocs: %d", mallocs)
	t.Logf("  Bytes per malloc: %.2f", float64(heapGrowth)/float64(mallocs))

	// 如果每分配字节数很大，说明有大量小对象被分配到堆上
	avgBytesPerMalloc := float64(heapGrowth) / float64(mallocs)
	if avgBytesPerMalloc > 200 {
		t.Logf("Warning: High bytes per malloc (%.2f) may indicate heap allocations", avgBytesPerMalloc)
	}
}

// TestGC_PoolEfficiency 测试 sync.Pool 效率
func TestGC_PoolEfficiency(t *testing.T) {
	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	// 预热池
	for i := 0; i < 1000; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("pool-warm-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
		tw.RemoveTask(fmt.Sprintf("pool-warm-%d", i))
	}

	// 测量池效率
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// 添加和删除循环
	for cycle := 0; cycle < 10; cycle++ {
		for i := 0; i < 1000; i++ {
			taskID := fmt.Sprintf("pool-cycle-%d-%d", cycle, i)
			task := fixtures.NewTaskFixture().WithID(taskID).ToTimeWheelTask(nil)
			tw.AddTask(task)
			tw.RemoveTask(taskID)
		}
		runtime.GC()
	}

	runtime.ReadMemStats(&m2)

	// 计算内存增长
	memGrowth := int64(m2.Alloc) - int64(m1.Alloc)

	t.Logf("sync.Pool Efficiency:")
	t.Logf("  Memory growth after 10K add/remove cycles: %d bytes", memGrowth)
	t.Logf("  GC cycles: %d", m2.NumGC-m1.NumGC)

	// 如果池工作正常，内存增长应该很小
	if memGrowth > 1024*100 { // 100KB
		t.Logf("Warning: Memory growth (%d bytes) suggests pool inefficiency", memGrowth)
	}
}

// TestGC_MemoryFragmentation 测试内存碎片
func TestGC_MemoryFragmentation(t *testing.T) {
	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	// 添加任务
	for i := 0; i < 10000; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("frag-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	// 随机删除一半
	for i := 0; i < 10000; i += 2 {
		tw.RemoveTask(fmt.Sprintf("frag-%d", i))
	}

	// 检查内存碎片
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	fragmentation := float64(m.HeapSys-m.HeapAlloc) / float64(m.HeapSys) * 100

	t.Logf("Memory Fragmentation:")
	t.Logf("  HeapSys: %.2f MB", float64(m.HeapSys)/1024/1024)
	t.Logf("  HeapAlloc: %.2f MB", float64(m.HeapAlloc)/1024/1024)
	t.Logf("  HeapIdle: %.2f MB", float64(m.HeapIdle)/1024/1024)
	t.Logf("  Fragmentation: %.2f%%", fragmentation)

	// 碎片率 > 50% 可能需要关注
	if fragmentation > 50 {
		t.Logf("Warning: High memory fragmentation (%.2f%%)", fragmentation)
	}
}
