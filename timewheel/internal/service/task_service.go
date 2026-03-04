package service

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"timewheel/internal/config"
	"timewheel/internal/model/dto"
	"timewheel/internal/repository"
	"timewheel/internal/repository/model"
	timewheelCore "timewheel/pkg/timewheel"
)

// TaskService 任务服务接口
type TaskService interface {
	// 创建任务
	Create(ctx context.Context, req *dto.TaskCreateRequest) (*dto.TaskResponse, error)
	// 获取任务
	GetByID(ctx context.Context, id string) (*dto.TaskResponse, error)
	// 任务列表
	List(ctx context.Context, query *repository.TaskQuery) (*dto.TaskListResponse, error)
	// 更新任务
	Update(ctx context.Context, id string, req *dto.TaskUpdateRequest) (*dto.TaskResponse, error)
	// 删除任务
	Delete(ctx context.Context, id string) error
	// 启用任务
	Enable(ctx context.Context, id string) error
	// 禁用任务
	Disable(ctx context.Context, id string) error
	// 暂停任务
	Pause(ctx context.Context, id string) error
	// 恢复任务
	Resume(ctx context.Context, id string) error
}

// taskService 任务服务实现
type taskService struct {
	repo       repository.TaskRepository
	timeWheel  *timewheelCore.MultiLevelTimeWheel
	cfg        *config.Config
	logger     *zap.Logger
}

// NewTaskService 创建任务服务
func NewTaskService(
	repo repository.TaskRepository,
	timeWheel *timewheelCore.MultiLevelTimeWheel,
	cfg *config.Config,
	logger *zap.Logger,
) TaskService {
	return &taskService{
		repo:      repo,
		timeWheel: timeWheel,
		cfg:       cfg,
		logger:    logger,
	}
}

// Create 创建任务
func (s *taskService) Create(ctx context.Context, req *dto.TaskCreateRequest) (*dto.TaskResponse, error) {
	// 检查任务 ID 是否已存在
	existing, err := s.repo.GetByID(ctx, req.ID)
	if err == nil && existing != nil {
		return nil, errors.New("task with this ID already exists")
	}

	// 转换为数据库模型
	taskModel := req.ToTaskModel()

	// 先保存到数据库
	if err := s.repo.Create(ctx, taskModel); err != nil {
		s.logger.Error("Failed to create task in database",
			zap.String("task_id", req.ID),
			zap.Error(err),
		)
		return nil, err
	}

	// 如果任务启用，添加到时间轮
	if req.Enabled {
		if err := s.addToTimeWheel(taskModel); err != nil {
			s.logger.Warn("Failed to add task to time wheel, rolling back",
				zap.String("task_id", req.ID),
				zap.Error(err),
			)
			// 回滚数据库操作
			_ = s.repo.Delete(ctx, req.ID)
			return nil, err
		}
	}

	s.logger.Info("Task created successfully",
		zap.String("task_id", req.ID),
		zap.Bool("enabled", req.Enabled),
	)

	return dto.ToTaskResponse(taskModel), nil
}

// GetByID 获取任务
func (s *taskService) GetByID(ctx context.Context, id string) (*dto.TaskResponse, error) {
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return dto.ToTaskResponse(task), nil
}

// List 任务列表
func (s *taskService) List(ctx context.Context, query *repository.TaskQuery) (*dto.TaskListResponse, error) {
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}

	tasks, total, err := s.repo.List(ctx, query)
	if err != nil {
		return nil, err
	}

	list := make([]*dto.TaskResponse, 0, len(tasks))
	for _, task := range tasks {
		list = append(list, dto.ToTaskResponse(task))
	}

	return &dto.TaskListResponse{
		List:  list,
		Total: total,
		Page:  query.Page,
		Size:  query.PageSize,
	}, nil
}

// Update 更新任务
func (s *taskService) Update(ctx context.Context, id string, req *dto.TaskUpdateRequest) (*dto.TaskResponse, error) {
	// 获取现有任务
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 更新字段
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Mode != nil {
		existing.Mode = model.TaskMode(*req.Mode)
	}
	if req.IntervalMs != nil {
		existing.IntervalMs = *req.IntervalMs
	}
	if req.Times != nil {
		existing.Times = *req.Times
	}
	if req.Priority != nil {
		existing.Priority = model.TaskPriority(*req.Priority)
	}
	if req.TimeoutMs != nil {
		existing.TimeoutMs = *req.TimeoutMs
	}
	if req.Severity != nil {
		existing.Severity = model.Severity(*req.Severity)
	}
	if req.ForDurationMs != nil {
		existing.ForDurationMs = *req.ForDurationMs
	}
	if req.RepeatIntervalMs != nil {
		existing.RepeatIntervalMs = *req.RepeatIntervalMs
	}
	if req.Labels != nil {
		existing.Labels = *req.Labels
	}
	if req.Annotations != nil {
		existing.Annotations = *req.Annotations
	}

	// 更新数据库
	if err := s.repo.Update(ctx, existing); err != nil {
		s.logger.Error("Failed to update task in database",
			zap.String("task_id", id),
			zap.Error(err),
		)
		return nil, err
	}

	// 更新时间轮中的任务
	if existing.Enabled && !existing.Paused {
		// 先移除旧任务
		_ = s.timeWheel.RemoveTask(id)
		// 添加更新后的任务
		if err := s.addToTimeWheel(existing); err != nil {
			s.logger.Warn("Failed to update task in time wheel",
				zap.String("task_id", id),
				zap.Error(err),
			)
		}
	}

	s.logger.Info("Task updated successfully", zap.String("task_id", id))
	return dto.ToTaskResponse(existing), nil
}

// Delete 删除任务
func (s *taskService) Delete(ctx context.Context, id string) error {
	// 先从时间轮移除
	if err := s.timeWheel.RemoveTask(id); err != nil {
		s.logger.Debug("Task not found in time wheel or already removed",
			zap.String("task_id", id),
		)
	}

	// 从数据库删除
	if err := s.repo.Delete(ctx, id); err != nil {
		s.logger.Error("Failed to delete task from database",
			zap.String("task_id", id),
			zap.Error(err),
		)
		return err
	}

	s.logger.Info("Task deleted successfully", zap.String("task_id", id))
	return nil
}

// Enable 启用任务
func (s *taskService) Enable(ctx context.Context, id string) error {
	// 获取任务
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if task.Enabled {
		return nil // 已经启用
	}

	// 更新数据库状态
	if err := s.repo.UpdateStatus(ctx, id, true, task.Paused); err != nil {
		return err
	}

	// 添加到时间轮
	task.Enabled = true
	if !task.Paused {
		if err := s.addToTimeWheel(task); err != nil {
			s.logger.Error("Failed to add task to time wheel",
				zap.String("task_id", id),
				zap.Error(err),
			)
			// 回滚状态
			_ = s.repo.UpdateStatus(ctx, id, false, task.Paused)
			return err
		}
	}

	s.logger.Info("Task enabled", zap.String("task_id", id))
	return nil
}

// Disable 禁用任务
func (s *taskService) Disable(ctx context.Context, id string) error {
	// 获取任务
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if !task.Enabled {
		return nil // 已经禁用
	}

	// 从时间轮移除
	_ = s.timeWheel.RemoveTask(id)

	// 更新数据库状态
	if err := s.repo.UpdateStatus(ctx, id, false, task.Paused); err != nil {
		return err
	}

	s.logger.Info("Task disabled", zap.String("task_id", id))
	return nil
}

// Pause 暂停任务
func (s *taskService) Pause(ctx context.Context, id string) error {
	// 获取任务
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if task.Paused {
		return nil // 已经暂停
	}

	// 更新数据库状态
	if err := s.repo.UpdateStatus(ctx, id, task.Enabled, true); err != nil {
		return err
	}

	// 暂停时间轮中的任务
	if err := s.timeWheel.PauseTask(id); err != nil {
		s.logger.Debug("Task not found in time wheel",
			zap.String("task_id", id),
		)
	}

	s.logger.Info("Task paused", zap.String("task_id", id))
	return nil
}

// Resume 恢复任务
func (s *taskService) Resume(ctx context.Context, id string) error {
	// 获取任务
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if !task.Paused {
		return nil // 未暂停
	}

	// 更新数据库状态
	if err := s.repo.UpdateStatus(ctx, id, task.Enabled, false); err != nil {
		return err
	}

	// 恢复时间轮中的任务
	if task.Enabled {
		if err := s.timeWheel.ResumeTask(id); err != nil {
			// 如果时间轮中不存在，重新添加
			if err := s.addToTimeWheel(task); err != nil {
				s.logger.Error("Failed to add task to time wheel",
					zap.String("task_id", id),
					zap.Error(err),
				)
				return err
			}
		}
	}

	s.logger.Info("Task resumed", zap.String("task_id", id))
	return nil
}

// addToTimeWheel 添加任务到时间轮
func (s *taskService) addToTimeWheel(m *model.TaskModel) error {
	// 创建时间轮任务
	task := &timewheelCore.Task{
		ID:          m.ID,
		Description: m.Name + " - " + m.Description, // 将名称合并到描述中
		Mode:        timewheelCore.TaskMode(m.Mode),
		Interval:    time.Duration(m.IntervalMs) * time.Millisecond,
		Times:       m.Times,
		Priority:    timewheelCore.TaskPriority(m.Priority),
		Timeout:     time.Duration(m.TimeoutMs) * time.Millisecond,
		Severity:    timewheelCore.Severity(m.Severity),
		For:         time.Duration(m.ForDurationMs) * time.Millisecond,
		RepeatInterval: time.Duration(m.RepeatIntervalMs) * time.Millisecond,
		Labels:      m.Labels,
		Annotations: m.Annotations,
	}

	// Run 函数需要在应用层注入
	// 这里创建一个占位符，实际执行时会从任务注册表中获取
	task.Run = func(ctx context.Context) timewheelCore.AlarmResult {
		// 由调用方通过回调或注册表提供实际执行逻辑
		return timewheelCore.AlarmResult{}
	}

	return s.timeWheel.AddTask(task)
}
