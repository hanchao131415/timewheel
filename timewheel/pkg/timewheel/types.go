package timewheel

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// 任务模式定义
// ============================================================================

// TaskMode 任务执行模式
//
//   - Once: 只执行一次
//   - FixedTimes: 执行固定次数
//   - Repeated: 周期重复执行（默认）
type TaskMode int

const (
	TaskModeRepeated   TaskMode = iota // 周期重复执行（默认）
	TaskModeOnce                       // 执行一次
	TaskModeFixedTimes                 // 执行固定次数
)

// TaskPriority 任务优先级
//
//   - High: 高优先级（优先执行）
//   - Normal: 普通优先级
//   - Low: 低优先级
type TaskPriority int

const (
	TaskPriorityHigh   TaskPriority = iota // 高优先级
	TaskPriorityNormal                     // 普通优先级（默认）
	TaskPriorityLow                        // 低优先级
)

// ============================================================================
// 告警状态定义
// ============================================================================

// AlertState 告警状态
//
//   - Pending: 条件满足但未达到持续时间
//   - Firing: 条件满足且达到持续时间
//   - Resolved: 从 Firing 转为不满足条件
type AlertState int

const (
	AlertStatePending  AlertState = iota // 条件满足但未达到持续时间
	AlertStateFiring                     // 条件满足且达到持续时间
	AlertStateResolved                   // 之前 firing，现在条件不再满足
)

// Severity 告警级别
//
//   - Critical: 严重告警
//   - Warning: 警告
//   - Info: 信息
type Severity int

const (
	SeverityCritical Severity = iota // 严重告警
	SeverityWarning                  // 警告
	SeverityInfo                     // 信息
)

// AlarmResult 任务执行结果（用于告警服务）
type AlarmResult struct {
	Value     float64 // 当前值
	Threshold float64 // 阈值
	IsFiring  bool    // 是否触发告警
}

// ============================================================================
// 任务定义
// ============================================================================

// Task 定时任务结构
//
// 字段说明：
//   - ID: 任务唯一标识，用于取消任务
//   - Mode: 执行模式（Once/FixedTimes/Repeated）
//   - Interval: 执行间隔
//   - Times: 执行次数（仅当Mode为FixedTimes时有效）
//   - Run: 任务执行函数（接受context，返回AlarmResult）
//   - Description: 任务描述，用于日志记录
//   - Timeout: 任务超时时间（0表示无超时）
//   - Severity: 告警级别（Critical/Warning/Info）
//   - For: 持续时间（Prometheus for）
//   - RepeatInterval: 重复告警间隔
//   - Labels: 告警标签
//   - Annotations: 告警描述
type Task struct {
	ID          string
	Mode        TaskMode
	Priority    TaskPriority // 任务优先级
	Interval    time.Duration
	Times       int                                   // 执行次数（FixedTimes模式）
	Run         func(ctx context.Context) AlarmResult // 返回告警结果
	Description string
	Timeout     time.Duration // 任务超时时间

	// 告警相关字段
	Severity       Severity          // 告警级别
	For            time.Duration     // 持续时间
	RepeatInterval time.Duration     // 重复告警间隔
	Labels         map[string]string // 告警标签
	Annotations    map[string]string // 告警描述
}

// taskSlot 槽位中的任务节点（双向链表）
//
// 设计考虑：
//   - 使用双向链表便于任务的插入和删除
//   - 原子操作保证并发安全
type taskSlot struct {
	task      *Task
	ctx       context.Context    // 任务专属context，用于取消
	cancel    context.CancelFunc // 取消函数
	next      *taskSlot
	prev      *taskSlot
	slotIndex int       // 所属槽位索引
	addedAt   time.Time // 添加时间
	runAt     time.Time // 下次执行时间
	executed  int       // 已执行次数（用于FixedTimes模式）

	// 告警状态跟踪
	alertState   AlertState  // 当前告警状态
	pendingSince time.Time   // 进入pending状态的时间
	lastFiredAt  time.Time   // 上次触发告警的时间
	lastResult   AlarmResult // 上次评估结果
	paused       bool        // 任务是否已暂停
}

// ============================================================================
// 轻量级任务缓存（替代go-cache，减少内存占用）
// ============================================================================

type TaskCache struct {
	mu              sync.RWMutex
	items           map[string]*taskSlot
	hits            atomic.Int64
	misses          atomic.Int64
	expired         atomic.Int64
	maxSize         int
	cleanupInterval time.Duration
	lastCleanup     time.Time
}

func NewTaskCache(maxSize int) *TaskCache {
	if maxSize <= 0 {
		maxSize = 100000
	}
	return &TaskCache{
		items:           make(map[string]*taskSlot, maxSize),
		maxSize:         maxSize,
		cleanupInterval: 5 * time.Minute, // 默认5分钟清理一次
		lastCleanup:     time.Now(),
	}
}

func (tc *TaskCache) Get(key string) (*taskSlot, bool) {
	tc.mu.RLock()
	node, ok := tc.items[key]
	tc.mu.RUnlock()
	if ok {
		tc.hits.Add(1)
		return node, true
	}
	tc.misses.Add(1)
	return nil, false
}

func (tc *TaskCache) Set(key string, node *taskSlot) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// 检查是否需要清理
	tc.checkAndCleanup()

	// 如果超过最大大小，先删除一些旧项
	if len(tc.items) >= tc.maxSize {
		tc.evictItems(10) // 每次删除10个项
		tc.expired.Add(1)
	}

	tc.items[key] = node
}

func (tc *TaskCache) Delete(key string) {
	tc.mu.Lock()
	delete(tc.items, key)
	tc.mu.Unlock()
}

func (tc *TaskCache) Len() int {
	tc.mu.RLock()
	l := len(tc.items)
	tc.mu.RUnlock()
	return l
}

func (tc *TaskCache) Stats() (hits, misses, expired int64) {
	return tc.hits.Load(), tc.misses.Load(), tc.expired.Load()
}

// TaskCacheStats 缓存统计信息
type TaskCacheStats struct {
	Size    int     `json:"size"`
	Hits    int64   `json:"hits"`
	Misses  int64   `json:"misses"`
	Expired int64   `json:"expired"`
	HitRate float64 `json:"hit_rate"`
}

func (tc *TaskCache) GetStats() TaskCacheStats {
	hits := tc.hits.Load()
	misses := tc.misses.Load()
	total := hits + misses
	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}
	return TaskCacheStats{
		Size:    tc.Len(),
		Hits:    hits,
		Misses:  misses,
		Expired: tc.expired.Load(),
		HitRate: hitRate,
	}
}

// checkAndCleanup 检查并执行清理
func (tc *TaskCache) checkAndCleanup() {
	now := time.Now()
	if now.Sub(tc.lastCleanup) >= tc.cleanupInterval {
		tc.evictItems(50) // 每次清理50个项
		tc.lastCleanup = now
	}
}

// evictItems 移除指定数量的项（简单实现：移除最早添加的项）
func (tc *TaskCache) evictItems(count int) {
	if len(tc.items) <= count {
		// 如果项数小于等于要移除的数量，清空所有项
		for k := range tc.items {
			delete(tc.items, k)
		}
		return
	}

	// 简单实现：随机移除一些项
	evicted := 0
	for k := range tc.items {
		delete(tc.items, k)
		evicted++
		if evicted >= count {
			break
		}
	}
}

// ============================================================================
// 字符串池（减少字符串分配）
// ============================================================================

type StringPool struct {
	mu   sync.RWMutex
	pool map[string]string
}

func NewStringPool() *StringPool {
	return &StringPool{
		pool: make(map[string]string),
	}
}

func (sp *StringPool) Get(s string) string {
	// 第一次检查：使用读锁
	sp.mu.RLock()
	v, ok := sp.pool[s]
	sp.mu.RUnlock()
	if ok {
		return v
	}

	// 第二次检查：使用写锁（修复变量遮蔽问题）
	sp.mu.Lock()
	// 使用不同的变量名避免遮蔽
	existing, exists := sp.pool[s]
	if exists {
		sp.mu.Unlock()
		return existing
	}
	// 添加新字符串
	sp.pool[s] = s
	sp.mu.Unlock()
	return s
}

// ============================================================================
// 时间轮指标
// ============================================================================

// Metrics 时间轮指标
type Metrics struct {
	TotalTasks        int64          `json:"total_tasks"`     // 当前任务总数
	RunningTasks      int32          `json:"running_tasks"`   // 正在执行的任务数
	Executed          int64          `json:"executed"`        // 已执行任务总数
	SlotNum           int            `json:"slot_num"`        // 槽位数量
	ShardNum          int            `json:"shard_num"`       // 分片数量
	CacheStats        TaskCacheStats `json:"cache_stats"`     // 缓存统计
	PausedTasks       int            `json:"paused_tasks"`    // 暂停的任务数
	PendingAlerts     int            `json:"pending_alerts"`  // Pending告警数
	FiringAlerts      int            `json:"firing_alerts"`   // Firing告警数
	ResolvedAlerts    int            `json:"resolved_alerts"` // Resolved告警数
	ShardDistribution []int          `json:"shard_dist"`      // 分片任务分布
	// 新增指标
	TotalTasksAdded   int64 `json:"total_tasks_added"`   // 总添加任务数
	TotalTasksRemoved int64 `json:"total_tasks_removed"` // 总移除任务数
	TotalTasksFailed  int64 `json:"total_tasks_failed"`  // 总失败任务数
	TotalTaskPanics   int64 `json:"total_task_panics"`   // 总panic任务数
	TotalTaskTime     int64 `json:"total_task_time"`     // 总任务执行时间（微秒）
	TotalCacheHits    int64 `json:"total_cache_hits"`    // 总缓存命中数
	TotalCacheMisses  int64 `json:"total_cache_misses"`  // 总缓存未命中数
	TotalAlertsFired  int64 `json:"total_alerts_fired"`  // 总告警触发数
}

// taskInfo 任务信息（用于任务执行时的快照）
type taskInfo struct {
	ID             string
	Mode           TaskMode
	Description    string
	Times          int
	Interval       time.Duration
	Timeout        time.Duration
	For            time.Duration
	RepeatInterval time.Duration
	Severity       Severity
	Labels         map[string]string
	Annotations    map[string]string
}
