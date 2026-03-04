package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"timewheel/internal/repository/model"
)

// TaskRepository 任务仓储接口
type TaskRepository interface {
	// 创建任务
	Create(ctx context.Context, task *model.TaskModel) error
	// 根据 ID 获取任务
	GetByID(ctx context.Context, id string) (*model.TaskModel, error)
	// 获取任务列表
	List(ctx context.Context, query *TaskQuery) ([]*model.TaskModel, int64, error)
	// 更新任务
	Update(ctx context.Context, task *model.TaskModel) error
	// 删除任务（软删除）
	Delete(ctx context.Context, id string) error
	// 恢复软删除的任务（用于回滚）
	Undelete(ctx context.Context, task *model.TaskModel) error
	// 获取所有启用的任务
	GetEnabled(ctx context.Context) ([]*model.TaskModel, error)
	// 更新任务状态
	UpdateStatus(ctx context.Context, id string, enabled, paused bool) error
	// 更新执行统计
	UpdateExecutionStats(ctx context.Context, id string, resultValue float64, isFiring bool) error
}

// TaskQuery 任务查询参数
type TaskQuery struct {
	Page      int    `form:"page"`
	PageSize  int    `form:"page_size"`
	Name      string `form:"name"`
	Enabled   *bool  `form:"enabled"`
	Paused    *bool  `form:"paused"`
	Priority  *int   `form:"priority"`
	Mode      *int   `form:"mode"`
	AlertState *int  `form:"alert_state"`
}

// taskRepository 任务仓储实现
type taskRepository struct {
	db *gorm.DB
}

// NewTaskRepository 创建任务仓储
func NewTaskRepository(db *gorm.DB) TaskRepository {
	return &taskRepository{db: db}
}

// Create 创建任务
func (r *taskRepository) Create(ctx context.Context, task *model.TaskModel) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// GetByID 根据 ID 获取任务
func (r *taskRepository) GetByID(ctx context.Context, id string) (*model.TaskModel, error) {
	var task model.TaskModel
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// List 获取任务列表
func (r *taskRepository) List(ctx context.Context, query *TaskQuery) ([]*model.TaskModel, int64, error) {
	var tasks []*model.TaskModel
	var total int64

	db := r.db.WithContext(ctx).Model(&model.TaskModel{})

	// 构建查询条件
	if query.Name != "" {
		db = db.Where("name LIKE ?", "%"+query.Name+"%")
	}
	if query.Enabled != nil {
		db = db.Where("enabled = ?", *query.Enabled)
	}
	if query.Paused != nil {
		db = db.Where("paused = ?", *query.Paused)
	}
	if query.Priority != nil {
		db = db.Where("priority = ?", *query.Priority)
	}
	if query.Mode != nil {
		db = db.Where("mode = ?", *query.Mode)
	}
	if query.AlertState != nil {
		db = db.Where("alert_state = ?", *query.AlertState)
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

	if err := db.Order("created_at DESC").Offset(offset).Limit(query.PageSize).Find(&tasks).Error; err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
}

// Update 更新任务
func (r *taskRepository) Update(ctx context.Context, task *model.TaskModel) error {
	return r.db.WithContext(ctx).Save(task).Error
}

// Delete 删除任务
func (r *taskRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.TaskModel{}, "id = ?", id).Error
}

// Undelete 恢复软删除的任务（用于回滚）
func (r *taskRepository) Undelete(ctx context.Context, task *model.TaskModel) error {
	// 清除 DeletedAt 字段以恢复记录
	task.DeletedAt = gorm.DeletedAt{}
	return r.db.WithContext(ctx).Unscoped().Model(&model.TaskModel{}).
		Where("id = ?", task.ID).
		Update("deleted_at", nil).Error
}

// GetEnabled 获取所有启用的任务
func (r *taskRepository) GetEnabled(ctx context.Context) ([]*model.TaskModel, error) {
	var tasks []*model.TaskModel
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Find(&tasks).Error
	return tasks, err
}

// UpdateStatus 更新任务状态
func (r *taskRepository) UpdateStatus(ctx context.Context, id string, enabled, paused bool) error {
	updates := map[string]interface{}{
		"enabled": enabled,
		"paused":  paused,
	}

	now := time.Now()
	if enabled {
		updates["enabled_at"] = &now
	}
	if paused {
		updates["paused_at"] = &now
	}

	return r.db.WithContext(ctx).Model(&model.TaskModel{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateExecutionStats 更新执行统计
func (r *taskRepository) UpdateExecutionStats(ctx context.Context, id string, resultValue float64, isFiring bool) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.TaskModel{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"executed_count":    gorm.Expr("executed_count + 1"),
			"last_executed_at":  &now,
			"last_result_value": resultValue,
			"last_is_firing":    isFiring,
		}).Error
}
