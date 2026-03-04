package testutil

import (
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"
)

// MetricsCollector 指标收集器
type MetricsCollector struct {
	mu           sync.Mutex
	latencies    []time.Duration
	errors       []error
	startTime    time.Time
	endTime      time.Time
	memoryBefore runtime.MemStats
	memoryAfter  runtime.MemStats
	gcCountBefore uint32
	gcCountAfter  uint32
}

// NewMetricsCollector 创建指标收集器
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		latencies: make([]time.Duration, 0),
		errors:    make([]error, 0),
	}
}

// Start 开始收集
func (c *MetricsCollector) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.startTime = time.Now()
	runtime.ReadMemStats(&c.memoryBefore)
	c.gcCountBefore = c.memoryBefore.NumGC
}

// Stop 停止收集
func (c *MetricsCollector) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.endTime = time.Now()
	runtime.ReadMemStats(&c.memoryAfter)
	c.gcCountAfter = c.memoryAfter.NumGC
}

// RecordLatency 记录延迟
func (c *MetricsCollector) RecordLatency(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latencies = append(c.latencies, d)
}

// RecordError 记录错误
func (c *MetricsCollector) RecordError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errors = append(c.errors, err)
}

// RecordOperation 记录操作
func (c *MetricsCollector) RecordOperation(start time.Time, err error) {
	latency := time.Since(start)
	c.RecordLatency(latency)
	if err != nil {
		c.RecordError(err)
	}
}

// GetLatencyStats 获取延迟统计
func (c *MetricsCollector) GetLatencyStats() LatencyStats {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.latencies) == 0 {
		return LatencyStats{}
	}

	// 复制并排序
	sorted := make([]time.Duration, len(c.latencies))
	copy(sorted, c.latencies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	var sum time.Duration
	for _, l := range sorted {
		sum += l
	}

	return LatencyStats{
		Count: len(sorted),
		Min:   sorted[0],
		Max:   sorted[len(sorted)-1],
		Mean:  sum / time.Duration(len(sorted)),
		P50:   sorted[len(sorted)*50/100],
		P90:   sorted[len(sorted)*90/100],
		P95:   sorted[len(sorted)*95/100],
		P99:   sorted[len(sorted)*99/100],
	}
}

// GetErrorRate 获取错误率
func (c *MetricsCollector) GetErrorRate() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	total := len(c.latencies) + len(c.errors)
	if total == 0 {
		return 0
	}
	return float64(len(c.errors)) / float64(total)
}

// GetThroughput 获取吞吐量（操作/秒）
func (c *MetricsCollector) GetThroughput() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	duration := c.endTime.Sub(c.startTime)
	if duration == 0 {
		return 0
	}
	totalOps := len(c.latencies) + len(c.errors)
	return float64(totalOps) / duration.Seconds()
}

// GetMemoryStats 获取内存统计
func (c *MetricsCollector) GetMemoryStats() MemoryStats {
	c.mu.Lock()
	defer c.mu.Unlock()

	return MemoryStats{
		AllocBefore:   c.memoryBefore.Alloc,
		AllocAfter:    c.memoryAfter.Alloc,
		AllocDiff:     int64(c.memoryAfter.Alloc) - int64(c.memoryBefore.Alloc),
		TotalAlloc:    c.memoryAfter.TotalAlloc,
		Sys:           c.memoryAfter.Sys,
		GCCount:       c.gcCountAfter - c.gcCountBefore,
		GCPauseTotal:  c.memoryAfter.PauseTotalNs,
		HeapObjects:   c.memoryAfter.HeapObjects,
		StackInUse:    c.memoryAfter.StackInuse,
	}
}

// GetReport 获取完整报告
func (c *MetricsCollector) GetReport() MetricsReport {
	return MetricsReport{
		Duration:      c.endTime.Sub(c.startTime),
		Latency:       c.GetLatencyStats(),
		Memory:        c.GetMemoryStats(),
		ErrorRate:     c.GetErrorRate(),
		Throughput:    c.GetThroughput(),
		TotalOps:      len(c.latencies) + len(c.errors),
		SuccessCount:  len(c.latencies),
		ErrorCount:    len(c.errors),
	}
}

// LatencyStats 延迟统计
type LatencyStats struct {
	Count int
	Min   time.Duration
	Max   time.Duration
	Mean  time.Duration
	P50   time.Duration
	P90   time.Duration
	P95   time.Duration
	P99   time.Duration
}

// String 格式化输出
func (s LatencyStats) String() string {
	return fmt.Sprintf("Count=%d, Min=%v, Max=%v, Mean=%v, P50=%v, P90=%v, P95=%v, P99=%v",
		s.Count, s.Min, s.Max, s.Mean, s.P50, s.P90, s.P95, s.P99)
}

// MemoryStats 内存统计
type MemoryStats struct {
	AllocBefore  uint64
	AllocAfter   uint64
	AllocDiff    int64
	TotalAlloc   uint64
	Sys          uint64
	GCCount      uint32
	GCPauseTotal uint64
	HeapObjects  uint64
	StackInUse   uint64
}

// String 格式化输出
func (s MemoryStats) String() string {
	return fmt.Sprintf("AllocDiff=%d bytes, GCCount=%d, HeapObjects=%d",
		s.AllocDiff, s.GCCount, s.HeapObjects)
}

// MetricsReport 指标报告
type MetricsReport struct {
	Duration     time.Duration
	Latency      LatencyStats
	Memory       MemoryStats
	ErrorRate    float64
	Throughput   float64
	TotalOps     int
	SuccessCount int
	ErrorCount   int
}

// String 格式化输出
func (r MetricsReport) String() string {
	return fmt.Sprintf(`
=== Metrics Report ===
Duration:     %v
Total Ops:    %d
Success:      %d
Errors:       %d
Error Rate:   %.2f%%
Throughput:   %.2f ops/sec

Latency:
  %s

Memory:
  %s
====================`,
		r.Duration,
		r.TotalOps,
		r.SuccessCount,
		r.ErrorCount,
		r.ErrorRate*100,
		r.Throughput,
		r.Latency.String(),
		r.Memory.String(),
	)
}

// BenchmarkRunner 基准测试运行器
type BenchmarkRunner struct {
	collector *MetricsCollector
}

// NewBenchmarkRunner 创建基准测试运行器
func NewBenchmarkRunner() *BenchmarkRunner {
	return &BenchmarkRunner{
		collector: NewMetricsCollector(),
	}
}

// RunConcurrent 并发运行测试
func (r *BenchmarkRunner) RunConcurrent(ops int, concurrency int, fn func() error) MetricsReport {
	r.collector.Start()

	var wg sync.WaitGroup
	opsPerWorker := ops / concurrency
	remainder := ops % concurrency

	for i := 0; i < concurrency; i++ {
		count := opsPerWorker
		if i < remainder {
			count++
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < count; j++ {
				start := time.Now()
				err := fn()
				r.collector.RecordOperation(start, err)
			}
		}()
	}

	wg.Wait()
	r.collector.Stop()

	return r.collector.GetReport()
}

// RunSequential 顺序运行测试
func (r *BenchmarkRunner) RunSequential(ops int, fn func() error) MetricsReport {
	r.collector.Start()

	for i := 0; i < ops; i++ {
		start := time.Now()
		err := fn()
		r.collector.RecordOperation(start, err)
	}

	r.collector.Stop()
	return r.collector.GetReport()
}

// TimeOperation 计时操作
func TimeOperation(fn func()) time.Duration {
	start := time.Now()
	fn()
	return time.Since(start)
}

// TimeOperationWithError 计时操作（带错误返回）
func TimeOperationWithError(fn func() error) (time.Duration, error) {
	start := time.Now()
	err := fn()
	return time.Since(start), err
}
