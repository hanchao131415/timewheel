package performance

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"timewheel/pkg/timewheel"
	"timewheel/tests/fixtures"
	"timewheel/tests/testutil"
)

// TestScalability_10K_Tasks_Baseline 测试 10K 任务基线
func TestScalability_10K_Tasks_Baseline(t *testing.T) {
	tw, err := timewheel.New()
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	const taskCount = 10000

	// 测量添加时间
	addStart := time.Now()
	for i := 0; i < taskCount; i++ {
		task := fixtures.NewTaskFixture().
			WithID(fmt.Sprintf("10k-%d", i)).
			WithInterval(10000). // 10s 间隔
			ToTimeWheelTask(nil)
		tw.AddTask(task)
	}
	addDuration := time.Since(addStart)

	// 测量内存
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 获取指标
	metrics := tw.GetMetrics()

	t.Logf("10K Tasks Baseline:")
	t.Logf("  Add time: %v", addDuration)
	t.Logf("  Add throughput: %.2f ops/sec", float64(taskCount)/addDuration.Seconds())
	t.Logf("  Memory: %.2f MB", float64(m.Alloc)/1024/1024)
	t.Logf("  Total tasks: %d", metrics.TotalTasks)
	t.Logf("  Goroutines: %d", runtime.NumGoroutine())

	// 验证
	assert := testutil.NewAssertion(t)
	assert.Equal(int64(taskCount), metrics.TotalTasks, "All tasks should be added")
	assert.LessThan(runtime.NumGoroutine(), 50, "Goroutine count should be reasonable")
}

// TestScalability_50K_Tasks_TickLessThan100ms 测试 50K 任务 tick 性能
func TestScalability_50K_Tasks_TickLessThan100ms(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tw, err := timewheel.New(
		timewheel.WithSlotNum(1000),
		timewheel.WithInterval(100*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	const taskCount = 50000

	// 添加任务
	for i := 0; i < taskCount; i++ {
		task := fixtures.NewTaskFixture().
			WithID(fmt.Sprintf("50k-%d", i)).
			WithInterval(10000).
			ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	// 测量 tick 处理时间（通过观察执行间隔）
	// 由于内部 tick 不暴露，我们测量操作延迟

	const testOps = 1000
	latencies := make([]time.Duration, testOps)

	for i := 0; i < testOps; i++ {
		start := time.Now()
		tw.AddTask(fixtures.NewTaskFixture().WithID(fmt.Sprintf("tick-test-%d", i)).ToTimeWheelTask(nil))
		latencies[i] = time.Since(start)
	}

	// 计算延迟统计
	var sum time.Duration
	var max time.Duration
	for _, l := range latencies {
		sum += l
		if l > max {
			max = l
		}
	}
	avg := sum / time.Duration(testOps)

	t.Logf("50K Tasks Under Load:")
	t.Logf("  Avg latency: %v", avg)
	t.Logf("  Max latency: %v", max)
	t.Logf("  Goroutines: %d", runtime.NumGoroutine())

	// 验证性能
	assert := testutil.NewAssertion(t)
	assert.LessThan(avg.Microseconds(), int64(100), "Avg latency should be < 100us")
}

// TestScalability_100K_Tasks_Stress 测试 100K 任务压力
func TestScalability_100K_Tasks_Stress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tw, err := timewheel.New(
		timewheel.WithSlotNum(2000),
		timewheel.WithCacheEnabled(true),
	)
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	const taskCount = 100000

	// 添加任务
	start := time.Now()
	var failedCount atomic.Int32
	var wg sync.WaitGroup

	batchSize := 1000
	batches := taskCount / batchSize

	for b := 0; b < batches; b++ {
		wg.Add(1)
		go func(batch int) {
			defer wg.Done()

			for i := 0; i < batchSize; i++ {
				taskID := fmt.Sprintf("100k-%d-%d", batch, i)
				task := fixtures.NewTaskFixture().
					WithID(taskID).
					WithInterval(60000).
					ToTimeWheelTask(nil)
				if err := tw.AddTask(task); err != nil {
					failedCount.Add(1)
				}
			}
		}(b)
	}

	wg.Wait()
	addDuration := time.Since(start)

	// 获取指标
	metrics := tw.GetMetrics()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	t.Logf("100K Tasks Stress Test:")
	t.Logf("  Add time: %v", addDuration)
	t.Logf("  Add throughput: %.2f ops/sec", float64(taskCount-failedCount.Load())/addDuration.Seconds())
	t.Logf("  Failed: %d", failedCount.Load())
	t.Logf("  Total tasks: %d", metrics.TotalTasks)
	t.Logf("  Memory: %.2f MB", float64(m.Alloc)/1024/1024)
	t.Logf("  Goroutines: %d", runtime.NumGoroutine())

	// 验证
	assert := testutil.NewAssertion(t)
	assert.GreaterThan(int(metrics.TotalTasks), taskCount*90/100, "At least 90% tasks should be added")
}

// TestScalability_ShardDistribution_Even 测试分片均匀分布
func TestScalability_ShardDistribution_Even(t *testing.T) {
	tw, err := timewheel.New(
		timewheel.WithShardNum(64),
	)
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	const taskCount = 10000

	// 添加任务
	for i := 0; i < taskCount; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("shard-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	// 获取分片分布
	metrics := tw.GetMetrics()
	shardDist := metrics.ShardDistribution

	// 计算分布均匀度
	var total, min, max int
	min = taskCount // 初始化为最大值
	for _, count := range shardDist {
		total += count
		if count < min {
			min = count
		}
		if count > max {
			max = count
		}
	}

	avg := float64(total) / float64(len(shardDist))
	variance := 0.0
	for _, count := range shardDist {
		variance += (float64(count) - avg) * (float64(count) - avg)
	}
	variance /= float64(len(shardDist))
	stdDev := sqrt(variance)

	t.Logf("Shard Distribution:")
	t.Logf("  Shards: %d", len(shardDist))
	t.Logf("  Total tasks: %d", total)
	t.Logf("  Average per shard: %.2f", avg)
	t.Logf("  Min: %d, Max: %d", min, max)
	t.Logf("  Std Dev: %.2f", stdDev)
	t.Logf("  Distribution: %v", shardDist[:10]) // 显示前 10 个

	// 验证分布均匀（标准差 < 平均值的 50%）
	assert := testutil.NewAssertion(t)
	acceptableStdDev := avg * 0.5
	assert.LessThan(stdDev, acceptableStdDev, "Distribution should be relatively even")
}

// TestScalability_GoroutineStability 测试 Goroutine 稳定性
func TestScalability_GoroutineStability(t *testing.T) {
	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	initialGoroutines := runtime.NumGoroutine()

	// 执行大量操作
	for i := 0; i < 10000; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("goroutine-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)

		if i%2 == 0 {
			tw.RemoveTask(fmt.Sprintf("goroutine-%d", i))
		}
	}

	// 等待稳定
	time.Sleep(500 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	goroutineGrowth := finalGoroutines - initialGoroutines

	t.Logf("Goroutine Stability:")
	t.Logf("  Initial: %d", initialGoroutines)
	t.Logf("  Final: %d", finalGoroutines)
	t.Logf("  Growth: %d", goroutineGrowth)

	// 验证 Goroutine 泄漏
	assert := testutil.NewAssertion(t)
	assert.LessThan(goroutineGrowth, 20, "Goroutine growth should be minimal")
}

// TestScalability_MemoryGrowth 测试内存增长
func TestScalability_MemoryGrowth(t *testing.T) {
	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// 添加 50K 任务
	const taskCount = 50000
	for i := 0; i < taskCount; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("mem-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	runtime.ReadMemStats(&m2)
	memoryPerTask := float64(m2.Alloc-m1.Alloc) / float64(taskCount)

	t.Logf("Memory Growth:")
	t.Logf("  Tasks: %d", taskCount)
	t.Logf("  Memory used: %.2f MB", float64(m2.Alloc-m1.Alloc)/1024/1024)
	t.Logf("  Per task: %.2f bytes", memoryPerTask)

	// 目标: 每任务 < 1KB
	assert := testutil.NewAssertion(t)
	assert.LessThan(memoryPerTask, 1024.0, "Memory per task should be < 1KB")
}

// TestScalability_ConcurrentWorkers 测试并发工作者
func TestScalability_ConcurrentWorkers(t *testing.T) {
	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	testCases := []struct {
		workers  int
		opsEach  int
	}{
		{10, 1000},
		{50, 200},
		{100, 100},
		{500, 20},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Workers_%d_Ops_%d", tc.workers, tc.opsEach), func(t *testing.T) {
			var successCount atomic.Int32
			var wg sync.WaitGroup

			start := time.Now()

			for w := 0; w < tc.workers; w++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()

					for i := 0; i < tc.opsEach; i++ {
						taskID := fmt.Sprintf("worker-%d-%d", workerID, i)
						task := fixtures.NewTaskFixture().WithID(taskID).ToTimeWheelTask(nil)
						if err := tw.AddTask(task); err == nil {
							successCount.Add(1)
						}
					}
				}(w)
			}

			wg.Wait()
			duration := time.Since(start)

			totalOps := tc.workers * tc.opsEach
			throughput := float64(successCount.Load()) / duration.Seconds()

			t.Logf("  Duration: %v", duration)
			t.Logf("  Success: %d/%d", successCount.Load(), totalOps)
			t.Logf("  Throughput: %.2f ops/sec", throughput)

			// 清理
			for w := 0; w < tc.workers; w++ {
				for i := 0; i < tc.opsEach; i++ {
					tw.RemoveTask(fmt.Sprintf("worker-%d-%d", w, i))
				}
			}
		})
	}
}

// TestScalability_LongRunning 测试长时间运行
func TestScalability_LongRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	// 运行 1 分钟，每秒添加/删除任务
	duration := 60 * time.Second
	endTime := time.Now().Add(duration)

	var addCount, removeCount atomic.Int32
	taskID := 0

	for time.Now().Before(endTime) {
		// 添加 10 个任务
		for i := 0; i < 10; i++ {
			task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("long-%d", taskID)).ToTimeWheelTask(nil)
			tw.AddTask(task)
			addCount.Add(1)
			taskID++
		}

		// 删除 5 个旧任务
		if taskID > 100 {
			for i := 0; i < 5; i++ {
				tw.RemoveTask(fmt.Sprintf("long-%d", taskID-100-i))
				removeCount.Add(1)
			}
		}

		time.Sleep(time.Second)
	}

	// 检查资源使用
	metrics := tw.GetMetrics()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	t.Logf("Long Running Test (1 min):")
	t.Logf("  Adds: %d", addCount.Load())
	t.Logf("  Removes: %d", removeCount.Load())
	t.Logf("  Final tasks: %d", metrics.TotalTasks)
	t.Logf("  Memory: %.2f MB", float64(m.Alloc)/1024/1024)
	t.Logf("  Goroutines: %d", runtime.NumGoroutine())
}

// TestScalability_ResourceLimits 测试资源限制
func TestScalability_ResourceLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	const (
		maxMemoryMB     = 100
		maxGoroutines   = 100
	)

	tw, _ := timewheel.New(
		timewheel.WithMaxConcurrentTasks(50),
	)
	tw.Start()
	defer tw.Stop()

	// 添加大量任务
	for i := 0; i < 20000; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("limit-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	// 检查资源使用
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	goroutines := runtime.NumGoroutine()

	t.Logf("Resource Limits Test:")
	t.Logf("  Memory: %.2f MB (limit: %d MB)", float64(m.Alloc)/1024/1024, maxMemoryMB)
	t.Logf("  Goroutines: %d (limit: %d)", goroutines, maxGoroutines)

	assert := testutil.NewAssertion(t)
	assert.LessThan(m.Alloc, uint64(maxMemoryMB*1024*1024), "Memory should be within limit")
	assert.LessThan(goroutines, maxGoroutines, "Goroutines should be within limit")
}

// Helper function for square root
func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}
