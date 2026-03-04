package timewheel

import (
	"fmt"
	"sync"
	"time"
)

// MultiLevelTimeWheel 多层时间轮管理器
//
// 设计考虑：
//   - 为不同优先级的任务创建独立的时间轮
//   - 高优先级时间轮使用更细粒度的时间间隔，确保紧急任务更快执行
//   - 低优先级时间轮使用更粗粒度的时间间隔，节省系统资源
//   - 统一管理各时间轮的生命周期
type MultiLevelTimeWheel struct {
	highPriorityTW   *TimeWheel   // 高优先级时间轮
	normalPriorityTW *TimeWheel   // 普通优先级时间轮
	lowPriorityTW    *TimeWheel   // 低优先级时间轮
	poolManager      *PoolManager // 协程池管理器
	mu               sync.RWMutex // 读写锁
	started          bool          // 是否已启动
}

// NewMultiLevelTimeWheel 创建多层时间轮
//
// 返回值：
//   - *MultiLevelTimeWheel: 多层时间轮实例
//   - error: 创建失败时返回错误
func NewMultiLevelTimeWheel() (*MultiLevelTimeWheel, error) {
	// 初始化各优先级时间轮
	highTW, err := New(
		WithSlotNum(60),
		WithInterval(10*time.Millisecond),
	)
	if err != nil {
		return nil, err
	}

	normalTW, err := New(
		WithSlotNum(60),
		WithInterval(100*time.Millisecond),
	)
	if err != nil {
		highTW.Stop()
		return nil, err
	}

	lowTW, err := New(
		WithSlotNum(60),
		WithInterval(1*time.Second),
	)
	if err != nil {
		highTW.Stop()
		normalTW.Stop()
		return nil, err
	}

	// 初始化协程池管理器
	poolManager, err := NewPoolManager()
	if err != nil {
		highTW.Stop()
		normalTW.Stop()
		lowTW.Stop()
		return nil, err
	}

	return &MultiLevelTimeWheel{
		highPriorityTW:   highTW,
		normalPriorityTW: normalTW,
		lowPriorityTW:    lowTW,
		poolManager:      poolManager,
		started:          false,
	}, nil
}

// AddTask 添加任务
//
// 参数：
//   - task: 任务对象
//
// 返回值：
//   - error: 添加失败时返回错误
func (mltw *MultiLevelTimeWheel) AddTask(task *Task) error {
	mltw.mu.Lock()
	defer mltw.mu.Unlock()

	// 参数校验
	if task == nil {
		return ErrInvalidParam
	}
	if task.ID == "" {
		return ErrInvalidParam
	}
	if task.Run == nil {
		return ErrInvalidParam
	}
	// 间隔校验：必须大于0
	if task.Interval <= 0 {
		return fmt.Errorf("task interval must be greater than 0")
	}

	return mltw.addTask(task)
}

// addTask 内部添加任务方法（不获取锁）
func (mltw *MultiLevelTimeWheel) addTask(task *Task) error {
	// 规范化优先级到有效范围
	priority := task.Priority
	if priority < TaskPriorityHigh {
		priority = TaskPriorityHigh
	} else if priority > TaskPriorityLow {
		priority = TaskPriorityLow
	}

	var tw *TimeWheel
	switch priority {
	case TaskPriorityHigh:
		tw = mltw.highPriorityTW
	case TaskPriorityNormal:
		tw = mltw.normalPriorityTW
	case TaskPriorityLow:
		tw = mltw.lowPriorityTW
	default:
		tw = mltw.normalPriorityTW
	}

	// 检查目标时间轮是否在运行
	if !tw.running.Load() {
		return ErrWheelNotRunning
	}

	return tw.AddTask(task)
}

// Start 启动所有时间轮
//
// 返回值：
//   - error: 启动失败时返回错误
func (mltw *MultiLevelTimeWheel) Start() error {
	mltw.mu.Lock()
	defer mltw.mu.Unlock()

	if mltw.started {
		return nil
	}

	// 设置时间轮的协程池管理器
	mltw.highPriorityTW.poolManager = mltw.poolManager
	mltw.normalPriorityTW.poolManager = mltw.poolManager
	mltw.lowPriorityTW.poolManager = mltw.poolManager

	// 启动高优先级时间轮
	if err := mltw.highPriorityTW.Start(); err != nil {
		return err
	}

	// 启动普通优先级时间轮
	if err := mltw.normalPriorityTW.Start(); err != nil {
		mltw.highPriorityTW.Stop()
		return err
	}

	// 启动低优先级时间轮
	if err := mltw.lowPriorityTW.Start(); err != nil {
		mltw.highPriorityTW.Stop()
		mltw.normalPriorityTW.Stop()
		return err
	}

	mltw.started = true
	return nil
}

// Stop 停止所有时间轮
func (mltw *MultiLevelTimeWheel) Stop() {
	mltw.mu.Lock()
	defer mltw.mu.Unlock()

	if !mltw.started {
		return
	}

	mltw.highPriorityTW.Stop()
	mltw.normalPriorityTW.Stop()
	mltw.lowPriorityTW.Stop()

	if mltw.poolManager != nil {
		mltw.poolManager.Release()
	}

	mltw.started = false
}

// RemoveTask 移除任务
//
// 参数：
//   - taskID: 任务ID
//
// 返回值：
//   - error: 移除失败时返回错误
func (mltw *MultiLevelTimeWheel) RemoveTask(taskID string) error {
	mltw.mu.Lock()
	defer mltw.mu.Unlock()

	return mltw.removeTask(taskID)
}

// removeTask 内部移除任务方法（不获取锁）
func (mltw *MultiLevelTimeWheel) removeTask(taskID string) error {
	// 尝试在各个时间轮中移除任务
	err := mltw.highPriorityTW.RemoveTask(taskID)
	if err == nil {
		return nil
	}

	err = mltw.normalPriorityTW.RemoveTask(taskID)
	if err == nil {
		return nil
	}

	return mltw.lowPriorityTW.RemoveTask(taskID)
}

// GetTask 获取任务
//
// 参数：
//   - taskID: 任务ID
//
// 返回值：
//   - *taskSlot: 任务槽位
func (mltw *MultiLevelTimeWheel) GetTask(taskID string) *taskSlot {
	mltw.mu.RLock()
	defer mltw.mu.RUnlock()

	// 尝试在各个时间轮中获取任务
	task := mltw.highPriorityTW.GetTask(taskID)
	if task != nil {
		return task
	}

	task = mltw.normalPriorityTW.GetTask(taskID)
	if task != nil {
		return task
	}

	return mltw.lowPriorityTW.GetTask(taskID)
}

// GetAllTasks 获取所有任务
//
// 返回值：
//   - []*Task: 所有任务列表
func (mltw *MultiLevelTimeWheel) GetAllTasks() []*Task {
	mltw.mu.RLock()
	defer mltw.mu.RUnlock()

	var tasks []*Task
	tasks = append(tasks, mltw.highPriorityTW.GetAllTasks()...)
	tasks = append(tasks, mltw.normalPriorityTW.GetAllTasks()...)
	tasks = append(tasks, mltw.lowPriorityTW.GetAllTasks()...)

	return tasks
}

// ClearAllTasks 清空所有任务
//
// 返回值：
//   - int: 清空的任务数量
func (mltw *MultiLevelTimeWheel) ClearAllTasks() int {
	mltw.mu.Lock()
	defer mltw.mu.Unlock()

	count1 := mltw.highPriorityTW.ClearAllTasks()
	count2 := mltw.normalPriorityTW.ClearAllTasks()
	count3 := mltw.lowPriorityTW.ClearAllTasks()

	return count1 + count2 + count3
}

// PauseTask 暂停任务
//
// 参数：
//   - taskID: 任务ID
//
// 返回值：
//   - error: 暂停失败时返回错误
func (mltw *MultiLevelTimeWheel) PauseTask(taskID string) error {
	mltw.mu.Lock()
	defer mltw.mu.Unlock()

	// 尝试在各个时间轮中暂停任务
	err := mltw.highPriorityTW.PauseTask(taskID)
	if err == nil {
		return nil
	}

	err = mltw.normalPriorityTW.PauseTask(taskID)
	if err == nil {
		return nil
	}

	return mltw.lowPriorityTW.PauseTask(taskID)
}

// ResumeTask 恢复任务
//
// 参数：
//   - taskID: 任务ID
//
// 返回值：
//   - error: 恢复失败时返回错误
func (mltw *MultiLevelTimeWheel) ResumeTask(taskID string) error {
	mltw.mu.Lock()
	defer mltw.mu.Unlock()

	// 尝试在各个时间轮中恢复任务
	err := mltw.highPriorityTW.ResumeTask(taskID)
	if err == nil {
		return nil
	}

	err = mltw.normalPriorityTW.ResumeTask(taskID)
	if err == nil {
		return nil
	}

	return mltw.lowPriorityTW.ResumeTask(taskID)
}

// UpdateTask 更新任务
//
// 参数：
//   - task: 任务对象
//
// 返回值：
//   - error: 更新失败时返回错误
func (mltw *MultiLevelTimeWheel) UpdateTask(task *Task) error {
	mltw.mu.Lock()
	defer mltw.mu.Unlock()

	// 先尝试移除任务
	err := mltw.removeTask(task.ID)
	if err != nil && err != ErrTaskNotFound {
		return err
	}

	// 再添加更新后的任务
	return mltw.addTask(task)
}
