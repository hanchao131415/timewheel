package timewheel

import (
	"context"
	"time"
)

// ============================================================================
// 任务 CRUD 操作
// ============================================================================

// GetTask 获取任务（线程安全）
//
// 优化：优先从缓存获取，避免锁竞争
func (tw *TimeWheel) GetTask(id string) *taskSlot {
	// 优先从缓存获取
	if tw.taskCache != nil {
		if node, ok := tw.taskCache.Get(id); ok {
			tw.totalCacheHits.Add(1)
			return node
		}
		tw.totalCacheMisses.Add(1)
	}

	// 缓存未命中，从分片获取
	shard := tw.getShard(id)
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	return shard.tasks[id]
}

// AddTask 添加定时任务
//
// 请求参数：
//   - task: 任务结构体，不能为空
//
// 返回值：
//   - error: 添加失败时返回错误
//
// 错误列表：
//   - ErrInvalidParam: 任务参数无效
//   - ErrWheelNotRunning: 时间轮未运行
//   - ErrTaskAlreadyExists: 任务ID已存在
//
// 性能考虑：
//   - 使用对象池分配任务节点
//   - O(1)时间复杂度计算槽位索引
func (tw *TimeWheel) AddTask(task *Task) error {
	// 参数校验
	if task == nil {
		tw.logger.Printf("[ERROR] 添加任务失败: %v", ErrInvalidParam)
		return ErrInvalidParam
	}
	if task.ID == "" {
		tw.logger.Printf("[ERROR] 添加任务失败: %v", ErrInvalidParam)
		return ErrInvalidParam
	}
	if task.Run == nil {
		tw.logger.Printf("[ERROR] 添加任务失败: %v", ErrInvalidParam)
		return ErrInvalidParam
	}

	// 根据模式校验参数
	switch task.Mode {
	case TaskModeRepeated:
		// 周期重复模式
		if task.Interval <= 0 {
			tw.logger.Printf("[ERROR] 添加任务失败: %v", ErrIntervalInvalid)
			return WrapError(ErrIntervalInvalid, "task %s", task.ID)
		}
	case TaskModeOnce:
		// 执行一次模式，间隔至少需要1ms（避免除零和立即执行导致的槽位问题）
		if task.Interval <= 0 {
			task.Interval = time.Millisecond // 默认1ms
		}
	case TaskModeFixedTimes:
		// 固定次数模式
		if task.Times <= 0 {
			tw.logger.Printf("[ERROR] 添加任务失败: %v", ErrTimesInvalid)
			return WrapError(ErrTimesInvalid, "task %s", task.ID)
		}
		if task.Interval <= 0 {
			tw.logger.Printf("[ERROR] 添加任务失败: %v", ErrIntervalInvalid)
			return WrapError(ErrIntervalInvalid, "task %s", task.ID)
		}
	default:
		// 默认设置为周期重复模式
		task.Mode = TaskModeRepeated
		if task.Interval <= 0 {
			tw.logger.Printf("[ERROR] 添加任务失败: %v", ErrIntervalInvalid)
			return WrapError(ErrIntervalInvalid, "task %s", task.ID)
		}
	}

	// 检查运行状态
	if !tw.running.Load() {
		tw.logger.Printf("[ERROR] 添加任务失败: %v", ErrWheelNotRunning)
		return ErrWheelNotRunning
	}

	// 计算槽位索引
	now := time.Now()
	delayMs := task.Interval.Milliseconds()
	intervalMs := tw.interval.Milliseconds()
	if intervalMs == 0 {
		intervalMs = 1 // 避免除以零
	}
	slotIndex := int((now.UnixMilli()+delayMs)/intervalMs) % tw.slotNum

	// 从对象池获取任务节点
	node := tw.taskPool.Get().(*taskSlot)
	node.task = task
	node.ctx, node.cancel = context.WithCancel(tw.ctx) // 从时间轮context继承
	node.slotIndex = slotIndex
	node.addedAt = now
	node.runAt = now.Add(task.Interval)
	node.executed = 0 // 初始化执行次数
	node.next = nil
	node.prev = nil

	// 输出任务信息，包括runAt时间
	tw.logger.Printf("[DEBUG] 任务信息: id=%s, 优先级=%d, 间隔=%v, 槽位=%d, runAt=%v",
		task.ID, task.Priority, task.Interval, slotIndex, node.runAt)

	// 重置告警状态字段
	node.alertState = AlertStatePending
	node.pendingSince = time.Time{}
	node.lastFiredAt = time.Time{}
	node.lastResult = AlarmResult{}

	// 添加到任务映射（使用分片锁）
	shard := tw.getShard(task.ID)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	// 检查任务是否已存在
	if _, exists := shard.tasks[task.ID]; exists {
		tw.taskPool.Put(node)
		tw.logger.Printf("[ERROR] 添加任务失败: %v, taskID=%s", ErrTaskAlreadyExists, task.ID)
		return ErrTaskAlreadyExists
	}

	// 使用字符串池优化字符串分配
	if tw.stringPool != nil {
		task.ID = tw.stringPool.Get(task.ID)
		if task.Description != "" {
			task.Description = tw.stringPool.Get(task.Description)
		}
		// 优化标签和注解的字符串
		if task.Labels != nil {
			for k, v := range task.Labels {
				k = tw.stringPool.Get(k)
				v = tw.stringPool.Get(v)
				task.Labels[k] = v
			}
		}
		if task.Annotations != nil {
			for k, v := range task.Annotations {
				k = tw.stringPool.Get(k)
				v = tw.stringPool.Get(v)
				task.Annotations[k] = v
			}
		}
	}

	// 使用写锁保护槽位数组修改（修复竞态条件）
	tw.slotsMu.Lock()

	// 根据优先级插入到槽位链表的合适位置
	// 高优先级任务排在前面
	if tw.slots[slotIndex] == nil {
		// 槽位为空，直接作为头节点
		tw.slots[slotIndex] = node
	} else {
		// 找到第一个优先级比当前任务低的节点
		var prev *taskSlot
		current := tw.slots[slotIndex]
		for current != nil && current.task != nil && current.task.Priority < node.task.Priority {
			prev = current
			current = current.next
		}

		// 插入到找到的位置
		if prev == nil {
			// 插入到链表头部
			node.next = tw.slots[slotIndex]
			tw.slots[slotIndex].prev = node
			tw.slots[slotIndex] = node
		} else {
			// 插入到链表中间或尾部
			node.next = prev.next
			if prev.next != nil {
				prev.next.prev = node
			}
			prev.next = node
			node.prev = prev
		}
	}

	tw.slotsMu.Unlock()

	// 添加到任务映射
	shard.tasks[task.ID] = node

	// 更新缓存（如果启用）
	if tw.taskCache != nil {
		tw.taskCache.Set(task.ID, node)
	}

	// 保存到存储（如果启用）
	if tw.taskStore != nil {
		if err := tw.taskStore.Save(task); err != nil {
			tw.logger.Printf("[WARN] 保存任务失败: id=%s, err=%v", task.ID, err)
		}
	}

	tw.totalTasksAdded.Add(1)
	tw.logger.Printf("[INFO] 添加任务成功: id=%s, 描述=%s, 间隔=%v, 槽位=%d",
		task.ID, task.Description, task.Interval, slotIndex)

	return nil
}

// AddTaskBatch 批量添加任务（优化：减少锁竞争）
//
// 请求参数：
//   - tasks: 任务切片
//
// 返回值：
//   - error: 添加失败时返回错误
func (tw *TimeWheel) AddTaskBatch(tasks []*Task) (int, error) {
	if !tw.running.Load() {
		return 0, ErrWheelNotRunning
	}

	successCount := 0
	var lastErr error

	for _, task := range tasks {
		if err := tw.AddTask(task); err != nil {
			lastErr = err
			continue
		}
		successCount++
	}

	return successCount, lastErr
}

// RemoveTask 移除定时任务
//
// 请求参数：
//   - taskID: 任务ID
//
// 返回值：
//   - error: 移除失败时返回错误
//
// 错误列表：
//   - ErrTaskNotFound: 任务不存在
func (tw *TimeWheel) RemoveTask(taskID string) error {
	if taskID == "" {
		tw.logger.Printf("[ERROR] 移除任务失败: %v", ErrInvalidParam)
		return ErrInvalidParam
	}

	shard := tw.getShard(taskID)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	// 查找任务
	node, exists := shard.tasks[taskID]
	if !exists {
		tw.logger.Printf("[WARN] 移除任务: 任务不存在, id=%s", taskID)
		return ErrTaskNotFound
	}

	// 使用写锁保护槽位数组修改（修复竞态条件）
	tw.slotsMu.Lock()

	// 从双向链表中移除
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		// 是槽位头节点
		tw.slots[node.slotIndex] = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	}

	tw.slotsMu.Unlock()

	// 从映射中删除
	delete(shard.tasks, taskID)

	// 从缓存中删除
	if tw.taskCache != nil {
		tw.taskCache.Delete(taskID)
	}

	// 取消任务context，通知任务停止
	if node.cancel != nil {
		node.cancel()
	}

	// 归还到对象池
	node.task = nil
	node.ctx = nil
	node.cancel = nil
	node.next = nil
	node.prev = nil
	tw.taskPool.Put(node)

	// 从存储删除（如果启用）
	if tw.taskStore != nil {
		if err := tw.taskStore.Delete(taskID); err != nil {
			tw.logger.Printf("[WARN] 删除任务存储失败: id=%s, err=%v", taskID, err)
		}
	}

	tw.totalTasksRemoved.Add(1)
	tw.logger.Printf("[INFO] 移除任务成功: id=%s", taskID)

	return nil
}

// RemoveTaskBatch 批量移除任务（优化：减少锁竞争）
//
// 请求参数：
//   - taskIDs: 任务ID切片
//
// 返回值：
//   - int: 成功移除的数量
func (tw *TimeWheel) RemoveTaskBatch(taskIDs []string) int {
	successCount := 0

	for _, taskID := range taskIDs {
		if err := tw.RemoveTask(taskID); err == nil {
			successCount++
		}
	}

	return successCount
}

// UpdateTask 更新任务
//
// 请求参数：
//   - task: 任务结构体，ID必须与现有任务一致
//
// 返回值：
//   - error: 更新失败时返回错误
//
// 错误列表：
//   - ErrInvalidParam: 任务参数无效
//   - ErrWheelNotRunning: 时间轮未运行
//   - ErrTaskNotFound: 任务不存在
//
// 设计考虑：
//   - 原子操作：保存旧任务副本，失败时自动恢复
func (tw *TimeWheel) UpdateTask(task *Task) error {
	// 参数校验
	if task == nil {
		tw.logger.Printf("[ERROR] 更新任务失败: %v", ErrInvalidParam)
		return ErrInvalidParam
	}
	if task.ID == "" {
		tw.logger.Printf("[ERROR] 更新任务失败: %v", ErrInvalidParam)
		return ErrInvalidParam
	}
	if task.Run == nil {
		tw.logger.Printf("[ERROR] 更新任务失败: %v", ErrInvalidParam)
		return ErrInvalidParam
	}

	// 检查运行状态
	if !tw.running.Load() {
		tw.logger.Printf("[ERROR] 更新任务失败: %v", ErrWheelNotRunning)
		return ErrWheelNotRunning
	}

	// 获取旧任务副本（用于回滚）
	shard := tw.getShard(task.ID)
	shard.mu.RLock()
	oldNode, exists := shard.tasks[task.ID]
	var oldTaskCopy *Task
	if exists && oldNode.task != nil {
		// 复制旧任务
		oldTaskCopy = &Task{
			ID:             oldNode.task.ID,
			Description:    oldNode.task.Description,
			Interval:       oldNode.task.Interval,
			Mode:           oldNode.task.Mode,
			Times:          oldNode.task.Times,
			Priority:       oldNode.task.Priority,
			Run:            oldNode.task.Run,
			Timeout:        oldNode.task.Timeout,
			For:            oldNode.task.For,
			RepeatInterval: oldNode.task.RepeatInterval,
			Severity:       oldNode.task.Severity,
			Labels:         oldNode.task.Labels,
			Annotations:    oldNode.task.Annotations,
		}
	}
	shard.mu.RUnlock()

	if !exists {
		tw.logger.Printf("[ERROR] 更新任务失败: %v, taskID=%s", ErrTaskNotFound, task.ID)
		return ErrTaskNotFound
	}

	// 先移除旧任务
	if err := tw.RemoveTask(task.ID); err != nil {
		tw.logger.Printf("[ERROR] 更新任务失败: 移除旧任务失败: %v", err)
		return err
	}

	// 尝试添加新任务
	if err := tw.AddTask(task); err != nil {
		tw.logger.Printf("[WARN] 更新任务: 添加新任务失败，尝试回滚: %v", err)

		// 尝试恢复旧任务
		if oldTaskCopy != nil {
			if rollbackErr := tw.AddTask(oldTaskCopy); rollbackErr != nil {
				tw.logger.Printf("[ERROR] 更新任务: 回滚失败，任务丢失! id=%s, err=%v", task.ID, rollbackErr)
				return WrapError(err, "update failed and rollback also failed for task %s", task.ID)
			}
			tw.logger.Printf("[INFO] 更新任务: 已回滚到旧任务, id=%s", task.ID)
		}

		return WrapError(err, "add new task failed for task %s", task.ID)
	}

	tw.logger.Printf("[INFO] 更新任务成功: id=%s", task.ID)
	return nil
}

// GetAllTasks 获取所有任务
//
// 返回值：
//   - []*Task: 所有任务列表
func (tw *TimeWheel) GetAllTasks() []*Task {
	var result []*Task

	for i := range tw.shards {
		shard := &tw.shards[i]
		shard.mu.RLock()

		for _, node := range shard.tasks {
			result = append(result, node.task)
		}

		shard.mu.RUnlock()
	}

	return result
}

// PauseTask 暂停任务
//
// 请求参数：
//   - taskID: 任务ID
//
// 返回值：
//   - error: 暂停失败时返回错误
//
// 错误列表：
//   - ErrInvalidParam: 参数无效
//   - ErrTaskNotFound: 任务不存在
func (tw *TimeWheel) PauseTask(taskID string) error {
	if taskID == "" {
		tw.logger.Printf("[ERROR] 暂停任务失败: %v", ErrInvalidParam)
		return ErrInvalidParam
	}

	shard := tw.getShard(taskID)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	node, ok := shard.tasks[taskID]
	if !ok {
		tw.logger.Printf("[ERROR] 暂停任务失败: %v, taskID=%s", ErrTaskNotFound, taskID)
		return ErrTaskNotFound
	}

	if node.paused {
		tw.logger.Printf("[WARN] 任务已处于暂停状态, taskID=%s", taskID)
		return nil
	}

	node.paused = true
	tw.logger.Printf("[INFO] 任务已暂停, taskID=%s", taskID)
	return nil
}

// ResumeTask 恢复任务
//
// 请求参数：
//   - taskID: 任务ID
//
// 返回值：
//   - error: 恢复失败时返回错误
//
// 错误列表：
//   - ErrInvalidParam: 参数无效
//   - ErrTaskNotFound: 任务不存在
func (tw *TimeWheel) ResumeTask(taskID string) error {
	if taskID == "" {
		tw.logger.Printf("[ERROR] 恢复任务失败: %v", ErrInvalidParam)
		return ErrInvalidParam
	}

	shard := tw.getShard(taskID)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	node, ok := shard.tasks[taskID]
	if !ok {
		tw.logger.Printf("[ERROR] 恢复任务失败: %v, taskID=%s", ErrTaskNotFound, taskID)
		return ErrTaskNotFound
	}

	if !node.paused {
		tw.logger.Printf("[WARN] 任务未处于暂停状态, taskID=%s", taskID)
		return nil
	}

	node.paused = false
	if node.task != nil && node.task.Interval > 0 {
		node.runAt = time.Now().Add(node.task.Interval)
	} else {
		node.runAt = time.Now().Add(time.Millisecond) // 默认1ms
	}
	tw.logger.Printf("[INFO] 任务已恢复, taskID=%s", taskID)
	return nil
}

// ClearAllTasks 清空所有任务
//
// 返回值：
//   - int: 清空的任务数量
func (tw *TimeWheel) ClearAllTasks() int {
	clearedCount := 0

	// 遍历所有分片
	for i := range tw.shards {
		shard := &tw.shards[i]
		shard.mu.Lock()

		// 遍历该分片的所有任务
		for taskID, node := range shard.tasks {
			// 从槽位链表中移除（使用写锁保护）
			if node != nil {
				tw.slotsMu.Lock()
				if node.prev != nil {
					node.prev.next = node.next
				} else if node.slotIndex >= 0 && node.slotIndex < len(tw.slots) {
					// 是槽位头节点
					tw.slots[node.slotIndex] = node.next
				}
				if node.next != nil {
					node.next.prev = node.prev
				}
				tw.slotsMu.Unlock()

				// 取消任务context
				if node.cancel != nil {
					node.cancel()
				}

				// 归还到对象池
				node.task = nil
				node.ctx = nil
				node.cancel = nil
				node.next = nil
				node.prev = nil
				node.alertState = AlertStatePending
				node.pendingSince = time.Time{}
				node.lastFiredAt = time.Time{}
				node.lastResult = AlarmResult{}
				tw.taskPool.Put(node)

				// 从映射中删除
				delete(shard.tasks, taskID)
				clearedCount++
			}
		}

		shard.mu.Unlock()
	}

	// 清空缓存（如果启用）
	if tw.taskCache != nil {
		tw.taskCache = NewTaskCache(DefaultCacheSize) // 重新创建缓存
	}

	tw.logger.Printf("[INFO] 已清空所有任务，共清除 %d 个", clearedCount)
	return clearedCount
}

// Stats 获取时间轮统计信息
//
// 返回值：
//   - totalTasks: 任务总数
//   - executedTasks: 已执行任务总数
func (tw *TimeWheel) Stats() (totalTasks int, executedTasks int64) {
	// 遍历所有分片计算总任务数
	for i := range tw.shards {
		shard := &tw.shards[i]
		shard.mu.RLock()
		totalTasks += len(shard.tasks)
		shard.mu.RUnlock()
	}

	return totalTasks, tw.stats.Load()
}
