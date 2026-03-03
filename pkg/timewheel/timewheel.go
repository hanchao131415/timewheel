package timewheel

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/panjf2000/ants/v2"
)

// ============================================================================
// 常量定义
// ============================================================================

const (
	// DefaultSlots 默认槽位数量
	DefaultSlots = 60
	// DefaultInterval 默认时间间隔（毫秒）
	DefaultInterval = 100
	// DefaultCacheTTL 默认缓存TTL
	DefaultCacheTTL = 24 * time.Hour
	// DefaultCacheCleanupInterval 默认缓存清理间隔
	DefaultCacheCleanupInterval = 10 * time.Minute

	// 新增：分片和缓存相关常量（修复魔法数字）
	// DefaultShardNum 默认分片数量
	DefaultShardNum = 64
	// DefaultShardCapacity 每个分片的初始容量
	DefaultShardCapacity = 16
	// DefaultCacheSize 默认任务缓存大小
	DefaultCacheSize = 100000
	// DefaultPoolSizeMultiplier 协程池大小乘数（相对于 CPU 核心数）
	DefaultPoolSizeMultiplier = 10
	// DefaultRecordChannelSize 历史记录通道缓冲区大小
	DefaultRecordChannelSize = 1000
)

// ============================================================================
// 时间轮核心结构
// ============================================================================

// TimeWheel 时间轮结构
//
// 设计考虑：
//   - 多层时间轮支持更大范围的延迟（可选优化，本版实现单层）
//   - 使用sync.Pool减少任务对象分配
//   - 使用原子操作替代锁，提高并发性能
//   - 支持动态添加和取消任务
//   - 使用分片锁减少锁竞争
//
// 线程安全：
//   - 任务添加和删除使用分片锁
//   - 轮转执行时使用读锁
//   - 任务执行在独立的goroutine中

type TimeWheel struct {
	shards             []TaskMapShard                                                         // 分片数组，用于减少锁竞争
	shardNum           int                                                                    // 分片数量
	slots              []*taskSlot                                                            // 槽位数组（双向链表头）
	slotsMu            sync.RWMutex                                                           // 槽位数组读写锁（修复竞态条件）
	slotNum            int                                                                    // 槽位数量
	interval           time.Duration                                                          // 时间间隔
	currentSlot        atomic.Int64                                                           // 当前槽位索引（原子操作）
	taskPool           sync.Pool                                                              // 任务节点对象池
	running            atomic.Bool                                                            // 运行状态（原子操作）
	ctx                context.Context                                                        // 上下文，用于优雅关闭
	cancel             context.CancelFunc                                                     // 取消函数
	wg                 sync.WaitGroup                                                         // WaitGroup，管理轮转goroutine
	taskWg             sync.WaitGroup                                                         // WaitGroup，管理任务执行goroutine
	pool               *ants.Pool                                                             // 任务执行协程池
	poolManager        *PoolManager                                                           // 协程池管理器（用于优先级任务）
	logger             *log.Logger                                                            // 日志记录器
	stats              atomic.Int64                                                           // 统计：已执行任务总数
	onError            func(error)                                                            // 错误回调
	onAlertStateChange func(taskID string, oldState, newState AlertState, result AlarmResult) // 告警状态变化回调
	historyManager     *AlertHistoryManager                                                   // 告警历史管理器

	// 缓存优化
	taskCache    *TaskCache  // 任务缓存（加速GetTask查找）
	stringPool   *StringPool // 字符串池（减少字符串分配）
	cacheEnabled bool        // 是否启用缓存

	// 并发控制
	maxConcurrentTasks int           // 最大并发任务数
	runningTasks       atomic.Int32  // 当前运行中的任务数
	taskSemaphore      chan struct{} // 任务信号量，用于控制并发

	// 状态监控
	statusInterval time.Duration // 状态打印间隔
	statusEnabled  bool          // 是否启用状态打印

	// 日志控制
	logLevel int // 日志级别: 0=Debug, 1=Info, 2=Warn, 3=Error

	// 监控指标
	totalTasksAdded    atomic.Int64 // 总添加任务数
	totalTasksRemoved  atomic.Int64 // 总移除任务数
	totalTasksExecuted atomic.Int64 // 总执行任务数
	totalTasksFailed   atomic.Int64 // 总失败任务数
	totalTaskPanics    atomic.Int64 // 总panic任务数
	totalTaskTime      atomic.Int64 // 总任务执行时间（微秒）
	totalCacheHits     atomic.Int64 // 总缓存命中数
	totalCacheMisses   atomic.Int64 // 总缓存未命中数
	totalAlertsFired   atomic.Int64 // 总告警触发数

	// 参数校验
	requestedSlotNum     int           // 用户请求的槽位数（用于校验）

	// 存储相关
	taskStore    TaskStore    // 任务存储接口
	historyStore HistoryStore // 历史存储接口
	autoRestore  bool         // 启动时自动恢复任务
	startTime    time.Time    // 启动时间（用于健康检查）
	requestedInterval    time.Duration // 用户请求的时间间隔（用于校验）
	requestedSlotNumSet  bool          // 是否设置了槽位数
	requestedIntervalSet bool          // 是否设置了时间间隔
}

// 日志级别常量
const (
	LogLevelDebug = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// debug 调试日志
func (tw *TimeWheel) debug(format string, args ...interface{}) {
	if tw.logLevel <= LogLevelDebug {
		tw.logger.Printf("[DEBUG] "+format, args...)
	}
}

// info 信息日志
func (tw *TimeWheel) info(format string, args ...interface{}) {
	if tw.logLevel <= LogLevelInfo {
		tw.logger.Printf("[INFO] "+format, args...)
	}
}

// warn 警告日志
func (tw *TimeWheel) warn(format string, args ...interface{}) {
	if tw.logLevel <= LogLevelWarn {
		tw.logger.Printf("[WARN] "+format, args...)
	}
}

// error 错误日志
func (tw *TimeWheel) error(format string, args ...interface{}) {
	if tw.logLevel <= LogLevelError {
		tw.logger.Printf("[ERROR] "+format, args...)
	}
}

// ============================================================================
// 构造函数
// ============================================================================

// New 创建时间轮实例
//
// 请求参数：
//   - opts: 可变配置选项
//
// 返回值：
//   - *TimeWheel: 时间轮实例
//   - error: 创建失败时返回错误
//
// 错误列表：
//   - ErrSlotsTooFew: 槽位数量必须大于0
//   - ErrIntervalTooSmall: 时间间隔必须大于0
//
// 性能考虑：
//   - 预分配槽位数组，减少运行时内存分配
//   - 初始化对象池，减少GC压力
func New(opts ...Option) (*TimeWheel, error) {
	// 初始化默认值
	tw := &TimeWheel{
		slots:             make([]*taskSlot, DefaultSlots),
		slotNum:           DefaultSlots,
		interval:          time.Duration(DefaultInterval) * time.Millisecond,
		logger:            log.Default(),
		requestedSlotNum:  DefaultSlots, // 用于校验
		requestedInterval: time.Duration(DefaultInterval) * time.Millisecond,
		shardNum:          DefaultShardNum,
		shards:            make([]TaskMapShard, DefaultShardNum),
	}

	// 初始化分片
	for i := 0; i < DefaultShardNum; i++ {
		tw.shards[i].tasks = make(map[string]*taskSlot, DefaultShardCapacity)
	}

	// 应用配置选项
	for _, opt := range opts {
		opt(tw)
	}

	// 初始化缓存（如果启用）
	if tw.cacheEnabled {
		tw.taskCache = NewTaskCache(DefaultCacheSize)
		tw.stringPool = NewStringPool()
		tw.logger.Printf("[INFO] 任务缓存已启用")
	}

	// 参数校验（仅在用户提供了选项时校验）
	if tw.requestedSlotNumSet && tw.requestedSlotNum <= 0 {
		tw.logger.Printf("[ERROR] 创建时间轮失败: %v", ErrSlotsTooFew)
		return nil, ErrSlotsTooFew
	}
	if tw.requestedIntervalSet && tw.requestedInterval <= 0 {
		tw.logger.Printf("[ERROR] 创建时间轮失败: %v", ErrIntervalTooSmall)
		return nil, ErrIntervalTooSmall
	}

	// 重新分配槽位数组（如果使用了自定义槽位数）
	if tw.slotNum != DefaultSlots {
		tw.slots = make([]*taskSlot, tw.slotNum)
	}

	// 初始化任务节点对象池
	tw.taskPool = newTaskPool()

	// 初始化协程池
	poolSize := runtime.GOMAXPROCS(0) * DefaultPoolSizeMultiplier
	pool, err := ants.NewPool(poolSize, ants.WithPanicHandler(func(panic interface{}) {
		tw.logger.Printf("[PANIC] 协程池发生panic: %v", panic)
	}))
	if err != nil {
		tw.logger.Printf("[ERROR] 创建协程池失败: %v", err)
		return nil, fmt.Errorf("创建协程池失败: %w", err)
	}
	tw.pool = pool

	// 初始化并发控制
	if tw.maxConcurrentTasks > 0 {
		tw.taskSemaphore = make(chan struct{}, tw.maxConcurrentTasks)
		tw.logger.Printf("[INFO] 并发控制已启用: 最大并发任务数=%d", tw.maxConcurrentTasks)
	} else {
		tw.logger.Printf("[INFO] 并发控制未启用: 最大并发任务数=无限制")
	}

	// 初始化上下文
	tw.ctx, tw.cancel = context.WithCancel(context.Background())

	tw.logger.Printf("[INFO] 时间轮创建成功: 槽位数量=%d, 时间间隔=%v, 协程池大小=%d, 最大并发任务数=%d",
		tw.slotNum, tw.interval, poolSize, tw.maxConcurrentTasks)

	return tw, nil
}

// ============================================================================
// 核心方法
// ============================================================================

// Start 启动时间轮
//
// 请求参数：
//   - 无
//
// 返回值：
//   - error: 启动失败时返回错误
//
// 线程安全：
//   - 使用原子操作保证状态一致性
func (tw *TimeWheel) Start() error {
	// 检查是否已经在运行
	if tw.running.Load() {
		tw.logger.Printf("[WARN] 时间轮已经在运行")
		return nil
	}

	// 设置运行状态
	if !tw.running.CompareAndSwap(false, true) {
		tw.logger.Printf("[WARN] 时间轮启动失败: 已被其他goroutine启动")
		return nil
	}

	// 启动轮转goroutine
	tw.wg.Add(1)
	go tw.runLoop()

	// 启动状态监控goroutine（如果启用）
	if tw.statusEnabled {
		tw.wg.Add(1)
		go tw.statusLoop()
	}

	tw.logger.Printf("[INFO] 时间轮已启动: 槽位数量=%d, 时间间隔=%v",
		tw.slotNum, tw.interval)

	// 自动恢复任务（如果启用）
	if tw.autoRestore && tw.taskStore != nil {
		go tw.restoreTasks()
	}

	return nil
}

// restoreTasks 从存储恢复任务
func (tw *TimeWheel) restoreTasks() {
	tasks, err := tw.taskStore.LoadEnabled()
	if err != nil {
		tw.logger.Printf("[WARN] 加载任务失败: %v", err)
		return
	}

	for _, task := range tasks {
		if err := tw.AddTask(task); err != nil {
			tw.logger.Printf("[WARN] 恢复任务失败: id=%s, err=%v", task.ID, err)
		} else {
			tw.logger.Printf("[INFO] 恢复任务成功: id=%s", task.ID)
		}
	}

	tw.logger.Printf("[INFO] 任务恢复完成: 恢复了 %d 个任务", len(tasks))
}

// statusLoop 状态监控循环
func (tw *TimeWheel) statusLoop() {
	defer tw.wg.Done()

	startTime := time.Now()
	ticker := time.NewTicker(tw.statusInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tw.printStatus(startTime)
		case <-tw.ctx.Done():
			tw.logger.Printf("[INFO] 状态监控循环已退出")
			return
		}
	}
}

// printStatus 打印时间轮状态
func (tw *TimeWheel) printStatus(startTime time.Time) {
	// 计算基本指标 - 遍历所有分片
	totalTasks := 0
	for i := range tw.shards {
		shard := &tw.shards[i]
		shard.mu.RLock()
		totalTasks += len(shard.tasks)
		shard.mu.RUnlock()
	}

	executedTasks := tw.stats.Load()
	currentSlot := tw.currentSlot.Load()
	runTime := time.Since(startTime)

	// 使用读锁保护槽位数组访问（修复竞态条件）
	tw.slotsMu.RLock()
	activeSlots := 0
	totalInSlots := 0
	for _, slot := range tw.slots {
		if slot != nil {
			activeSlots++
			// 统计该槽位的任务数
			taskCount := 0
			for node := slot; node != nil; node = node.next {
				taskCount++
			}
			totalInSlots += taskCount
		}
	}
	tw.slotsMu.RUnlock()

	// 打印详细状态
	tw.logger.Printf("[STATUS] ========================================")
	tw.logger.Printf("[STATUS] 时间轮状态报告")
	tw.logger.Printf("[STATUS] ========================================")
	tw.logger.Printf("[STATUS] 基本信息:")
	tw.logger.Printf("[STATUS]   运行时间: %v", runTime)
	tw.logger.Printf("[STATUS]   槽位数量: %d", tw.slotNum)
	tw.logger.Printf("[STATUS]   时间间隔: %v", tw.interval)
	tw.logger.Printf("[STATUS]   当前槽位: %d", currentSlot)
	tw.logger.Printf("[STATUS] 任务统计:")
	tw.logger.Printf("[STATUS]   总任务数: %d", totalTasks)
	tw.logger.Printf("[STATUS]   已执行任务: %d", executedTasks)
	tw.logger.Printf("[STATUS]   槽位使用: %d/%d (%.1f%%)", activeSlots, tw.slotNum, float64(activeSlots)/float64(tw.slotNum)*100)
	tw.logger.Printf("[STATUS]   槽位任务数: %d", totalInSlots)
	tw.logger.Printf("[STATUS] 模式分布:")

	// 统计任务模式分布
	modeCounts := make(map[string]int)
	for i := range tw.shards {
		shard := &tw.shards[i]
		shard.mu.RLock()
		for _, node := range shard.tasks {
			switch node.task.Mode {
			case TaskModeOnce:
				modeCounts["Once"]++
			case TaskModeFixedTimes:
				modeCounts["FixedTimes"]++
			case TaskModeRepeated:
				modeCounts["Repeated"]++
			default:
				modeCounts["Unknown"]++
			}
		}
		shard.mu.RUnlock()
	}

	for mode, count := range modeCounts {
		tw.logger.Printf("[STATUS]   %s: %d", mode, count)
	}

	tw.logger.Printf("[STATUS] ========================================")
}

// Stop 优雅停止时间轮
//
// 请求参数：
//   - 无
//
// 返回值：
//   - 无
//
// 线程安全：
//   - 使用原子操作保证状态一致性
//   - 等待所有任务执行完成
func (tw *TimeWheel) Stop() {
	// 原子性地检查并设置停止状态
	if !tw.running.CompareAndSwap(true, false) {
		tw.logger.Printf("[WARN] 时间轮未在运行")
		return
	}

	// 发送停止信号
	tw.cancel()

	// 等待所有轮转goroutine退出
	tw.wg.Wait()

	// 等待所有任务执行goroutine退出
	tw.taskWg.Wait()

	// 关闭协程池管理器（修复: PoolManager 泄漏）
	if tw.poolManager != nil {
		tw.poolManager.Release()
		tw.poolManager = nil
	}

	// 关闭协程池
	if tw.pool != nil {
		tw.pool.Release()
	}

	// 关闭告警历史管理器
	if tw.historyManager != nil {
		tw.historyManager.Close()
	}

	// 重置上下文（为下次启动做准备）
	tw.ctx, tw.cancel = context.WithCancel(context.Background())

	// 重新初始化协程池
	pool, err := ants.NewPool(runtime.GOMAXPROCS(0)*10, ants.WithPanicHandler(func(panic interface{}) {
		tw.logger.Printf("[PANIC] 协程池发生panic: %v", panic)
	}))
	if err != nil {
		tw.logger.Printf("[ERROR] 重新创建协程池失败: %v", err)
	}
	tw.pool = pool

	// 重新初始化信号量
	if tw.maxConcurrentTasks > 0 {
		tw.taskSemaphore = make(chan struct{}, tw.maxConcurrentTasks)
		tw.logger.Printf("[INFO] 并发控制已重新初始化: 最大并发任务数=%d", tw.maxConcurrentTasks)
	}

	// 重置运行任务计数器
	tw.runningTasks.Store(0)

	// 重置 taskWg，避免重用导致的错误
	tw.taskWg = sync.WaitGroup{}

	tw.logger.Printf("[INFO] 时间轮已停止: 共执行任务数=%d", tw.stats.Load())
}

// GetTasksByState 根据告警状态获取任务列表
//
// 请求参数：
//   - state: 告警状态（Pending/Firing/Resolved）
//
// 返回值：
//   - []*Task: 匹配状态的任务列表
func (tw *TimeWheel) GetTasksByState(state AlertState) []*Task {
	var result []*Task

	for i := range tw.shards {
		shard := &tw.shards[i]
		shard.mu.RLock()

		for _, node := range shard.tasks {
			if node.alertState == state && !node.paused {
				result = append(result, node.task)
			}
		}

		shard.mu.RUnlock()
	}

	return result
}


// ============================================================================
// 内部方法
// ============================================================================

// runLoop 时间轮轮转主循环
//
// 设计考虑：
//   - 固定间隔轮转，检查当前槽位的任务
//   - 执行到期的任务，支持延迟调整
//   - 任务执行在独立goroutine中，避免阻塞轮转
func (tw *TimeWheel) runLoop() {
	defer tw.wg.Done()

	ticker := time.NewTicker(tw.interval)
	defer ticker.Stop()

	tw.debug("时间轮轮转循环已启动")

	for {
		select {
		case <-tw.ctx.Done():
			tw.debug("时间轮轮转循环已退出")
			return
		case <-ticker.C:
			tw.tick()
		}
	}
}

// tick 处理当前槽位的任务
func (tw *TimeWheel) tick() {
	// 获取当前槽位索引
	current := int(tw.currentSlot.Add(1)) % tw.slotNum

	tw.debug("开始处理槽位: index=%d", current)

	// 使用读锁保护槽位数组访问（修复竞态条件）
	tw.slotsMu.RLock()
	head := tw.slots[current]

	// 先收集所有需要执行的任务，避免在遍历过程中链表结构被修改
	var tasksToExecute []*taskSlot
	now := time.Now() // 只调用一次time.Now()

	for node := head; node != nil; node = node.next {
		if now.Sub(node.runAt) >= 0 {
			tasksToExecute = append(tasksToExecute, node)
		}
	}
	tw.slotsMu.RUnlock()

	// 根据任务优先级排序，高优先级任务先执行
	sort.Slice(tasksToExecute, func(i, j int) bool {
		// 优先级数值越小，优先级越高
		// 添加task nil检查，避免空指针异常
		if tasksToExecute[i].task == nil {
			return false
		}
		if tasksToExecute[j].task == nil {
			return true
		}
		return tasksToExecute[i].task.Priority < tasksToExecute[j].task.Priority
	})

	// 执行收集到的任务（按照优先级顺序执行）
	// 为了确保优先级顺序，我们使用一个通道来控制任务的执行顺序
	taskChan := make(chan *taskSlot, len(tasksToExecute))

	// 按照优先级顺序将任务发送到通道
	for _, node := range tasksToExecute {
		taskChan <- node
	}
	close(taskChan)

	// 在启动 goroutine 前批量调用 Add，确保 Wait() 能正确等待
	taskCount := len(tasksToExecute)
	if taskCount > 0 {
		tw.taskWg.Add(taskCount)

		// 启动一个goroutine来执行任务，确保按照优先级顺序执行
		go func() {
			for node := range taskChan {
				// 创建局部变量捕获当前node值，避免闭包引用问题
				currentNode := node
				if tw.poolManager != nil {
					// 使用协程池管理器执行任务
					err := tw.poolManager.Execute(currentNode.task, func() {
						tw.executeTask(currentNode, now, current)
					})
					if err != nil {
						tw.logger.Printf("[ERROR] 提交任务到协程池失败: %v", err)
						tw.taskWg.Done()
					}
				} else {
					// 使用默认协程池执行任务
					err := tw.pool.Submit(func() {
						tw.executeTask(currentNode, now, current)
					})
					if err != nil {
						tw.logger.Printf("[ERROR] 提交任务到协程池失败: %v", err)
						tw.taskWg.Done()
					}
				}
			}
		}()
	}
}

// executeTask 执行任务
//
// 参数：
//   - node: 任务节点
//   - now: 当前时间
//   - currentSlot: 当前槽位索引
func (tw *TimeWheel) GetMetrics() *Metrics {
	totalTasks := 0
	pausedTasks := 0
	pendingAlerts := 0
	firingAlerts := 0
	resolvedAlerts := 0
	shardDist := make([]int, tw.shardNum)

	for i := range tw.shards {
		shard := &tw.shards[i]
		shard.mu.RLock()
		count := len(shard.tasks)
		totalTasks += count
		shardDist[i] = count

		for _, node := range shard.tasks {
			if node.paused {
				pausedTasks++
			}
			switch node.alertState {
			case AlertStatePending:
				pendingAlerts++
			case AlertStateFiring:
				firingAlerts++
			case AlertStateResolved:
				resolvedAlerts++
			}
		}
		shard.mu.RUnlock()
	}

	var cacheStats TaskCacheStats
	if tw.taskCache != nil {
		cacheStats = tw.taskCache.GetStats()
	}

	return &Metrics{
		TotalTasks:        int64(totalTasks),
		RunningTasks:      tw.runningTasks.Load(),
		Executed:          tw.stats.Load(),
		SlotNum:           tw.slotNum,
		ShardNum:          tw.shardNum,
		CacheStats:        cacheStats,
		PausedTasks:       pausedTasks,
		PendingAlerts:     pendingAlerts,
		FiringAlerts:      firingAlerts,
		ResolvedAlerts:    resolvedAlerts,
		ShardDistribution: shardDist,
		// 新增指标
		TotalTasksAdded:   tw.totalTasksAdded.Load(),
		TotalTasksRemoved: tw.totalTasksRemoved.Load(),
		TotalTasksFailed:  tw.totalTasksFailed.Load(),
		TotalTaskPanics:   tw.totalTaskPanics.Load(),
		TotalTaskTime:     tw.totalTaskTime.Load(),
		TotalCacheHits:    tw.totalCacheHits.Load(),
		TotalCacheMisses:  tw.totalCacheMisses.Load(),
		TotalAlertsFired:  tw.totalAlertsFired.Load(),
	}
}

// IsRunning 检查时间轮是否在运行
//
// 返回值：
//   - bool: 运行状态
func (tw *TimeWheel) IsRunning() bool {
	return tw.running.Load()
}

// acquireSemaphore 获取信号量
func (tw *TimeWheel) acquireSemaphore(semaphoreAcquired *bool) bool {
	if tw.taskSemaphore != nil {
		select {
		case tw.taskSemaphore <- struct{}{}:
			running := tw.runningTasks.Add(1)
			tw.debug("获取信号量成功，当前运行任务数: %d", running)
			*semaphoreAcquired = true
		case <-tw.ctx.Done():
			// 上下文被取消，直接返回
			tw.debug("上下文被取消，跳过任务执行")
			return false
		}
	}
	return true
}

// checkTaskState 检查任务状态
func (tw *TimeWheel) checkTaskState(node *taskSlot) bool {
	// 首先检查任务是否已被移除
	if node.task == nil {
		return false
	}

	// 检查任务是否已暂停
	if node.paused {
		tw.debug("任务已暂停，跳过执行: id=%s", node.task.ID)
		return false
	}

	return true
}

// copyTaskInfo 复制任务信息
func (tw *TimeWheel) copyTaskInfo(node *taskSlot) *taskInfo {
	task := node.task
	return &taskInfo{
		ID:             task.ID,
		Mode:           task.Mode,
		Description:    task.Description,
		Times:          task.Times,
		Interval:       task.Interval,
		Timeout:        task.Timeout,
		For:            task.For,
		RepeatInterval: task.RepeatInterval,
		Severity:       task.Severity,
		Labels:         task.Labels,
		Annotations:    task.Annotations,
	}
}

// runTask 执行任务函数
func (tw *TimeWheel) runTask(node *taskSlot, taskInfo *taskInfo) (AlarmResult, time.Duration, error) {
	var result AlarmResult
	var err error

	tw.logger.Printf("[INFO] 开始执行任务: id=%s, 模式=%v, 描述=%s, 延迟=%v, 超时=%v",
		taskInfo.ID, taskInfo.Mode, taskInfo.Description, taskInfo.Interval, taskInfo.Timeout)

	// 执行任务函数（支持超时）
	startTime := time.Now()

	// 修复: 检查 context 是否为 nil（任务可能已被移除）
	if node.ctx == nil {
		tw.debug("任务 context 为 nil，跳过执行: id=%s", taskInfo.ID)
		return result, 0, WrapError(ErrContextNil, "task %s", taskInfo.ID)
	}

	if taskInfo.Timeout > 0 {
		// 检查父context是否已取消
		select {
		case <-node.ctx.Done():
			tw.debug("任务context已取消，跳过执行: id=%s", taskInfo.ID)
			return result, 0, WrapError(ErrContextCanceled, "task %s", taskInfo.ID)
		default:
		}
		ctx, cancel := context.WithTimeout(node.ctx, taskInfo.Timeout)
		result = node.task.Run(ctx)
		cancel()
	} else {
		result = node.task.Run(node.ctx)
	}
	duration := time.Since(startTime)

	return result, duration, err
}

// updateTaskResult 更新任务执行结果
func (tw *TimeWheel) updateTaskResult(node *taskSlot, result AlarmResult, duration time.Duration, err error, panicValue interface{}) {
	// 更新统计
	tw.stats.Add(1)
	tw.totalTasksExecuted.Add(1)
	tw.totalTaskTime.Add(duration.Microseconds())

	// 增加执行次数
	node.executed++

	// 保存当前结果到节点
	node.lastResult = result

	// 处理执行结果
	// 如果发生panic，视为执行失败
	if panicValue != nil {
		tw.totalTasksFailed.Add(1)
		tw.totalTaskPanics.Add(1)
		taskID := "unknown"
		taskDesc := ""
		if node.task != nil {
			taskID = node.task.ID
			taskDesc = node.task.Description
		}
		tw.logger.Printf("[ERROR] 任务执行panic: id=%s, 描述=%s, panic=%v, 耗时=%v",
			taskID, taskDesc, panicValue, duration)

		// 调用错误回调
		if tw.onError != nil {
			tw.onError(fmt.Errorf("task %s panicked: %v", taskID, panicValue))
		}
	} else if err != nil {
		tw.totalTasksFailed.Add(1)
		taskID := "unknown"
		taskDesc := ""
		if node.task != nil {
			taskID = node.task.ID
			taskDesc = node.task.Description
		}
		tw.logger.Printf("[ERROR] 任务执行失败: id=%s, 描述=%s, 错误=%v, 耗时=%v",
			taskID, taskDesc, err, duration)

		// 调用错误回调
		if tw.onError != nil {
			tw.onError(err)
		}
	} else {
		taskID := "unknown"
		taskDesc := ""
		if node.task != nil {
			taskID = node.task.ID
			taskDesc = node.task.Description
		}
		tw.logger.Printf("[INFO] 任务执行完成: id=%s, 描述=%s, 值=%v, 阈值=%v, 触发=%v, 耗时=%v",
			taskID, taskDesc, result.Value, result.Threshold, result.IsFiring, duration)
	}
}

