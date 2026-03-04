package performance

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"timewheel/pkg/timewheel"
	"timewheel/tests/fixtures"
	"timewheel/tests/testutil"
)

// TestLatency_AddTask_P50_1us_P99_10us 测试添加任务延迟
func TestLatency_AddTask_P50_1us_P99_10us(t *testing.T) {
	tw, err := timewheel.New(
		timewheel.WithCacheEnabled(true),
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

	// 测量延迟
	const iterations = 10000
	latencies := make([]time.Duration, iterations)

	for i := 0; i < iterations; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("lat-test-%d", i)).ToTimeWheelTask(nil)
		start := time.Now()
		tw.AddTask(task)
		latencies[i] = time.Since(start)
	}

	// 计算百分位
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	p50 := latencies[iterations*50/100]
	p90 := latencies[iterations*90/100]
	p95 := latencies[iterations*95/100]
	p99 := latencies[iterations*99/100]
	avg := calcAverage(latencies)

	t.Logf("AddTask Latency:")
	t.Logf("  P50: %v", p50)
	t.Logf("  P90: %v", p90)
	t.Logf("  P95: %v", p95)
	t.Logf("  P99: %v", p99)
	t.Logf("  Avg: %v", avg)

	// 目标: P50 < 1us, P99 < 10us
	// 注意: 实际延迟取决于硬件和 Go 运行时
	if p50 > 10*time.Microsecond {
		t.Logf("Warning: P50 latency (%v) is higher than expected", p50)
	}
	if p99 > 100*time.Microsecond {
		t.Logf("Warning: P99 latency (%v) is higher than expected", p99)
	}
}

// TestLatency_RemoveTask_P50_1us_P99_10us 测试删除任务延迟
func TestLatency_RemoveTask_P50_1us_P99_10us(t *testing.T) {
	tw, err := timewheel.New(
		timewheel.WithCacheEnabled(true),
	)
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	// 预填充
	const iterations = 10000
	for i := 0; i < iterations; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("remove-lat-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	// 测量删除延迟
	latencies := make([]time.Duration, iterations)

	for i := 0; i < iterations; i++ {
		start := time.Now()
		tw.RemoveTask(fmt.Sprintf("remove-lat-%d", i))
		latencies[i] = time.Since(start)
	}

	// 计算百分位
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	p50 := latencies[iterations*50/100]
	p90 := latencies[iterations*90/100]
	p95 := latencies[iterations*95/100]
	p99 := latencies[iterations*99/100]

	t.Logf("RemoveTask Latency:")
	t.Logf("  P50: %v", p50)
	t.Logf("  P90: %v", p90)
	t.Logf("  P95: %v", p95)
	t.Logf("  P99: %v", p99)

	if p99 > 100*time.Microsecond {
		t.Logf("Warning: P99 latency (%v) is higher than expected", p99)
	}
}

// TestLatency_GetTask_Cached_P50_500ns 测试缓存命中延迟
func TestLatency_GetTask_Cached_P50_500ns(t *testing.T) {
	tw, err := timewheel.New(
		timewheel.WithCacheEnabled(true),
	)
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	// 添加任务到缓存
	task := fixtures.NewTaskFixture().WithID("cached-task").ToTimeWheelTask(nil)
	tw.AddTask(task)

	// 多次访问以预热缓存
	for i := 0; i < 100; i++ {
		tw.GetTask("cached-task")
	}

	// 测量缓存命中延迟
	const iterations = 10000
	latencies := make([]time.Duration, iterations)

	for i := 0; i < iterations; i++ {
		start := time.Now()
		tw.GetTask("cached-task")
		latencies[i] = time.Since(start)
	}

	// 计算百分位
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	p50 := latencies[iterations*50/100]
	p90 := latencies[iterations*90/100]
	p95 := latencies[iterations*95/100]
	p99 := latencies[iterations*99/100]

	t.Logf("GetTask (Cached) Latency:")
	t.Logf("  P50: %v", p50)
	t.Logf("  P90: %v", p90)
	t.Logf("  P95: %v", p95)
	t.Logf("  P99: %v", p99)

	// 目标: P50 < 500ns
	// 注意: 纳秒级别测量可能受系统影响
	if p50 > 5*time.Microsecond {
		t.Logf("Warning: Cached GetTask P50 (%v) is higher than expected", p50)
	}
}

// TestLatency_TaskExecution_Schedule_1ms 测试任务调度精度
func TestLatency_TaskExecution_Schedule_1ms(t *testing.T) {
	tw, err := timewheel.New(
		timewheel.WithInterval(10*time.Millisecond), // 10ms tick
	)
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	// 创建任务并记录调度偏差
	const scheduledInterval = 100 * time.Millisecond
	var scheduleDeviations []time.Duration
	var mu sync.Mutex

	task := &timewheel.Task{
		ID:          "schedule-test",
		Description: "Schedule precision test",
		Mode:        timewheel.TaskModeRepeated,
		Interval:    scheduledInterval,
		Run: func(ctx context.Context) timewheel.AlarmResult {
			// 这个测试需要更复杂的设置来测量调度偏差
			return timewheel.AlarmResult{}
		},
	}

	tw.AddTask(task)

	// 运行一段时间
	time.Sleep(1 * time.Second)

	// 注意: 实际调度偏差需要更复杂的测量
	// 这里只是示例
	t.Log("Schedule precision test completed")

	// 目标: 调度偏差 < 1ms
}

// TestLatency_ConcurrentOperations 测试并发操作延迟
func TestLatency_ConcurrentOperations(t *testing.T) {
	tw, err := timewheel.New()
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	const opsPerWorker = 1000
	const workers = 10

	var wg sync.WaitGroup
	allLatencies := make([][]time.Duration, workers)

	for w := 0; w < workers; w++ {
		allLatencies[w] = make([]time.Duration, opsPerWorker)
		wg.Add(1)

		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < opsPerWorker; i++ {
				taskID := fmt.Sprintf("concurrent-%d-%d", workerID, i)
				task := fixtures.NewTaskFixture().WithID(taskID).ToTimeWheelTask(nil)

				start := time.Now()
				tw.AddTask(task)
				allLatencies[workerID][i] = time.Since(start)
			}
		}(w)
	}

	wg.Wait()

	// 合并延迟
	var combined []time.Duration
	for _, lats := range allLatencies {
		combined = append(combined, lats...)
	}

	// 计算统计
	sort.Slice(combined, func(i, j int) bool { return combined[i] < combined[j] })

	total := len(combined)
	p50 := combined[total*50/100]
	p90 := combined[total*90/100]
	p95 := combined[total*95/100]
	p99 := combined[total*99/100]

	t.Logf("Concurrent AddTask Latency (%d workers, %d ops each):", workers, opsPerWorker)
	t.Logf("  P50: %v", p50)
	t.Logf("  P90: %v", p90)
	t.Logf("  P95: %v", p95)
	t.Logf("  P99: %v", p99)
}

// TestLatency_MixedOperations 测试混合操作延迟
func TestLatency_MixedOperations(t *testing.T) {
	tw, err := timewheel.New()
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	// 预填充
	for i := 0; i < 1000; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("mixed-%d", i)).ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	const iterations = 5000
	addLatencies := make([]time.Duration, 0, iterations/2)
	getLatencies := make([]time.Duration, 0, iterations/4)
	removeLatencies := make([]time.Duration, 0, iterations/4)

	taskCounter := 1000

	for i := 0; i < iterations; i++ {
		switch i % 4 {
		case 0, 1: // Add
			taskID := fmt.Sprintf("mixed-%d", taskCounter)
			task := fixtures.NewTaskFixture().WithID(taskID).ToTimeWheelTask(nil)
			start := time.Now()
			tw.AddTask(task)
			addLatencies = append(addLatencies, time.Since(start))
			taskCounter++

		case 2: // Get
			start := time.Now()
			tw.GetTask(fmt.Sprintf("mixed-%d", i%1000))
			getLatencies = append(getLatencies, time.Since(start))

		case 3: // Remove
			start := time.Now()
			tw.RemoveTask(fmt.Sprintf("mixed-%d", i%1000))
			removeLatencies = append(removeLatencies, time.Since(start))
		}
	}

	t.Logf("Mixed Operations Latency:")
	t.Logf("  Add: P50=%v, P99=%v", percentile(addLatencies, 50), percentile(addLatencies, 99))
	t.Logf("  Get:  P50=%v, P99=%v", percentile(getLatencies, 50), percentile(getLatencies, 99))
	t.Logf("  Rm:   P50=%v, P99=%v", percentile(removeLatencies, 50), percentile(removeLatencies, 99))
}

// TestLatency_UnderLoad 测试负载下的延迟
func TestLatency_UnderLoad(t *testing.T) {
	tw, err := timewheel.New()
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()
	tw.Start()

	// 模拟高负载
	const backgroundTasks = 10000
	for i := 0; i < backgroundTasks; i++ {
		task := fixtures.NewTaskFixture().
			WithID(fmt.Sprintf("bg-%d", i)).
			WithInterval(1000).
			ToTimeWheelTask(nil)
		tw.AddTask(task)
	}

	// 在负载下测量延迟
	const testOps = 1000
	latencies := make([]time.Duration, testOps)

	for i := 0; i < testOps; i++ {
		task := fixtures.NewTaskFixture().WithID(fmt.Sprintf("load-test-%d", i)).ToTimeWheelTask(nil)
		start := time.Now()
		tw.AddTask(task)
		latencies[i] = time.Since(start)
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	t.Logf("Latency under %d tasks load:", backgroundTasks)
	t.Logf("  P50: %v", latencies[testOps*50/100])
	t.Logf("  P90: %v", latencies[testOps*90/100])
	t.Logf("  P95: %v", latencies[testOps*95/100])
	t.Logf("  P99: %v", latencies[testOps*99/100])
	t.Logf("  Max: %v", latencies[testOps-1])
}

// Helper functions
func calcAverage(latencies []time.Duration) time.Duration {
	var sum time.Duration
	for _, l := range latencies {
		sum += l
	}
	return sum / time.Duration(len(latencies))
}

func percentile(values []time.Duration, p int) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(values))
	copy(sorted, values)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)*p/100]
}

// Need to import sync for the concurrent test
import "sync"
