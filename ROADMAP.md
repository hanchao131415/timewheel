# TimeWheel 改进路线图

本文档记录 TimeWheel 项目的改进计划和已知问题。

---

## 当前版本状态

| 维度 | 评分 | 说明 |
|------|------|------|
| **功能完整性** | 7/10 | 核心功能完善，缺少持久化和分布式 |
| **性能** | 6/10 | 有优化意识，但存在锁竞争和阻塞IO |
| **可测试性** | 5/10 | 缺少接口抽象，测试覆盖不足 |
| **可维护性** | 5/10 | 单文件过大，缺少模块化 |
| **可观测性** | 6/10 | 有基础指标，缺少标准协议支持 |
| **文档** | 8/10 | README完整，代码注释详细 |
| **综合评分** | **6.2/10** | 可用于生产，但需要持续改进 |

---

## 改进优先级矩阵

```
┌─────────────────────────────────────────────────────────────────┐
│                                                                 │
│  优先级 = 影响 × 紧急度                                          │
│                                                                 │
│  P0 (立即修复):                                                  │
│  ├── [P1] 修复双重锁问题                                         │
│  ├── [E1] 修复失败的测试                                         │
│  └── [P3] 历史持久化改为异步                                      │
│                                                                 │
│  P1 (短期改进 - 1-2周):                                          │
│  ├── [D1] 添加 Prometheus Metrics                               │
│  ├── [D3] 任务持久化支持                                         │
│  ├── [E2] 拆分 timewheel.go                                     │
│  └── [E3] 添加接口抽象                                           │
│                                                                 │
│  P2 (中期规划 - 1-2月):                                          │
│  ├── [D2] 分布式支持（etcd/Redis协调）                           │
│  ├── [D4] 任务依赖/DAG支持                                       │
│  ├── [A2] 告警模块解耦                                           │
│  └── [P2] 优化排序算法（跳表/堆）                                 │
│                                                                 │
│  P3 (长期演进 - 3-6月):                                          │
│  ├── [A4] 插件机制                                              │
│  ├── [D5] 动态优先级                                            │
│  └── [A3] 配置热更新                                            │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## 版本规划

### v1.1.0 - 稳定性改进（计划：2026-03）

**目标**：修复关键性能问题和工程问题

| 任务 | 类型 | 优先级 | 状态 |
|------|------|--------|------|
| 修复 tick() 双重锁问题 | 性能 | P0 | pending |
| 修复测试失败用例 | 工程 | P0 | pending |
| 历史持久化改为异步IO | 性能 | P0 | pending |
| 拆分 timewheel.go (2177行→多文件) | 工程 | P1 | pending |
| 添加接口抽象（Pool/Logger/Store） | 工程 | P1 | pending |
| 统一错误处理 | 工程 | P1 | pending |
| 添加代码覆盖率报告 | 工程 | P1 | pending |

---

### v1.2.0 - 可观测性增强（计划：2026-04）

**目标**：完善监控和告警能力

| 任务 | 类型 | 优先级 | 状态 |
|------|------|--------|------|
| 添加 Prometheus Metrics 导出 | 功能 | P1 | pending |
| 添加 OpenTelemetry 集成 | 功能 | P2 | pending |
| 添加 HTTP 健康检查接口 | 功能 | P1 | pending |
| 结构化日志支持（zap/logrus） | 工程 | P2 | pending |
| 告警模块解耦 | 架构 | P2 | pending |
| 添加 Dashboard 模板 | 功能 | P3 | pending |

---

### v1.3.0 - 持久化支持（计划：2026-05）

**目标**：支持任务持久化和恢复

| 任务 | 类型 | 优先级 | 状态 |
|------|------|--------|------|
| 任务快照持久化 | 功能 | P1 | pending |
| 任务恢复机制 | 功能 | P1 | pending |
| 告警历史优化（分片存储） | 性能 | P2 | pending |
| BoltDB/LevelDB 后端支持 | 功能 | P2 | pending |
| 任务导入/导出 | 功能 | P3 | pending |

---

### v2.0.0 - 分布式支持（计划：2026-Q3）

**目标**：支持分布式部署和协调

| 任务 | 类型 | 优先级 | 状态 |
|------|------|--------|------|
| etcd 协调器 | 功能 | P1 | pending |
| Redis 协调器 | 功能 | P1 | pending |
| 任务分片和负载均衡 | 功能 | P1 | pending |
| Leader 选举 | 功能 | P1 | pending |
| 任务依赖/DAG | 功能 | P2 | pending |
| 动态优先级调整 | 功能 | P3 | pending |
| 配置热更新 | 功能 | P3 | pending |

---

## 详细问题清单

### 一、性能问题

#### P1: tick() 中重复加锁（严重）

**位置**：`timewheel.go:1886` 和 `timewheel.go:1950`

**描述**：handleAlertState 和 handleTaskScheduling 都对同一分片加锁，造成双重锁竞争

**代码示例**：
```go
// 第一次加锁
func (tw *TimeWheel) handleAlertState(...) {
    shard := tw.getShard(taskInfo.ID)
    shard.mu.Lock()  // ← 第一次锁
    defer shard.mu.Unlock()
    // ...
}

// 第二次加锁（同一任务）
func (tw *TimeWheel) handleTaskScheduling(...) {
    shard := tw.getShard(taskInfo.ID)
    shard.mu.Lock()  // ← 第二次锁
    defer shard.mu.Unlock()
    // ...
}
```

**解决方案**：
```go
// 合并为一次锁操作
func (tw *TimeWheel) executeTask(node *taskSlot, now time.Time, currentSlot int) {
    // ... 前置处理 ...
    
    // 一次性获取锁，完成所有状态更新
    shard := tw.getShard(taskInfo.ID)
    shard.mu.Lock()
    defer shard.mu.Unlock()
    
    // 告警状态处理
    tw.handleAlertStateLocked(node, taskInfo, result, now)
    // 任务调度处理
    tw.handleTaskSchedulingLocked(node, taskInfo, now, currentSlot)
}
```

**预期收益**：减少 50% 锁竞争

---

#### P2: 任务执行排序开销（严重）

**位置**：`timewheel.go:1602-1612`

**描述**：每次tick都对任务列表排序，O(n log n)，高频tick下开销大

**当前实现**：
```go
// 每次tick都排序
sort.Slice(tasksToExecute, func(i, j int) bool {
    return tasksToExecute[i].task.Priority < tasksToExecute[j].task.Priority
})
```

**解决方案**：
1. **方案A - 跳表**：使用跳表维护有序任务列表，O(log n) 插入
2. **方案B - 优先级堆**：每个优先级一个堆，O(log n) 取出
3. **方案C - 分桶**：按优先级分桶，避免排序

**推荐**：方案C（分桶），实现简单，O(1) 取出

---

#### P3: history.go 同步IO阻塞（严重）

**位置**：`history.go:130-158`

**描述**：persist() 同步写文件，阻塞告警记录线程

**当前实现**：
```go
func (m *AlertHistoryManager) Record(...) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    m.history = append(m.history, record)
    
    if len(m.history) >= 1000 {
        m.persist()  // ← 同步阻塞
    }
}
```

**解决方案**：
```go
type AlertHistoryManager struct {
    recordCh chan AlertHistory  // 异步通道
    // ...
}

func (m *AlertHistoryManager) Record(...) {
    record := AlertHistory{...}
    select {
    case m.recordCh <- record:  // 非阻塞发送
    default:
        // 通道满，丢弃或记录警告
    }
}

func (m *AlertHistoryManager) runWriter() {
    for record := range m.recordCh {
        m.history = append(m.history, record)
        if len(m.history) >= 1000 {
            m.persist()
        }
    }
}
```

**预期收益**：告警记录延迟降低 90%

---

#### P4: 内存无限增长

**位置**：`history.go:95`

**描述**：history切片只清理不回收，大流量下内存持续增长

**解决方案**：
```go
func (m *AlertHistoryManager) persist() {
    // 清理过期记录后，重新分配切片释放内存
    cutoff := time.Now().AddDate(0, 0, -m.retentionDays)
    valid := make([]AlertHistory, 0, len(m.history)/2)  // 预分配较小容量
    for _, h := range m.history {
        if h.Timestamp.After(cutoff) {
            valid = append(valid, h)
        }
    }
    m.history = valid
}
```

---

#### P5: 缓存穿透风险

**位置**：`timewheel.go:462-477`

**描述**：GetTask缓存未命中时仍访问分片锁，缓存意义降低

**解决方案**：
```go
func (tw *TimeWheel) GetTask(id string) *taskSlot {
    // 使用 singleflight 防止缓存穿透
    val, err, _ := tw.sfGroup.Do(id, func() (interface{}, error) {
        if tw.taskCache != nil {
            if node, ok := tw.taskCache.Get(id); ok {
                return node, nil
            }
        }
        shard := tw.getShard(id)
        shard.mu.RLock()
        node := shard.tasks[id]
        shard.mu.RUnlock()
        return node, nil
    })
    if err != nil {
        return nil
    }
    return val.(*taskSlot)
}
```

---

#### P6: StringPool 无容量限制

**位置**：`timewheel.go:309-335`

**描述**：字符串池无限增长，长运行场景下内存泄漏

**解决方案**：
```go
type StringPool struct {
    mu      sync.RWMutex
    pool    map[string]string
    maxSize int  // 添加容量限制
}

func (sp *StringPool) Get(s string) string {
    sp.mu.RLock()
    v, ok := sp.pool[s]
    sp.mu.RUnlock()
    if ok {
        return v
    }
    
    sp.mu.Lock()
    defer sp.mu.Unlock()
    
    // 容量检查
    if len(sp.pool) >= sp.maxSize {
        // LRU淘汰策略
        sp.evictOldest()
    }
    
    sp.pool[s] = s
    return s
}
```

---

#### P7: time.Now() 过度调用

**位置**：多处

**描述**：每次执行都调用 time.Now()，高并发下成为瓶颈

**解决方案**：
```go
// 在 tick() 入口获取一次时间，传递给后续函数
func (tw *TimeWheel) tick() {
    now := time.Now()  // 只调用一次
    // ...
    tw.executeTask(node, now, current)
}
```

---

### 二、产品设计问题

#### D1: 缺少 Prometheus Metrics 导出（严重）

**描述**：无法与主流监控系统对接

**解决方案**：
```go
// metrics.go
import "github.com/prometheus/client_golang/prometheus"

var (
    tasksTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
        Name: "timewheel_tasks_total",
        Help: "Total number of tasks",
    }, []string{"priority", "state"})
    
    tasksExecuted = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "timewheel_tasks_executed_total",
        Help: "Total executed tasks",
    }, []string{"priority"})
    
    taskDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "timewheel_task_duration_seconds",
        Help:    "Task execution duration",
        Buckets: []float64{.001, .005, .01, .05, .1, .5, 1, 5},
    }, []string{"priority"})
)

// 添加 HTTP handler
func (tw *TimeWheel) Handler() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("/metrics", promhttp.Handler().ServeHTTP)
    mux.HandleFunc("/health", tw.healthHandler)
    return mux
}
```

---

#### D2: 缺少分布式支持（严重）

**描述**：单机限制定时任务场景扩展

**解决方案架构**：
```
┌─────────────────────────────────────────────────────────────────┐
│                        分布式架构                                │
│                                                                 │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐        │
│  │  Node 1     │    │  Node 2     │    │  Node 3     │        │
│  │  (Leader)   │    │ (Follower)  │    │ (Follower)  │        │
│  └──────┬──────┘    └──────┬──────┘    └──────┬──────┘        │
│         │                  │                  │                │
│         └──────────────────┼──────────────────┘                │
│                            │                                   │
│                   ┌────────┴────────┐                         │
│                   │   Coordinator   │                         │
│                   │ (etcd/Redis)    │                         │
│                   └─────────────────┘                         │
└─────────────────────────────────────────────────────────────────┘
```

**核心组件**：
1. **协调器接口**：定义 Leader 选举、任务分配
2. **etcd 实现**：基于 etcd 的分布式锁和选举
3. **Redis 实现**：基于 Redis 的分布式锁和选举

---

#### D3: 任务持久化缺失（严重）

**描述**：重启后任务丢失，不适合关键业务

**解决方案**：
```go
// store.go
type TaskStore interface {
    Save(task *Task) error
    Delete(taskID string) error
    LoadAll() ([]*Task, error)
    Snapshot() ([]byte, error)
    Restore(data []byte) error
}

// 内存实现
type MemoryStore struct {
    tasks map[string]*Task
}

// BoltDB 实现
type BoltDBStore struct {
    db *bolt.DB
}

// 使用
func (tw *TimeWheel) Restore() error {
    tasks, err := tw.store.LoadAll()
    if err != nil {
        return err
    }
    for _, task := range tasks {
        tw.AddTask(task)
    }
    return nil
}
```

---

#### D4: 缺少任务依赖/DAG

**描述**：复杂任务编排困难

**解决方案**：
```go
type TaskDependency struct {
    TaskID       string
    Dependencies []string  // 依赖的任务ID列表
}

type DAGScheduler struct {
    tasks map[string]*TaskDependency
    graph *dag.Graph
}

func (d *DAGScheduler) AddTask(task *Task, deps []string) error {
    // 检查循环依赖
    if d.hasCycle(task.ID, deps) {
        return ErrCycleDependency
    }
    
    // 添加到DAG
    d.tasks[task.ID] = &TaskDependency{
        TaskID:       task.ID,
        Dependencies: deps,
    }
    
    return nil
}

func (d *DAGScheduler) GetReadyTasks() []*Task {
    // 返回所有依赖已完成的任务
}
```

---

#### D5: 优先级设计不灵活

**描述**：只有3级，无法动态扩展

**解决方案**：
```go
// 从枚举改为整数
type TaskPriority int

const (
    TaskPriorityCritical TaskPriority = 0   // 最高
    TaskPriorityHigh     TaskPriority = 10
    TaskPriorityNormal   TaskPriority = 50  // 默认
    TaskPriorityLow      TaskPriority = 100
    TaskPriorityIdle     TaskPriority = 1000  // 最低
)

// 支持自定义
task.Priority = 25  // 在 High 和 Normal 之间
```

---

#### D6: 缺少任务去重机制

**描述**：相同ID任务冲突直接报错

**解决方案**：
```go
type DeduplicationStrategy int

const (
    DedupError    DeduplicationStrategy = iota  // 报错（默认）
    DedupSkip                                    // 跳过
    DedupReplace                                 // 替换
    DedupMerge                                   // 合并
)

func (tw *TimeWheel) AddTask(task *Task, strategy DeduplicationStrategy) error {
    shard := tw.getShard(task.ID)
    shard.mu.Lock()
    defer shard.mu.Unlock()
    
    if _, exists := shard.tasks[task.ID]; exists {
        switch strategy {
        case DedupSkip:
            return nil
        case DedupReplace:
            tw.removeTaskLocked(task.ID)
        case DedupMerge:
            return tw.mergeTaskLocked(task)
        default:
            return ErrTaskAlreadyExists
        }
    }
    
    return tw.addTaskLocked(task)
}
```

---

#### D7: 缺少流量控制/限流

**描述**：任务提交无速率限制

**解决方案**：
```go
type RateLimiter interface {
    Allow() bool
    Wait(ctx context.Context) error
}

func (tw *TimeWheel) AddTaskWithRateLimit(task *Task, limiter RateLimiter) error {
    if !limiter.Allow() {
        return ErrRateLimitExceeded
    }
    return tw.AddTask(task)
}
```

---

#### D8: 缺少健康检查接口

**描述**：K8s等编排系统难以探测状态

**解决方案**：
```go
type HealthStatus struct {
    Status      string            `json:"status"`       // healthy/unhealthy
    Running     bool              `json:"running"`
    TaskCount   int64             `json:"task_count"`
    Uptime      time.Duration     `json:"uptime"`
    Components  map[string]string `json:"components"`
}

func (tw *TimeWheel) Health() *HealthStatus {
    return &HealthStatus{
        Status:     "healthy",
        Running:    tw.running.Load(),
        TaskCount:  tw.Stats().TotalTasks,
        Uptime:     time.Since(tw.startTime),
        Components: map[string]string{
            "pool":   "healthy",
            "cache":  "healthy",
        },
    }
}

func (tw *TimeWheel) healthHandler(w http.ResponseWriter, r *http.Request) {
    health := tw.Health()
    if !health.Running {
        w.WriteHeader(http.StatusServiceUnavailable)
    }
    json.NewEncoder(w).Encode(health)
}
```

---

### 三、工程质量问题

#### E1: 测试失败（严重）

**位置**：`multi_level_timewheel_extended_test.go:71`

**描述**：CI/CD 不稳定

**修复方案**：
1. 增加等待时间或使用同步机制
2. 检查测试逻辑的正确性
3. 添加重试机制

---

#### E2: 单文件过大（严重）

**位置**：`timewheel.go` (2177行)

**描述**：难以维护和测试

**解决方案**：
```
pkg/timewheel/
├── timewheel.go        # 核心结构和基础方法 (~500行)
├── options.go          # 配置选项 (~200行)
├── task.go             # 任务相关操作 (~300行)
├── alert.go            # 告警逻辑 (~300行)
├── metrics.go          # 监控指标 (~200行)
├── pool.go             # 协程池相关 (~100行)
├── cache.go            # 缓存相关 (~200行)
├── errors.go           # 错误定义 (~50行)
└── interfaces.go       # 接口定义 (~100行)
```

---

#### E3: 缺少接口抽象（严重）

**描述**：无法Mock测试，扩展困难

**解决方案**：
```go
// interfaces.go

// GoroutinePool 协程池接口
type GoroutinePool interface {
    Submit(func()) error
    Release()
}

// TaskStore 任务存储接口
type TaskStore interface {
    Save(task *Task) error
    Delete(taskID string) error
    LoadAll() ([]*Task, error)
}

// MetricsCollector 指标收集接口
type MetricsCollector interface {
    IncrTaskAdded(priority TaskPriority)
    IncrTaskExecuted(priority TaskPriority)
    ObserveTaskDuration(priority TaskPriority, duration time.Duration)
}

// Logger 日志接口
type Logger interface {
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, fields ...Field)
}
```

---

#### E4: 错误处理不一致

**描述**：有些用 fmt.Errorf，有些用自定义错误

**解决方案**：
```go
// errors.go
import "errors"

var (
    ErrIntervalTooSmall    = errors.New("interval must be greater than 0")
    ErrSlotsTooFew         = errors.New("slots must be greater than 0")
    ErrTaskNotFound        = errors.New("task not found")
    ErrWheelNotRunning     = errors.New("timewheel is not running")
    ErrInvalidParam        = errors.New("invalid parameter")
    ErrTaskAlreadyExists   = errors.New("task already exists")
    ErrRateLimitExceeded   = errors.New("rate limit exceeded")
)

// 包装错误时使用 fmt.Errorf
func (tw *TimeWheel) AddTask(task *Task) error {
    if task == nil {
        return fmt.Errorf("%w: task is nil", ErrInvalidParam)
    }
    // ...
}
```

---

#### E5: 缺少 context 传递

**位置**：`pool_manager.go`

**描述**：无法追踪任务链路

**解决方案**：
```go
func (pm *PoolManager) ExecuteWithContext(ctx context.Context, task *Task, f func(context.Context)) error {
    wrappedF := func() {
        f(ctx)  // 传递 context
    }
    return pm.Execute(task, wrappedF)
}
```

---

#### E6: 日志格式不统一

**描述**：有`[INFO]`、`[ERROR]`等，无结构化日志

**解决方案**：
```go
// 使用结构化日志接口
type Logger interface {
    Debug(msg string, fields ...Field)
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, fields ...Field)
}

type Field struct {
    Key   string
    Value interface{}
}

func String(key, value string) Field {
    return Field{Key: key, Value: value}
}

func Any(key string, value interface{}) Field {
    return Field{Key: key, Value: value}
}

// 使用
tw.logger.Info("task executed",
    String("task_id", taskID),
    Any("duration", duration),
)
```

---

#### E8: 魔法数字

**描述**：`64`、`100000`等硬编码值

**解决方案**：
```go
const (
    DefaultShardNum          = 64
    DefaultSlotNum           = 60
    DefaultInterval          = 100 * time.Millisecond
    DefaultCacheSize         = 100000
    DefaultHistoryRetention  = 30  // days
    DefaultMaxConcurrent     = 0   // unlimited
    DefaultHighPriorityPool  = 3   // CPU * 3
    DefaultNormalPriorityPool = 1  // CPU * 1
    DefaultLowPriorityPool   = 0.5 // CPU / 2
)
```

---

### 四、架构设计问题

#### A1: 单层时间轮精度限制

**描述**：最大延迟 = 槽位数 × 间隔，大延迟任务需要多层

**解决方案**：已实现 MultiLevelTimeWheel，可继续优化为真正的层级时间轮（像内核定时器）

---

#### A2: 告警与调度耦合

**描述**：告警逻辑和任务调度混在一起，职责不清

**解决方案**：
```go
// 告警服务独立
type AlertService struct {
    timeWheel *TimeWheel
    store     AlertStore
    notifier  AlertNotifier
}

func (s *AlertService) AddAlert(rule AlertRule) error {
    task := &Task{
        ID:       rule.ID,
        Interval: rule.Interval,
        Run:      s.evaluate(rule),
    }
    return s.timeWheel.AddTask(task)
}
```

---

#### A3: 配置不可热更新

**描述**：运行时无法调整参数

**解决方案**：
```go
type TimeWheelConfig struct {
    MaxConcurrentTasks int           `json:"max_concurrent_tasks"`
    LogLevel           int           `json:"log_level"`
    StatusInterval     time.Duration `json:"status_interval"`
}

func (tw *TimeWheel) UpdateConfig(config TimeWheelConfig) error {
    tw.mu.Lock()
    defer tw.mu.Unlock()
    
    // 更新并发控制
    if config.MaxConcurrentTasks != tw.maxConcurrentTasks {
        tw.updateSemaphore(config.MaxConcurrentTasks)
    }
    
    // 更新日志级别
    tw.logLevel = config.LogLevel
    
    return nil
}
```

---

#### A4: 缺少插件机制

**描述**：无法扩展自定义功能

**解决方案**：
```go
type Plugin interface {
    Name() string
    Init(tw *TimeWheel) error
    OnTaskAdd(task *Task) error
    OnTaskExecute(task *Task) error
    OnTaskComplete(task *Task, result AlarmResult)
    OnTaskRemove(taskID string)
}

func (tw *TimeWheel) RegisterPlugin(plugin Plugin) error {
    if err := plugin.Init(tw); err != nil {
        return err
    }
    tw.plugins = append(tw.plugins, plugin)
    return nil
}
```

---

## 快速修复清单

以下是可以立即执行的改进：

### 1. 修复双重锁（10分钟）
- 合并 handleAlertState 和 handleTaskScheduling 的锁

### 2. 异步持久化（15分钟）
- 历史记录用 channel 异步写入

### 3. 添加接口抽象（30分钟）
- GoroutinePool 接口
- Logger 接口
- HistoryStore 接口

### 4. 拆分文件（30分钟）
- options.go - 配置选项
- alert.go - 告警逻辑
- metrics.go - 监控指标
- task.go - 任务相关

### 5. 添加健康检查（15分钟）
- HTTP /health 接口
- HTTP /metrics 接口（Prometheus格式）

---

## 功能完善度分析

```
┌─────────────────────────────────────────────────────────────────┐
│                       功能完善度                                 │
├─────────────────────────────────────────────────────────────────┤
│  ✅ 已实现                        ❌ 缺失                        │
├─────────────────────────────────────────────────────────────────┤
│  ✅ 基础任务调度                  ❌ 任务持久化                   │
│  ✅ 多执行模式                    ❌ 任务恢复机制                 │
│  ✅ 优先级队列                    ❌ 分布式协调                   │
│  ✅ 告警状态机                    ❌ Prometheus导出               │
│  ✅ 历史记录                      ❌ 动态优先级调整               │
│  ✅ 并发控制                      ❌ 任务依赖/DAG                 │
│  ✅ 优雅停机                      ❌ 任务分片/负载均衡             │
│  ✅ 基础监控指标                  ❌ OpenTelemetry集成            │
│                                  ❌ 任务去重/合并                 │
│                                  ❌ 流量控制/限流                 │
│                                  ❌ 健康检查HTTP接口              │
└─────────────────────────────────────────────────────────────────┘
```

---

## 参与贡献

如果您想帮助改进这个项目，请查看：

1. [问题列表](./ISSUES.md) - 待解决的问题
2. [贡献指南](./README.md#贡献指南) - 如何贡献代码
3. [开发指南](./CONTRIBUTING.md) - 开发环境设置

---

## 更新日志

### 2026-03-02
- 创建改进路线图文档
- 分析并记录所有已知问题
- 制定版本规划
