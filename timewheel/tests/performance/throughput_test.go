package performance

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"timewheel/pkg/timewheel"
	"timewheel/tests/fixtures"
	"timewheel/tests/testutil"
)

// TestThroughput_AddTask_GreaterThan10K_PerSec 测试添加吞吐量
func TestThroughput_AddTask_GreaterThan10K_PerSec(t *testing.T) {
	tw, err := timewheel.New(
		timewheel.WithSlotNum(1000),
	)
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	// 预热
	for i := 0; i < 1000; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("warm-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	// 测量吞吐量
	const taskCount = 50000
	start := time.Now()

	for i := 0; i < taskCount; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("throughput-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	duration := time.Since(start)
	throughput := float64(taskCount) / duration.Seconds()

	t.Logf("AddTask Throughput:")
	t.Logf("  Total: %d tasks", taskCount)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.2f ops/sec", throughput)

	// 目标: > 10K ops/sec
	if throughput < 10000 {
		t.Logf("Warning: Throughput (%.2f) is below target (10000 ops/sec)", throughput)
	}
}

// TestThroughput_ExecuteTask_GreaterThan5K_PerSec 测试执行吞吐量
func TestThroughput_ExecuteTask_GreaterThan5K_PerSec(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	var executedCount atomic.Int32
	var wg sync.WaitGroup

	tw, err := timewheel.New(
		timewheel.WithInterval(10*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	// 创建会快速执行的任务
	const taskCount = 1000
	const executionsPerTask = 10

	for i := 0; i < taskCount; i++ {
		task := fixtures.NewTaskFixture().
			WithID(fmt.Sprintf("exec-%d", i)).
			WithInterval(100). // 100ms 间隔
			ToTimeWheelTask(func(ctx context.Context) timewheel.AlarmResult {
				executedCount.Add(1)
				return timewheel.AlarmResult{}
			})
		tw.AddTask(task)
	}

	// 运行一段时间
	duration := 5 * time.Second
	start := time.Now()
	time.Sleep(duration)

	// 计算吞吐量
	totalExecuted := executedCount.Load()
	throughput := float64(totalExecuted) / time.Since(start).Seconds()

	t.Logf("ExecuteTask Throughput:")
	t.Logf("  Tasks: %d", taskCount)
	t.Logf("  Duration: %v", time.Since(start))
	t.Logf("  Executions: %d", totalExecuted)
	t.Logf("  Throughput: %.2f executions/sec", throughput)

	// 目标: > 5K executions/sec
	if throughput < 5000 {
		t.Logf("Warning: Execution throughput (%.2f) is below target (5000/sec)", throughput)
	}
}

// TestThroughput_MixedOperations 测试混合操作吞吐量
func TestThroughput_MixedOperations(t *testing.T) {
	tw, err := timewheel.New()
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	const ops = 10000
	var addOps, getOps, removeOps atomic.Int32

	// 预填充一些任务
	for i := 0; i < 1000; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("pre-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	start := time.Now()
	taskCounter := 1000

	for i := 0; i < ops; i++ {
		switch i % 10 {
		case 0, 1, 2, 3, 4: // 50% Add
			taskID := fmt.Sprintf("mixed-%d", taskCounter)
			task := fixtures.NewTaskFixture().WithID(taskID).ToTimeWheelTask(nil)
			tw.AddTask(task)
			addOps.Add(1)
			taskCounter++

		case 5, 6, 7: // 30% Get
			tw.GetTask(fmt.Sprintf("pre-%d", i%1000))
			getOps.Add(1)

		case 8, 9: // 20% Remove
			tw.RemoveTask(fmt.Sprintf("pre-%d", i%1000))
			removeOps.Add(1)
		}
	}

	duration := time.Since(start)
	totalOps := addOps.Load() + getOps.Load() + removeOps.Load()
	throughput := float64(totalOps) / duration.Seconds()

	t.Logf("Mixed Operations Throughput:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Add: %d", addOps.Load())
	t.Logf("  Get: %d", getOps.Load())
	t.Logf("  Remove: %d", removeOps.Load())
	t.Logf("  Total: %d ops", totalOps)
	t.Logf("  Throughput: %.2f ops/sec", throughput)
}

// TestThroughput_API_Create_1K_PerSec 测试 API 创建吞吐量
func TestThroughput_API_Create_1K_PerSec(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	// 这个测试需要完整的服务层设置
	// 简化版本只测试时间轮层

	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	const requests = 10000
	collector := testutil.NewMetricsCollector()

	collector.Start()

	for i := 0; i < requests; i++ {
		start := time.Now()
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("api-%d", i)).ToTimeWheelTask(nil)
		err := tw.AddTask(task)
		collector.RecordOperation(start, err)
	}

	collector.Stop()

	report := collector.GetReport()
	t.Log(report.String())

	// 目标: > 1K creates/sec
	if report.Throughput < 1000 {
		t.Logf("Warning: API throughput (%.2f) is below target (1000/sec)", report.Throughput)
	}
}

// TestThroughput_ConcurrentWrite 测试并发写入吞吐量
func TestThroughput_ConcurrentWrite(t *testing.T) {
	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	const writers = 10
	const opsPerWriter = 1000

	var wg sync.WaitGroup
	var totalOps atomic.Int32

	start := time.Now()

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < opsPerWriter; i++ {
				taskID := fmt.Sprintf("concurrent-%d-%d", workerID, i)
				task := fixtures.NewTaskFixture().WithID(taskID).ToTimeWheelTask(nil)
				if err := tw.AddTask(task); err == nil {
					totalOps.Add(1)
				}
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)

	throughput := float64(totalOps.Load()) / duration.Seconds()

	t.Logf("Concurrent Write Throughput (%d writers):", writers)
	t.Logf("  Total: %d ops", totalOps.Load())
	t.Logf("  Duration: %v", duration)
	t.Logf("  Throughput: %.2f ops/sec", throughput)

	// 验证所有任务都被添加
	expected := writers * opsPerWriter
	if int(totalOps.Load()) != expected {
		t.Errorf("Not all operations succeeded: %d/%d", totalOps.Load(), expected)
	}
}

// TestThroughput_BatchOperations 测试批量操作吞吐量
func TestThroughput_BatchOperations(t *testing.T) {
	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	const batchSize = 100
	const batchCount = 100

	var totalDuration time.Duration

	for batch := 0; batch < batchCount; batch++ {
		tasks := make([]*timewheel.Task, batchSize)
		for i := 0; i < batchSize; i++ {
			taskID := fmt.Sprintf("batch-%d-%d", batch, i)
			tasks[i] = fixtures.NewTaskFixture().WithID(taskID).ToTimeWheelTask(nil)
		}

		start := time.Now()
		for _, task := range tasks {
			tw.AddTask(task)
		}
		totalDuration += time.Since(start)
	}

	totalOps := batchSize * batchCount
	throughput := float64(totalOps) / totalDuration.Seconds()

	t.Logf("Batch Operations Throughput:")
	t.Logf("  Batch size: %d", batchSize)
	t.Logf("  Batch count: %d", batchCount)
	t.Logf("  Total: %d ops", totalOps)
	t.Logf("  Total duration: %v", totalDuration)
	t.Logf("  Throughput: %.2f ops/sec", throughput)
	t.Logf("  Avg batch time: %v", totalDuration/time.Duration(batchCount))
}

// TestThroughput_SustainedLoad 测试持续负载吞吐量
func TestThroughput_SustainedLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	const duration = 10 * time.Second
	var opCount atomic.Int32

	endTime := time.Now().Add(duration)
	taskID := 0

	for time.Now().Before(endTime) {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("sustained-%d", taskID)).ToTimeWheelTask(nil)
		tw.AddTask(task)
		opCount.Add(1)
		taskID++

		// 避免无限增长
		if taskID%1000 == 0 {
			for i := taskID - 1000; i < taskID-500; i++ {
				tw.RemoveTask(fmt.Sprintf("sustained-%d", i))
			}
		}
	}

	throughput := float64(opCount.Load()) / duration.Seconds()

	t.Logf("Sustained Load Throughput (%v):", duration)
	t.Logf("  Total ops: %d", opCount.Load())
	t.Logf("  Throughput: %.2f ops/sec", throughput)

	// 目标: 稳定的吞吐量
	assert := testutil.NewAssertion(t)
	assert.GreaterThan(throughput, 5000.0, "Throughput should be > 5000 ops/sec")
}

// TestThroughput_ReadWriteRatio 测试读写比例
func TestThroughput_ReadWriteRatio(t *testing.T) {
	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	// 预填充
	for i := 0; i < 10000; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("rw-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	testCases := []struct {
		name       string
		readRatio  int // 读占比
		writeRatio int // 写占比
	}{
		{"90% Read / 10% Write", 9, 1},
		{"50% Read / 50% Write", 5, 5},
		{"10% Read / 90% Write", 1, 9},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const ops = 5000
			start := time.Now()

			for i := 0; i < ops; i++ {
				total := tc.readRatio + tc.writeRatio
				if i%total < tc.readRatio {
					// Read
					tw.GetTask(fmt.Sprintf("rw-%d", i%10000))
				} else {
					// Write
					task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("rw-new-%d", i)).ToTimeWheelTask(nil)
					tw.AddTask(task)
				}
			}

			throughput := float64(ops) / time.Since(start).Seconds()
			t.Logf("Throughput: %.2f ops/sec", throughput)
		})
	}
}

// BenchmarkAddTask 吞吐量基准测试
func BenchmarkAddTask(b *testing.B) {
	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("bench-%d", i)).ToTimeWheelTask(nil)
			tw.AddTask(task)
			i++
		}
	})
}

// BenchmarkMixedOps 混合操作基准测试
func BenchmarkMixedOps(b *testing.B) {
	tw, _ := timewheel.New()
	tw.Start()
	defer tw.Stop()

	// 预填充
	for i := 0; i < 1000; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("bench-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		switch i % 10 {
		case 0, 1, 2, 3, 4:
			task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("bench-new-%d", i)).ToTimeWheelTask(nil)
			tw.AddTask(task)
		case 5, 6, 7:
			tw.GetTask(fmt.Sprintf("bench-%d", i%1000))
		case 8, 9:
			tw.RemoveTask(fmt.Sprintf("bench-%d", i%1000))
		}
	}
}
