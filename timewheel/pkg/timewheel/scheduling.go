package timewheel

import (
	"time"
)

// ============================================================================
// 任务调度相关方法
// ============================================================================

// handleTaskSchedulingLocked 处理任务调度（调用前必须持有分片锁）
func (tw *TimeWheel) handleTaskSchedulingLocked(node *taskSlot, taskInfo *taskInfo, now time.Time, currentSlot int) {
	// 再次检查，避免在获取锁的过程中任务被移除
	if node.task == nil {
		return
	}

	// 根据任务模式决定是否重新调度
	shouldReschedule := tw.shouldRescheduleTask(node, taskInfo)

	// 如果需要重新调度
	if shouldReschedule {
		tw.rescheduleTask(node, taskInfo, now, currentSlot)
	} else {
		tw.removeTask(node, taskInfo, currentSlot)
	}
}

// shouldRescheduleTask 判断是否需要重新调度任务
func (tw *TimeWheel) shouldRescheduleTask(node *taskSlot, taskInfo *taskInfo) bool {
	switch taskInfo.Mode {
	case TaskModeOnce:
		// 执行一次，无论成功失败都不再调度
		tw.logger.Printf("[INFO] 任务执行一次模式，执行完成后移除: id=%s", taskInfo.ID)
		return false
	case TaskModeFixedTimes:
		// 固定次数模式，检查是否还有剩余次数（无论成功失败都计数）
		if node.executed < taskInfo.Times {
			tw.debug("任务固定次数模式，已执行=%d, 目标次数=%d, 继续调度",
				node.executed, taskInfo.Times)
			return true
		} else {
			tw.logger.Printf("[INFO] 任务固定次数模式，执行完成，已执行=%d, 目标次数=%d",
				node.executed, taskInfo.Times)
			return false
		}
	case TaskModeRepeated:
		// 周期重复模式，无论成功失败都继续调度
		return true
	default:
		// 默认视为周期重复
		return true
	}
}

// rescheduleTask 重新调度任务
func (tw *TimeWheel) rescheduleTask(node *taskSlot, taskInfo *taskInfo, now time.Time, currentSlot int) {
	// 重新计算槽位
	delayMs := taskInfo.Interval.Milliseconds()
	intervalMs := tw.interval.Milliseconds()
	if intervalMs == 0 {
		intervalMs = 1 // 避免除以零
	}
	newSlotIndex := int((now.UnixMilli()+delayMs)/intervalMs) % tw.slotNum

	// 更新任务节点
	node.runAt = now.Add(taskInfo.Interval)

	// 使用写锁保护槽位数组修改（修复竞态条件）
	tw.slotsMu.Lock()
	defer tw.slotsMu.Unlock()

	// 只有当槽位发生变化时才进行槽位操作
	if node.slotIndex != newSlotIndex {
		// 从当前槽位移除
		if node.prev != nil {
			node.prev.next = node.next
		} else {
			// 是槽位头节点
			tw.slots[currentSlot] = node.next
		}
		if node.next != nil {
			node.next.prev = node.prev
		}

		// 添加到新槽位
		node.next = tw.slots[newSlotIndex]
		if tw.slots[newSlotIndex] != nil {
			tw.slots[newSlotIndex].prev = node
		}
		tw.slots[newSlotIndex] = node
		node.prev = nil
		node.slotIndex = newSlotIndex
		tw.debug("任务已重新调度: id=%s, 新槽位=%d, 下次执行=%v",
			taskInfo.ID, newSlotIndex, node.runAt)
	} else {
		// 槽位未变化，只更新执行时间
		node.slotIndex = newSlotIndex // 确保槽位索引正确
		tw.debug("任务槽位未变化，只更新执行时间: id=%s, 槽位=%d, 下次执行=%v",
			taskInfo.ID, newSlotIndex, node.runAt)
	}
}

// removeTask 移除任务（内部方法，调用前必须持有分片锁）
func (tw *TimeWheel) removeTask(node *taskSlot, taskInfo *taskInfo, currentSlot int) {
	// 使用写锁保护槽位数组修改（修复竞态条件）
	tw.slotsMu.Lock()

	// 从槽位链表中移除
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		// 是槽位头节点
		tw.slots[currentSlot] = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	}

	tw.slotsMu.Unlock()

	// 从映射中删除（分片锁已由调用者持有）
	shard := tw.getShard(taskInfo.ID)
	delete(shard.tasks, taskInfo.ID)

	// 从缓存中删除
	if tw.taskCache != nil {
		tw.taskCache.Delete(taskInfo.ID)
	}

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

	tw.logger.Printf("[INFO] 任务执行完成并移除: id=%s, 已执行次数=%d", taskInfo.ID, node.executed)
}

// executeTask 执行任务
//
// 参数：
//   - node: 任务节点
//   - now: 当前时间
//   - currentSlot: 当前槽位索引
func (tw *TimeWheel) executeTask(node *taskSlot, now time.Time, currentSlot int) {
	// 用于标记是否发生了panic，并保存panic值
	var panicValue interface{}
	// 标记是否获取了信号量
	semaphoreAcquired := false

	// 捕获panic，防止用户任务导致时间轮崩溃
	defer func() {
		if r := recover(); r != nil {
			panicValue = r
			// 防止repanic
			func() {
				defer func() {
					_ = recover() // 吞掉任何二次panic
				}()

				taskID := "unknown"
				if node.task != nil {
					taskID = node.task.ID
				}
				tw.logger.Printf("[PANIC] 任务执行发生panic，已恢复: id=%s, panic=%v",
					taskID, r)
			}()
		}

		// 释放信号量（如果启用了并发控制且已获取）
		if tw.taskSemaphore != nil && semaphoreAcquired {
			<-tw.taskSemaphore
			running := tw.runningTasks.Add(-1)
			tw.debug("任务执行完成，释放信号量，当前运行任务数: %d", running)
		}

		// 始终调用 Done，因为 tick() 中已经调用了 Add(1)
		tw.taskWg.Done()
	}()

	// 获取信号量（如果启用了并发控制）
	if !tw.acquireSemaphore(&semaphoreAcquired) {
		return
	}

	// 检查任务状态
	if !tw.checkTaskState(node) {
		return
	}

	// 复制任务信息
	taskInfo := tw.copyTaskInfo(node)

	// 执行任务
	result, duration, err := tw.runTask(node, taskInfo)

	// 更新任务执行结果
	tw.updateTaskResult(node, result, duration, err, panicValue)

	// 检查任务是否在执行过程中被移除
	if node.task == nil {
		return
	}

	// 一次性获取锁，完成所有状态更新（修复双重锁问题）
	shard := tw.getShard(taskInfo.ID)
	shard.mu.Lock()

	// 处理告警状态（不再单独加锁）
	tw.handleAlertStateLocked(node, taskInfo, result, now)

	// 处理任务调度（不再单独加锁）
	tw.handleTaskSchedulingLocked(node, taskInfo, now, currentSlot)

	shard.mu.Unlock()
}
