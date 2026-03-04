package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"timewheel/internal/config"
	"timewheel/internal/model/dto"
	"timewheel/internal/repository"
	"timewheel/internal/repository/model"
	timewheelCore "timewheel/pkg/timewheel"
)

// AtomicTaskService 原子性任务服务接口
type AtomicTaskService interface {
	TaskService
	// GetPendingOperations 获取待处理的操作（用于恢复）
	GetPendingOperations(ctx context.Context) ([]*model.OperationLogModel, error)
	// RepairInconsistentState 修复不一致状态
	RepairInconsistentState(ctx context.Context) error
}

// atomicTaskService 原子性任务服务实现
type atomicTaskService struct {
	repo         repository.TaskRepository
	opLogRepo    repository.OperationLogRepository
	timeWheel    *timewheelCore.MultiLevelTimeWheel
	cfg          *config.Config
	logger       *zap.Logger

	// 任务级锁，保证同一任务的操作串行化
	taskLocks    sync.Map // map[string]*sync.Mutex
}

// NewAtomicTaskService 创建原子性任务服务
func NewAtomicTaskService(
	repo repository.TaskRepository,
	opLogRepo repository.OperationLogRepository,
	timeWheel *timewheelCore.MultiLevelTimeWheel,
	cfg *config.Config,
	logger *zap.Logger,
) AtomicTaskService {
	return &atomicTaskService{
		repo:      repo,
		opLogRepo: opLogRepo,
		timeWheel: timeWheel,
		cfg:       cfg,
		logger:    logger,
	}
}

// getTaskLock 获取任务锁
func (s *atomicTaskService) getTaskLock(taskID string) *sync.Mutex {
	lock, _ := s.taskLocks.LoadOrStore(taskID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// executeAtomically 原子执行操作
// opType: 操作类型
// oldState: 操作前的状态快照
// dbOp: 数据库操作
// twOp: 时间轮操作
// rollbackDB: 回滚数据库操作
func (s *atomicTaskService) executeAtomically(
	ctx context.Context,
	taskID string,
	opType string,
	oldState *model.TaskModel,
	dbOp func() error,
	twOp func() error,
	rollbackDB func() error,
) error {
	// 1. 获取任务锁，保证同一任务操作串行化
	lock := s.getTaskLock(taskID)
	lock.Lock()
	defer lock.Unlock()

	// 2. 保存状态快照
	var oldStateJSON json.RawMessage
	var err error
	if oldState != nil {
		oldStateJSON, err = json.Marshal(oldState)
		if err != nil {
			return fmt.Errorf("failed to marshal old state: %w", err)
		}
	}

	// 3. 记录操作日志（pending状态）
	opLog := &model.OperationLogModel{
		TaskID:    taskID,
		Operation: opType,
		Status:    model.OpStatusPending,
		OldState:  oldStateJSON,
	}
	if err := s.opLogRepo.Create(ctx, opLog); err != nil {
		return fmt.Errorf("failed to log operation: %w", err)
	}

	// 4. 执行数据库操作
	if err := dbOp(); err != nil {
		s.opLogRepo.UpdateStatus(ctx, opLog.ID, model.OpStatusFailed, err.Error())
		return err
	}

	// 5. 执行时间轮操作
	if err := twOp(); err != nil {
		s.logger.Warn("TimeWheel operation failed, attempting rollback",
			zap.String("task_id", taskID),
			zap.String("operation", opType),
			zap.Error(err))

		// 6. 时间轮失败，回滚数据库
		if rollbackErr := rollbackDB(); rollbackErr != nil {
			s.logger.Error("Rollback failed! Data may be inconsistent",
				zap.String("task_id", taskID),
				zap.String("operation", opType),
				zap.Error(rollbackErr))

			// 记录回滚失败，需要人工或后台修复
			errMsg := fmt.Sprintf("TW error: %v, Rollback error: %v", err, rollbackErr)
			s.opLogRepo.UpdateStatus(ctx, opLog.ID, model.OpStatusRollbackFailed, errMsg)
			return fmt.Errorf("operation failed and rollback also failed: %w (rollback: %v)", err, rollbackErr)
		}

		// 回滚成功，记录失败状态
		s.opLogRepo.UpdateStatus(ctx, opLog.ID, model.OpStatusFailed, err.Error())
		return err
	}

	// 7. 操作成功，更新日志状态
	s.opLogRepo.UpdateStatus(ctx, opLog.ID, model.OpStatusCompleted, "")
	return nil
}

// Create 创建任务（原子操作）
func (s *atomicTaskService) Create(ctx context.Context, req *dto.TaskCreateRequest) (*dto.TaskResponse, error) {
	// 检查任务 ID 是否已存在
	existing, err := s.repo.GetByID(ctx, req.ID)
	if err == nil && existing != nil {
		return nil, errors.New("task with this ID already exists")
	}

	taskModel := req.ToTaskModel()

	var createdTask *model.TaskModel

	err = s.executeAtomically(
		ctx,
		req.ID,
		model.OpCreate,
		nil, // 没有旧状态
		// 数据库操作
		func() error {
			if err := s.repo.Create(ctx, taskModel); err != nil {
				return err
			}
			createdTask = taskModel
			return nil
		},
		// 时间轮操作
		func() error {
			if req.Enabled {
				return s.addToTimeWheel(taskModel)
			}
			return nil
		},
		// 回滚数据库
		func() error {
			return s.repo.Delete(ctx, req.ID)
		},
	)

	if err != nil {
		return nil, err
	}

	return dto.ToTaskResponse(createdTask), nil
}

// Delete 删除任务（原子操作）
func (s *atomicTaskService) Delete(ctx context.Context, id string) error {
	// 获取任务快照
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	return s.executeAtomically(
		ctx,
		id,
		model.OpDelete,
		task, // 保存旧状态用于回滚
		// 数据库操作
		func() error {
			return s.repo.Delete(ctx, id)
		},
		// 时间轮操作
		func() error {
			return s.timeWheel.RemoveTask(id)
		},
		// 回滚数据库（恢复软删除）
		func() error {
			return s.repo.Undelete(ctx, task)
		},
	)
}

// Enable 启用任务（原子操作）
func (s *atomicTaskService) Enable(ctx context.Context, id string) error {
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if task.Enabled {
		return nil // 已经启用
	}

	return s.executeAtomically(
		ctx,
		id,
		model.OpEnable,
		task,
		// 数据库操作
		func() error {
			return s.repo.UpdateStatus(ctx, id, true, task.Paused)
		},
		// 时间轮操作
		func() error {
			task.Enabled = true
			if !task.Paused {
				return s.addToTimeWheel(task)
			}
			return nil
		},
		// 回滚数据库
		func() error {
			return s.repo.UpdateStatus(ctx, id, false, task.Paused)
		},
	)
}

// Disable 禁用任务（原子操作）
func (s *atomicTaskService) Disable(ctx context.Context, id string) error {
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if !task.Enabled {
		return nil // 已经禁用
	}

	return s.executeAtomically(
		ctx,
		id,
		model.OpDisable,
		task,
		// 数据库操作
		func() error {
			return s.repo.UpdateStatus(ctx, id, false, task.Paused)
		},
		// 时间轮操作
		func() error {
			return s.timeWheel.RemoveTask(id)
		},
		// 回滚数据库
		func() error {
			return s.repo.UpdateStatus(ctx, id, true, task.Paused)
		},
	)
}

// Pause 暂停任务（原子操作）
func (s *atomicTaskService) Pause(ctx context.Context, id string) error {
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if task.Paused {
		return nil // 已经暂停
	}

	return s.executeAtomically(
		ctx,
		id,
		model.OpPause,
		task,
		// 数据库操作
		func() error {
			return s.repo.UpdateStatus(ctx, id, task.Enabled, true)
		},
		// 时间轮操作
		func() error {
			return s.timeWheel.PauseTask(id)
		},
		// 回滚数据库
		func() error {
			return s.repo.UpdateStatus(ctx, id, task.Enabled, false)
		},
	)
}

// Resume 恢复任务（原子操作）
func (s *atomicTaskService) Resume(ctx context.Context, id string) error {
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if !task.Paused {
		return nil // 未暂停
	}

	return s.executeAtomically(
		ctx,
		id,
		model.OpResume,
		task,
		// 数据库操作
		func() error {
			return s.repo.UpdateStatus(ctx, id, task.Enabled, false)
		},
		// 时间轮操作
		func() error {
			if task.Enabled {
				if err := s.timeWheel.ResumeTask(id); err != nil {
					// 如果时间轮中不存在，尝试添加
					return s.addToTimeWheel(task)
				}
			}
			return nil
		},
		// 回滚数据库
		func() error {
			return s.repo.UpdateStatus(ctx, id, task.Enabled, true)
		},
	)
}

// Update 更新任务（原子操作）
func (s *atomicTaskService) Update(ctx context.Context, id string, req *dto.TaskUpdateRequest) (*dto.TaskResponse, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 保存旧状态副本用于回滚
	oldCopy := *existing

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

	var updatedTask *model.TaskModel

	err = s.executeAtomically(
		ctx,
		id,
		model.OpUpdate,
		&oldCopy,
		// 数据库操作
		func() error {
			if err := s.repo.Update(ctx, existing); err != nil {
				return err
			}
			updatedTask = existing
			return nil
		},
		// 时间轮操作
		func() error {
			if existing.Enabled && !existing.Paused {
				// 先移除旧任务
				_ = s.timeWheel.RemoveTask(id)
				// 添加更新后的任务
				return s.addToTimeWheel(existing)
			}
			return nil
		},
		// 回滚数据库
		func() error {
			return s.repo.Update(ctx, &oldCopy)
		},
	)

	if err != nil {
		return nil, err
	}

	return dto.ToTaskResponse(updatedTask), nil
}

// GetByID 获取任务
func (s *atomicTaskService) GetByID(ctx context.Context, id string) (*dto.TaskResponse, error) {
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return dto.ToTaskResponse(task), nil
}

// List 任务列表
func (s *atomicTaskService) List(ctx context.Context, query *repository.TaskQuery) (*dto.TaskListResponse, error) {
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

// GetPendingOperations 获取待处理的操作
func (s *atomicTaskService) GetPendingOperations(ctx context.Context) ([]*model.OperationLogModel, error) {
	return s.opLogRepo.GetPendingRepairs(ctx, 100)
}

// RepairInconsistentState 修复不一致状态
func (s *atomicTaskService) RepairInconsistentState(ctx context.Context) error {
	s.logger.Info("Starting state repair...")

	// 1. 获取数据库中所有启用的任务
	query := &repository.TaskQuery{
		Enabled:  boolPtr(true),
		Page:     1,
		PageSize: 10000,
	}
	enabledTasks, _, err := s.repo.List(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to get enabled tasks: %w", err)
	}

	// 2. 获取时间轮中的所有任务
	twTasks := s.timeWheel.GetAllTasks()
	twTaskMap := make(map[string]bool)
	for _, task := range twTasks {
		twTaskMap[task.ID] = true
	}

	// 3. 修复：数据库启用但时间轮中没有的任务
	for _, dbTask := range enabledTasks {
		if !dbTask.Paused && !twTaskMap[dbTask.ID] {
			s.logger.Warn("Repairing: task in DB but not in TimeWheel",
				zap.String("task_id", dbTask.ID))
			if err := s.addToTimeWheel(dbTask); err != nil {
				s.logger.Error("Failed to repair task",
					zap.String("task_id", dbTask.ID),
					zap.Error(err))
			}
		}
	}

	// 4. 修复：时间轮中有但数据库没有/禁用的任务（应该移除）
	dbTaskMap := make(map[string]bool)
	for _, task := range enabledTasks {
		dbTaskMap[task.ID] = true
	}

	for _, twTask := range twTasks {
		if !dbTaskMap[twTask.ID] {
			s.logger.Warn("Repairing: task in TimeWheel but not in DB (or disabled)",
				zap.String("task_id", twTask.ID))
			if err := s.timeWheel.RemoveTask(twTask.ID); err != nil {
				s.logger.Error("Failed to remove orphan task",
					zap.String("task_id", twTask.ID),
					zap.Error(err))
			}
		}
	}

	s.logger.Info("State repair completed")
	return nil
}

// addToTimeWheel 添加任务到时间轮
func (s *atomicTaskService) addToTimeWheel(m *model.TaskModel) error {
	task := &timewheelCore.Task{
		ID:             m.ID,
		Description:    m.Name + " - " + m.Description,
		Mode:           timewheelCore.TaskMode(m.Mode),
		Interval:       time.Duration(m.IntervalMs) * time.Millisecond,
		Times:          m.Times,
		Priority:       timewheelCore.TaskPriority(m.Priority),
		Timeout:        time.Duration(m.TimeoutMs) * time.Millisecond,
		Severity:       timewheelCore.Severity(m.Severity),
		For:            time.Duration(m.ForDurationMs) * time.Millisecond,
		RepeatInterval: time.Duration(m.RepeatIntervalMs) * time.Millisecond,
		Labels:         m.Labels,
		Annotations:    m.Annotations,
	}

	task.Run = func(ctx context.Context) timewheelCore.AlarmResult {
		return timewheelCore.AlarmResult{}
	}

	return s.timeWheel.AddTask(task)
}

func boolPtr(b bool) *bool {
	return &b
}
