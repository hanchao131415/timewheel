package timewheel

import (
	"log"
	"runtime"

	"github.com/panjf2000/ants/v2"
)

// PoolManager 协程池管理器
//
// 设计考虑：
//   - 为不同优先级的任务提供不同大小的协程池
//   - 高优先级任务使用更大的协程池，确保及时执行
//   - 低优先级任务使用较小的协程池，节省系统资源
type PoolManager struct {
	highPriorityPool   *ants.Pool
	normalPriorityPool *ants.Pool
	lowPriorityPool    *ants.Pool
}

// NewPoolManager 创建协程池管理器
//
// 返回值：
//   - *PoolManager: 协程池管理器实例
//   - error: 创建失败时返回错误
func NewPoolManager() (*PoolManager, error) {
	cpuCount := runtime.GOMAXPROCS(0)

	// 高优先级协程池：CPU核心数的3倍
	highPool, err := ants.NewPool(cpuCount*3, ants.WithPanicHandler(func(panic interface{}) {
		log.Printf("[PANIC] 高优先级协程池发生panic: %v", panic)
	}))
	if err != nil {
		return nil, err
	}

	// 普通优先级协程池：CPU核心数
	normalPool, err := ants.NewPool(cpuCount, ants.WithPanicHandler(func(panic interface{}) {
		log.Printf("[PANIC] 普通优先级协程池发生panic: %v", panic)
	}))
	if err != nil {
		highPool.Release()
		return nil, err
	}

	// 低优先级协程池：CPU核心数的1/2
	lowPool, err := ants.NewPool(max(1, cpuCount/2), ants.WithPanicHandler(func(panic interface{}) {
		log.Printf("[PANIC] 低优先级协程池发生panic: %v", panic)
	}))
	if err != nil {
		highPool.Release()
		normalPool.Release()
		return nil, err
	}

	return &PoolManager{
		highPriorityPool:   highPool,
		normalPriorityPool: normalPool,
		lowPriorityPool:    lowPool,
	}, nil
}

// Execute 执行任务
//
// 参数：
//   - task: 任务对象（可为nil，此时使用普通优先级池）
//   - f: 任务执行函数
//
// 返回值：
//   - error: 执行失败时返回错误
func (pm *PoolManager) Execute(task *Task, f func()) error {
	// nil task 使用普通优先级池
	if task == nil {
		if pm.normalPriorityPool == nil {
			return ErrInvalidParam
		}
		return pm.normalPriorityPool.Submit(f)
	}

	// 根据任务优先级选择对应的协程池
	var pool *ants.Pool
	switch task.Priority {
	case TaskPriorityHigh:
		pool = pm.highPriorityPool
	case TaskPriorityLow:
		pool = pm.lowPriorityPool
	default:
		pool = pm.normalPriorityPool
	}

	// 检查协程池是否有效
	if pool == nil {
		return ErrInvalidParam
	}

	return pool.Submit(f)
}

// Release 释放协程池资源
func (pm *PoolManager) Release() {
	if pm.highPriorityPool != nil {
		pm.highPriorityPool.Release()
	}
	if pm.normalPriorityPool != nil {
		pm.normalPriorityPool.Release()
	}
	if pm.lowPriorityPool != nil {
		pm.lowPriorityPool.Release()
	}
}
