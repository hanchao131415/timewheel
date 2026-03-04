package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"timewheel/internal/repository/model"
)

// AlertHistoryRepository 告警历史仓储接口
type AlertHistoryRepository interface {
	// 记录告警历史
	Create(ctx context.Context, history *model.AlertHistoryModel) error
	// 获取任务的告警历史
	GetByTaskID(ctx context.Context, taskID string, limit int) ([]*model.AlertHistoryModel, error)
	// 获取告警列表
	List(ctx context.Context, query *AlertHistoryQuery) ([]*model.AlertHistoryModel, int64, error)
	// 获取触发中的告警
	GetFiring(ctx context.Context) ([]*model.AlertHistoryModel, error)
	// 删除指定天数前的历史
	DeleteOlderThan(ctx context.Context, days int) error
	// 更新通知状态
	UpdateNotified(ctx context.Context, id uint, notifiedAt time.Time) error
}

// AlertHistoryQuery 告警历史查询参数
type AlertHistoryQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	TaskID   string `form:"task_id"`
	TaskName string `form:"task_name"`
	State    *int   `form:"state"`
	IsFiring *bool  `form:"is_firing"`
	Notified *bool  `form:"notified"`
}

// alertHistoryRepository 告警历史仓储实现
type alertHistoryRepository struct {
	db *gorm.DB
}

// NewAlertHistoryRepository 创建告警历史仓储
func NewAlertHistoryRepository(db *gorm.DB) AlertHistoryRepository {
	return &alertHistoryRepository{db: db}
}

// Create 记录告警历史
func (r *alertHistoryRepository) Create(ctx context.Context, history *model.AlertHistoryModel) error {
	return r.db.WithContext(ctx).Create(history).Error
}

// GetByTaskID 获取任务的告警历史
func (r *alertHistoryRepository) GetByTaskID(ctx context.Context, taskID string, limit int) ([]*model.AlertHistoryModel, error) {
	var histories []*model.AlertHistoryModel
	query := r.db.WithContext(ctx).Where("task_id = ?", taskID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&histories).Error
	return histories, err
}

// List 获取告警列表
func (r *alertHistoryRepository) List(ctx context.Context, query *AlertHistoryQuery) ([]*model.AlertHistoryModel, int64, error) {
	var histories []*model.AlertHistoryModel
	var total int64

	db := r.db.WithContext(ctx).Model(&model.AlertHistoryModel{})

	// 构建查询条件
	if query.TaskID != "" {
		db = db.Where("task_id = ?", query.TaskID)
	}
	if query.TaskName != "" {
		db = db.Where("task_name LIKE ?", "%"+query.TaskName+"%")
	}
	if query.State != nil {
		db = db.Where("new_state = ?", *query.State)
	}
	if query.IsFiring != nil {
		db = db.Where("is_firing = ?", *query.IsFiring)
	}
	if query.Notified != nil {
		db = db.Where("notified = ?", *query.Notified)
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := 0
	if query.Page > 0 && query.PageSize > 0 {
		offset = (query.Page - 1) * query.PageSize
	}

	if err := db.Order("created_at DESC").Offset(offset).Limit(query.PageSize).Find(&histories).Error; err != nil {
		return nil, 0, err
	}

	return histories, total, nil
}

// GetFiring 获取触发中的告警
func (r *alertHistoryRepository) GetFiring(ctx context.Context) ([]*model.AlertHistoryModel, error) {
	var histories []*model.AlertHistoryModel
	// 获取最近 24 小时内的 firing 告警
	since := time.Now().Add(-24 * time.Hour)
	err := r.db.WithContext(ctx).
		Where("is_firing = ? AND created_at > ?", true, since).
		Order("created_at DESC").
		Find(&histories).Error
	return histories, err
}

// DeleteOlderThan 删除指定天数前的历史
func (r *alertHistoryRepository) DeleteOlderThan(ctx context.Context, days int) error {
	cutoff := time.Now().AddDate(0, 0, -days)
	return r.db.WithContext(ctx).
		Where("created_at < ?", cutoff).
		Delete(&model.AlertHistoryModel{}).Error
}

// UpdateNotified 更新通知状态
func (r *alertHistoryRepository) UpdateNotified(ctx context.Context, id uint, notifiedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.AlertHistoryModel{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"notified":    true,
			"notified_at": &notifiedAt,
		}).Error
}

// WebhookQueueRepository Webhook 队列仓储接口
type WebhookQueueRepository interface {
	// 添加到队列
	Enqueue(ctx context.Context, item *model.WebhookQueueModel) error
	// 获取待处理项
	GetPending(ctx context.Context, limit int) ([]*model.WebhookQueueModel, error)
	// 更新状态
	UpdateStatus(ctx context.Context, id uint, status string, attempts int, errMsg string) error
	// 设置下次尝试时间
	SetNextAttempt(ctx context.Context, id uint, nextAttempt time.Time) error
	// 删除成功/过期项
	Cleanup(ctx context.Context, successRetentionDays, failedRetentionDays int) error
}

// webhookQueueRepository Webhook 队列仓储实现
type webhookQueueRepository struct {
	db *gorm.DB
}

// NewWebhookQueueRepository 创建 Webhook 队列仓储
func NewWebhookQueueRepository(db *gorm.DB) WebhookQueueRepository {
	return &webhookQueueRepository{db: db}
}

// Enqueue 添加到队列
func (r *webhookQueueRepository) Enqueue(ctx context.Context, item *model.WebhookQueueModel) error {
	return r.db.WithContext(ctx).Create(item).Error
}

// GetPending 获取待处理项
func (r *webhookQueueRepository) GetPending(ctx context.Context, limit int) ([]*model.WebhookQueueModel, error) {
	var items []*model.WebhookQueueModel
	now := time.Now()
	err := r.db.WithContext(ctx).
		Where("status = ? AND (next_attempt IS NULL OR next_attempt <= ?)", model.WebhookStatusPending, now).
		Order("created_at ASC").
		Limit(limit).
		Find(&items).Error
	return items, err
}

// UpdateStatus 更新状态
func (r *webhookQueueRepository) UpdateStatus(ctx context.Context, id uint, status string, attempts int, errMsg string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.WebhookQueueModel{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":      status,
			"attempts":    attempts,
			"last_attempt": &now,
			"error_msg":   errMsg,
		}).Error
}

// SetNextAttempt 设置下次尝试时间
func (r *webhookQueueRepository) SetNextAttempt(ctx context.Context, id uint, nextAttempt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.WebhookQueueModel{}).
		Where("id = ?", id).
		Update("next_attempt", &nextAttempt).Error
}

// Cleanup 清理过期项
func (r *webhookQueueRepository) Cleanup(ctx context.Context, successRetentionDays, failedRetentionDays int) error {
	// 删除成功的旧记录
	successCutoff := time.Now().AddDate(0, 0, -successRetentionDays)
	if err := r.db.WithContext(ctx).
		Where("status = ? AND updated_at < ?", model.WebhookStatusSuccess, successCutoff).
		Delete(&model.WebhookQueueModel{}).Error; err != nil {
		return err
	}

	// 删除失败的旧记录
	failedCutoff := time.Now().AddDate(0, 0, -failedRetentionDays)
	return r.db.WithContext(ctx).
		Where("status = ? AND updated_at < ?", model.WebhookStatusFailed, failedCutoff).
		Delete(&model.WebhookQueueModel{}).Error
}
