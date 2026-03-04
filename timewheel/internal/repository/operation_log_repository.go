package repository

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"timewheel/internal/repository/model"
)

// OperationLogRepository 操作日志仓储接口
type OperationLogRepository interface {
	// 创建操作日志
	Create(ctx context.Context, log *model.OperationLogModel) error
	// 更新操作日志状态
	UpdateStatus(ctx context.Context, id uint, status string, errMsg string) error
	// 获取待处理的操作日志（status=rollback_failed）
	GetPendingRepairs(ctx context.Context, limit int) ([]*model.OperationLogModel, error)
	// 获取任务最近的操作日志
	GetLatestByTaskID(ctx context.Context, taskID string) (*model.OperationLogModel, error)
	// 清理旧日志
	CleanupOldLogs(ctx context.Context, before time.Time) error
}

// operationLogRepository 操作日志仓储实现
type operationLogRepository struct {
	db *gorm.DB
}

// NewOperationLogRepository 创建操作日志仓储
func NewOperationLogRepository(db *gorm.DB) OperationLogRepository {
	return &operationLogRepository{db: db}
}

// Create 创建操作日志
func (r *operationLogRepository) Create(ctx context.Context, log *model.OperationLogModel) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// UpdateStatus 更新操作日志状态
func (r *operationLogRepository) UpdateStatus(ctx context.Context, id uint, status string, errMsg string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"status":        status,
		"completed_at":  &now,
		"error_message": errMsg,
	}
	return r.db.WithContext(ctx).Model(&model.OperationLogModel{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// GetPendingRepairs 获取需要修复的操作日志
func (r *operationLogRepository) GetPendingRepairs(ctx context.Context, limit int) ([]*model.OperationLogModel, error) {
	var logs []*model.OperationLogModel
	err := r.db.WithContext(ctx).
		Where("status = ?", model.OpStatusRollbackFailed).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}

// GetLatestByTaskID 获取任务最近的操作日志
func (r *operationLogRepository) GetLatestByTaskID(ctx context.Context, taskID string) (*model.OperationLogModel, error) {
	var log model.OperationLogModel
	err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("created_at DESC").
		First(&log).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}

// CleanupOldLogs 清理旧日志
func (r *operationLogRepository) CleanupOldLogs(ctx context.Context, before time.Time) error {
	return r.db.WithContext(ctx).
		Where("created_at < ? AND status IN ?", before,
			[]string{model.OpStatusCompleted, model.OpStatusFailed}).
		Delete(&model.OperationLogModel{}).Error
}

// SaveStateSnapshot 保存状态快照为JSON
func SaveStateSnapshot(task *model.TaskModel) (json.RawMessage, error) {
	if task == nil {
		return nil, nil
	}
	return json.Marshal(task)
}

// LoadStateSnapshot 从JSON加载状态快照
func LoadStateSnapshot(data json.RawMessage) (*model.TaskModel, error) {
	if data == nil || len(data) == 0 {
		return nil, nil
	}
	var task model.TaskModel
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}
