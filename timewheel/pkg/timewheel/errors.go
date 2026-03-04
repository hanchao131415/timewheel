package timewheel

import (
	"errors"
	"fmt"
)

// ============================================================================
// 错误定义
// ============================================================================

var (
	// --- 时间轮配置错误 ---

	// ErrIntervalTooSmall 时间间隔必须大于0
	ErrIntervalTooSmall = errors.New("interval must be greater than 0")
	// ErrSlotsTooFew 槽位数量必须大于0
	ErrSlotsTooFew = errors.New("slots must be greater than 0")

	// --- 任务相关错误 ---

	// ErrTaskNotFound 任务未找到
	ErrTaskNotFound = errors.New("task not found")
	// ErrTaskAlreadyExists 任务ID已存在
	ErrTaskAlreadyExists = errors.New("task already exists")
	// ErrTaskIntervalInvalid 任务间隔无效
	ErrTaskIntervalInvalid = errors.New("task interval must be greater than 0")
	// ErrTaskTimesInvalid 任务次数无效（FixedTimes模式）
	ErrTaskTimesInvalid = errors.New("task times must be greater than 0 for FixedTimes mode")

	// --- 运行状态错误 ---

	// ErrWheelNotRunning 时间轮未运行
	ErrWheelNotRunning = errors.New("timewheel is not running")

	// --- 参数错误 ---

	// ErrInvalidParam 参数无效
	ErrInvalidParam = errors.New("invalid parameter")

	// --- 上下文错误 ---

	// ErrContextCanceled 上下文已取消
	ErrContextCanceled = errors.New("context canceled")
	// ErrContextNil 上下文为空
	ErrContextNil = errors.New("task context is nil")

	// --- 已废弃（保留向后兼容） ---

	// ErrIntervalInvalid 已废弃，使用 ErrTaskIntervalInvalid
	// Deprecated: Use ErrTaskIntervalInvalid instead
	ErrIntervalInvalid = ErrTaskIntervalInvalid
	// ErrTimesInvalid 已废弃，使用 ErrTaskTimesInvalid
	// Deprecated: Use ErrTaskTimesInvalid instead
	ErrTimesInvalid = ErrTaskTimesInvalid
	// ErrIntervalInvalidForMode 已废弃，使用 ErrTaskIntervalInvalid
	// Deprecated: Use ErrTaskIntervalInvalid instead
	ErrIntervalInvalidForMode = ErrTaskIntervalInvalid
)

// WrapError 包装错误并添加上下文信息
func WrapError(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
}
