package timewheel

import (
	"sync"
)

// ============================================================================
// 分片锁结构
// ============================================================================

// TaskMapShard 分片锁结构
type TaskMapShard struct {
	mu    sync.RWMutex
	tasks map[string]*taskSlot
}

// ============================================================================
// 分片锁辅助函数
// ============================================================================

// getShard 根据任务ID获取对应的分片
func (tw *TimeWheel) getShard(taskID string) *TaskMapShard {
	h := fnv64(taskID)
	return &tw.shards[h%uint64(tw.shardNum)]
}

// fnv64 FNV-1a 64位哈希函数
func fnv64(key string) uint64 {
	hash := uint64(2166136261)
	for i := 0; i < len(key); i++ {
		hash ^= uint64(key[i])
		hash *= 16777619
	}
	return hash
}

// ============================================================================
// 任务节点对象池
// ============================================================================

// newTaskPool 创建任务节点对象池
func newTaskPool() sync.Pool {
	return sync.Pool{
		New: func() interface{} {
			return &taskSlot{}
		},
	}
}
