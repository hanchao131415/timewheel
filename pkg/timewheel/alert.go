package timewheel

import (
	"time"
)

// ============================================================================
// 告警状态机
// ============================================================================

// computeAlertState 计算告警状态
//
// 状态转换规则：
//   - Pending: 条件满足但未达到持续时间
//   - Firing: 条件满足且达到持续时间
//   - Resolved: 从 Firing 转为不满足条件
func (ts *taskSlot) computeAlertState(isFiring bool, forDuration time.Duration, now time.Time) AlertState {
	if isFiring {
		// 条件满足
		if ts.alertState == AlertStateFiring {
			// 已经是 Firing 状态，保持
			return AlertStateFiring
		}

		// 如果 For=0，直接进入 Firing（无需等待）
		if forDuration <= 0 {
			ts.lastFiredAt = now
			ts.pendingSince = time.Time{}
			return AlertStateFiring
		}

		// 首次触发或从 Resolved 回来
		if ts.pendingSince.IsZero() {
			// 首次满足条件，进入 Pending
			ts.pendingSince = now
			return AlertStatePending
		}
		// 检查是否达到持续时间
		if now.Sub(ts.pendingSince) >= forDuration {
			// 达到持续时间，进入 Firing
			ts.lastFiredAt = now
			ts.pendingSince = time.Time{} // 重置
			return AlertStateFiring
		}
		// 未达到持续时间，保持 Pending
		return AlertStatePending
	}

	// 条件不满足
	if ts.alertState == AlertStateFiring {
		// 从 Firing 转为不满足，进入 Resolved
		ts.pendingSince = time.Time{} // 重置
		return AlertStateResolved
	}
	// 从 Pending 或 Resolved 转为不满足，重置
	ts.pendingSince = time.Time{}
	return AlertStatePending
}

// handleAlertStateLocked 处理告警状态（调用前必须持有分片锁）
func (tw *TimeWheel) handleAlertStateLocked(node *taskSlot, taskInfo *taskInfo, result AlarmResult, now time.Time) {
	// 再次检查，避免在获取锁的过程中任务被移除
	if node.task == nil {
		return
	}

	oldState := node.alertState
	newState := node.computeAlertState(result.IsFiring, taskInfo.For, now)
	node.alertState = newState

	// 如果状态发生变化，记录历史
	if oldState != newState {
		tw.logger.Printf("[INFO] 告警状态变化: id=%s, %v -> %v", taskInfo.ID, oldState, newState)

		// 记录告警历史（如果有历史管理器）
		if tw.historyManager != nil {
			tw.historyManager.Record(taskInfo.ID, oldState, newState, result, taskInfo.Severity, taskInfo.Labels, taskInfo.Annotations)
		}

		// 记录到历史存储（如果有）
		if tw.historyStore != nil {
			historyRecord := AlertHistory{
				TaskID:      taskInfo.ID,
				OldState:    oldState,
				State:       newState,
				Timestamp:   now,
				Value:       result.Value,
				Threshold:   result.Threshold,
				IsFiring:    result.IsFiring,
				Severity:    taskInfo.Severity,
			}
			if err := tw.historyStore.Record(historyRecord); err != nil {
				tw.logger.Printf("[WARN] 记录告警历史失败: %v", err)
			}
		}

		// 调用状态变化回调（如果有）
		if tw.onAlertStateChange != nil {
			tw.onAlertStateChange(taskInfo.ID, oldState, newState, result)
		}

		// 如果变为 Firing 状态，检查是否需要重复告警
		if newState == AlertStateFiring && taskInfo.RepeatInterval > 0 {
			// 增加告警触发计数
			tw.totalAlertsFired.Add(1)
			// 设置下次重复告警时间
			node.lastFiredAt = now
		} else if newState == AlertStateFiring {
			// 增加告警触发计数
			tw.totalAlertsFired.Add(1)
		}
	} else if newState == AlertStateFiring && taskInfo.RepeatInterval > 0 {
		// 持续处于 Firing 状态，检查是否达到重复告警间隔
		if !node.lastFiredAt.IsZero() && now.Sub(node.lastFiredAt) >= taskInfo.RepeatInterval {
			// 触发重复告警
			tw.totalAlertsFired.Add(1)
			tw.logger.Printf("[INFO] 重复告警: id=%s, 距上次=%v, 间隔=%v",
				taskInfo.ID, now.Sub(node.lastFiredAt), taskInfo.RepeatInterval)

			// 记录重复告警历史
			if tw.historyManager != nil {
				tw.historyManager.Record(taskInfo.ID, oldState, newState, result, taskInfo.Severity, taskInfo.Labels, taskInfo.Annotations)
			}

			// 调用状态变化回调（通知重复告警）
			if tw.onAlertStateChange != nil {
				tw.onAlertStateChange(taskInfo.ID, oldState, newState, result)
			}

			// 更新上次触发时间
			node.lastFiredAt = now
		}
	}
}
