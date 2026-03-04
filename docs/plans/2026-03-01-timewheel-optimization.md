# Timewheel 高并发优化实施计划

> **For Claude:** REQUIRED SUB-SILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将时间轮优化为支持万级并发的生产级组件

**Architecture:** 采用分片锁(Shard Lock)替代全局锁，减少锁竞争；添加Prometheus监控指标

**Tech Stack:** Go 1.25, Prometheus client, sync.RWMutex

---

## Task 1: 添加分片锁 (Shard Lock)

**Files:**
- Modify: `timewheel/pkg/timewheel/timewheel.go:113-145`

**Step 1: 添加分片锁结构体**

```go
// TaskMapShard 分片锁结构
type TaskMapShard struct {
    mu      sync.RWMutex
    tasks   map[string]*taskSlot
}

// TimeWheel 添加分片
type TimeWheel struct {
    // ... existing fields ...
    shards    []TaskMapShard  // 分片数组
    shardNum  int            // 分片数量
}
```

**Step 2: Run test to verify it compiles**

Run: `go build ./...`
Expected: PASS

**Step 3: 实现分片读写方法**

```go
// GetTask 获取任务
func (tw *TimeWheel) GetTask(id string) *taskSlot {
    shard := tw.getShard(id)
    shard.mu.RLock()
    defer shard.mu.RUnlock()
    return shard.tasks[id]
}

// AddTask 添加任务
func (tw *TimeWheel) AddTaskToShard(id string, node *taskSlot) {
    shard := tw.getShard(id)
    shard.mu.Lock()
    defer shard.mu.Unlock()
    shard.tasks[id] = node
}

// RemoveTask 移除任务
func (tw *TimeWheel) RemoveTaskFromShard(id string) *taskSlot {
    shard := tw.getShard(id)
    shard.mu.Lock()
    defer shard.mu.Unlock()
    node := shard.tasks[id]
    delete(shard.tasks, id)
    return node
}
```

**Step 4: Run test to verify it passes**

Run: `go test -run TestTimeWheel_BasicFunctionality -v`
Expected: PASS

**Step 5: Commit**

```bash
git add timewheel/pkg/timewheel/timewheel.go
git commit -m "perf: add shard lock structure for high concurrency"
```

---

## Task 2: 替换全局锁为分片锁

**Files:**
- Modify: `timewheel/pkg/timewheel/timewheel.go` (AddTask, RemoveTask, UpdateTask, Stats)

**Step 1: 修改 New() 初始化分片**

```go
// 在 New() 函数中添加
const DefaultShardNum = 64 // 分片数量
tw.shardNum = DefaultShardNum
tw.shards = make([]TaskMapShard, DefaultShardNum)
for i := 0; i < DefaultShardNum; i++ {
    tw.shards[i].tasks = make(map[string]*taskSlot, 16)
}
```

**Step 2: 添加 getShard 方法**

```go
func (tw *TimeWheel) getShard(taskID string) *TaskMapShard {
    h := fnv64(taskID)
    return &tw.shards[h % uint64(tw.shardNum)]
}

func fnv64(key string) uint64 {
    hash := uint64(2166136261)
    for i := 0; i < len(key); i++ {
        hash ^= uint64(key[i])
        hash *= 16777619
    }
    return hash
}
```

**Step 3: 修改 AddTask 使用分片锁**

修改 `AddTask` 函数，将 `tw.taskMap[task.ID] = node` 改为:
```go
shard := tw.getShard(task.ID)
shard.mu.Lock()
shard.tasks[task.ID] = node
// 更新槽位链表...
shard.mu.Unlock()
```

**Step 4: Run test to verify it passes**

Run: `go test -run "TestAddTask|TestRemoveTask|TestUpdateTask" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add timewheel/pkg/timewheel/timewheel.go
git commit -m "perf: replace global lock with shard locks"
```

---

## Task 3: 添加生产级日志控制

**Files:**
- Modify: `timewheel/pkg/timewheel/timewheel.go:178-189`

**Step 1: 添加日志级别选项**

```go
// LogLevel 日志级别
type LogLevel int

const (
    LogLevelDebug LogLevel = iota
    LogLevelInfo
    LogLevelWarn
    LogLevelError
)

// WithLogLevel 设置日志级别
func WithLogLevel(level LogLevel) Option {
    return func(tw *TimeWheel) {
        tw.logLevel = level
    }
}
```

**Step 2: 修改日志输出**

```go
func (tw *TimeWheel) debug(format string, args ...interface{}) {
    if tw.logLevel <= LogLevelDebug {
        tw.logger.Printf("[DEBUG] "+format, args...)
    }
}

func (tw *TimeWheel) info(format string, args ...interface{}) {
    if tw.logLevel <= LogLevelInfo {
        tw.logger.Printf("[INFO] "+format, args...)
    }
}
```

**Step 3: 替换所有 debug 日志调用**

将 `tw.logger.Printf("[DEBUG]..."` 替换为 `tw.debug("...")`

**Step 4: Run test to verify it passes**

Run: `go test -run TestNew_InvalidParams -v`
Expected: PASS

**Step 5: Commit**

```bash
git add timewheel/pkg/timewheel/timewheel.go
git commit -m "perf: add log level control for production"
```

---

## Task 4: 添加 Prometheus Metrics

**Files:**
- Modify: `timewheel/pkg/timewheel/timewheel.go`
- Create: `timewheel/pkg/timewheel/metrics.go`

**Step 1: 创建 metrics.go**

```go
package timewheel

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    tasksTotal = promauto.NewCounter(prometheus.CounterOpts{
        Name: "timewheel_tasks_total",
        Help: "Total number of tasks executed",
    })
    tasksRunning = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "timewheel_tasks_running",
        Help: "Number of tasks currently running",
    })
    tasksQueued = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "timewheel_tasks_queued",
        Help: "Number of tasks in queue",
    })
)
```

**Step 2: 在 executeTask 中添加指标**

```go
func (tw *TimeWheel) executeTask(node *taskSlot, ...) {
    tasksRunning.Inc()
    defer tasksRunning.Dec()
    
    // ... existing code ...
    tasksTotal.Inc()
}
```

**Step 3: 添加 metrics 端点选项**

```go
// WithPrometheusMetrics 启用Prometheus指标
func WithPrometheusMetrics(registry *prometheus.Registry) Option {
    return func(tw *TimeWheel) {
        if registry != nil {
            registry.MustRegister(tasksTotal, tasksRunning, tasksQueued)
        }
    }
}
```

**Step 4: Run test to verify it compiles**

Run: `go build ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add timewheel/pkg/timewheel/metrics.go timewheel/pkg/timewheel/timewheel.go
git commit -m "feat: add Prometheus metrics support"
```

---

## Task 5: 添加任务超时控制

**Files:**
- Modify: `timewheel/pkg/timewheel/timewheel.go:62-78`

**Step 1: 修改 Task 结构体添加超时**

```go
type Task struct {
    ID          string
    Mode        TaskMode
    Interval    time.Duration
    Times       int
    Run         func(ctx context.Context) error
    Description string
    Timeout     time.Duration // 新增：任务超时时间
}
```

**Step 2: 在 executeTask 中添加超时控制**

```go
func (tw *TimeWheel) executeTask(node *taskSlot, ...) {
    // ... existing code ...
    
    // 添加超时控制
    if node.task.Timeout > 0 {
        var cancel context.CancelFunc
        node.ctx, cancel = context.WithTimeout(node.ctx, node.task.Timeout)
        defer cancel()
    }
    
    // ... existing code ...
}
```

**Step 3: Run test to verify it passes**

Run: `go test -run TestTimeWheel_ErrorHandling -v`
Expected: PASS

**Step 4: Commit**

```bash
git add timewheel/pkg/timewheel/timewheel.go
git commit -m "feat: add task timeout control"
```

---

## Task 6: 验证所有测试

**Step 1: 运行完整测试套件**

Run: `go test -v -count=1 ./...`
Expected: ALL PASS

**Step 2: 运行基准测试**

Run: `go test -bench=. -benchmem ./...`
Expected: 显示性能提升

**Step 3: Commit**

```bash
git add .
git commit -m "test: run full test suite and benchmarks"
```

---

## 执行选项

**Plan complete. Two execution options:**

**1. Subagent-Driven (this session)** - 逐个任务执行

**2. Parallel Session (separate)** - 新会话执行

**Which approach?**
