package fixtures

import (
	"context"
	"time"

	"timewheel/internal/repository/model"
	timewheelCore "timewheel/pkg/timewheel"
)

// TaskFixture 任务测试固件生成器
type TaskFixture struct {
	ID          string
	Name        string
	Description string
	Mode        model.TaskMode
	IntervalMs  int64
	Priority    model.TaskPriority
	TimeoutMs   int64
	Severity    model.Severity
	ForMs       int64
	RepeatMs    int64
	Times       int
	Enabled     bool
	Paused      bool
	Labels      map[string]string
	Annotations map[string]string
}

// NewTaskFixture 创建默认任务固件
func NewTaskFixture() *TaskFixture {
	return &TaskFixture{
		ID:          "test-task-001",
		Name:        "Test Task",
		Description: "A test task for integration testing",
		Mode:        model.TaskModeRepeated,
		IntervalMs:  1000, // 1 second
		Priority:    model.TaskPriorityNormal,
		TimeoutMs:   5000,
		Severity:    model.SeverityWarning,
		ForMs:       0,
		RepeatMs:    0,
		Times:       0,
		Enabled:     true,
		Paused:      false,
		Labels: map[string]string{
			"env":     "test",
			"service": "timewheel",
		},
		Annotations: map[string]string{
			"description": "Integration test task",
		},
	}
}

// WithID 设置任务ID
func (f *TaskFixture) WithID(id string) *TaskFixture {
	f.ID = id
	return f
}

// WithName 设置任务名称
func (f *TaskFixture) WithName(name string) *TaskFixture {
	f.Name = name
	return f
}

// WithInterval 设置执行间隔
func (f *TaskFixture) WithInterval(ms int64) *TaskFixture {
	f.IntervalMs = ms
	return f
}

// WithMode 设置执行模式
func (f *TaskFixture) WithMode(mode model.TaskMode) *TaskFixture {
	f.Mode = mode
	return f
}

// WithPriority 设置优先级
func (f *TaskFixture) WithPriority(priority model.TaskPriority) *TaskFixture {
	f.Priority = priority
	return f
}

// WithEnabled 设置启用状态
func (f *TaskFixture) WithEnabled(enabled bool) *TaskFixture {
	f.Enabled = enabled
	return f
}

// WithPaused 设置暂停状态
func (f *TaskFixture) WithPaused(paused bool) *TaskFixture {
	f.Paused = paused
	return f
}

// WithTimes 设置执行次数
func (f *TaskFixture) WithTimes(times int) *TaskFixture {
	f.Times = times
	return f
}

// WithSeverity 设置告警级别
func (f *TaskFixture) WithSeverity(severity model.Severity) *TaskFixture {
	f.Severity = severity
	return f
}

// WithForDuration 设置持续时间
func (f *TaskFixture) WithForDuration(ms int64) *TaskFixture {
	f.ForMs = ms
	return f
}

// WithRepeatInterval 设置重复间隔
func (f *TaskFixture) WithRepeatInterval(ms int64) *TaskFixture {
	f.RepeatMs = ms
	return f
}

// WithLabels 设置标签
func (f *TaskFixture) WithLabels(labels map[string]string) *TaskFixture {
	f.Labels = labels
	return f
}

// WithAnnotations 设置注解
func (f *TaskFixture) WithAnnotations(annotations map[string]string) *TaskFixture {
	f.Annotations = annotations
	return f
}

// ToModel 转换为数据库模型
func (f *TaskFixture) ToModel() *model.TaskModel {
	return &model.TaskModel{
		ID:              f.ID,
		Name:            f.Name,
		Description:     f.Description,
		Mode:            f.Mode,
		IntervalMs:      f.IntervalMs,
		Times:           f.Times,
		Priority:        f.Priority,
		TimeoutMs:       f.TimeoutMs,
		Severity:        f.Severity,
		ForDurationMs:   f.ForMs,
		RepeatIntervalMs: f.RepeatMs,
		Enabled:         f.Enabled,
		Paused:          f.Paused,
		Labels:          model.StringMap(f.Labels),
		Annotations:     model.StringMap(f.Annotations),
	}
}

// ToTimeWheelTask 转换为时间轮任务
func (f *TaskFixture) ToTimeWheelTask(runFunc func(ctx context.Context) timewheelCore.AlarmResult) *timewheelCore.Task {
	task := &timewheelCore.Task{
		ID:             f.ID,
		Description:    f.Name + " - " + f.Description,
		Mode:           timewheelCore.TaskMode(f.Mode),
		Interval:       time.Duration(f.IntervalMs) * time.Millisecond,
		Times:          f.Times,
		Priority:       timewheelCore.TaskPriority(f.Priority),
		Timeout:        time.Duration(f.TimeoutMs) * time.Millisecond,
		Severity:       timewheelCore.Severity(f.Severity),
		For:            time.Duration(f.ForMs) * time.Millisecond,
		RepeatInterval: time.Duration(f.RepeatMs) * time.Millisecond,
		Labels:         f.Labels,
		Annotations:    f.Annotations,
	}
	if runFunc != nil {
		task.Run = runFunc
	} else {
		task.Run = func(ctx context.Context) timewheelCore.AlarmResult {
			return timewheelCore.AlarmResult{
				Value:     42.0,
				Threshold: 50.0,
				IsFiring:  false,
			}
		}
	}
	return task
}

// BatchTaskFixtures 批量任务固件生成器
type BatchTaskFixtures struct {
	count   int
	baseID  string
	base    *TaskFixture
	variant func(i int, f *TaskFixture)
}

// NewBatchTaskFixtures 创建批量任务固件
func NewBatchTaskFixtures(count int) *BatchTaskFixtures {
	return &BatchTaskFixtures{
		count:  count,
		baseID: "batch-task",
		base:   NewTaskFixture(),
		variant: func(i int, f *TaskFixture) {
			f.ID = f.ID // Keep as is
		},
	}
}

// WithBaseID 设置基础ID
func (b *BatchTaskFixtures) WithBaseID(id string) *BatchTaskFixtures {
	b.baseID = id
	return b
}

// WithVariant 设置变体函数
func (b *BatchTaskFixtures) WithVariant(fn func(i int, f *TaskFixture)) *BatchTaskFixtures {
	b.variant = fn
	return b
}

// Generate 生成批量任务模型
func (b *BatchTaskFixtures) Generate() []*model.TaskModel {
	tasks := make([]*model.TaskModel, b.count)
	for i := 0; i < b.count; i++ {
		f := NewTaskFixture()
		f.ID = b.baseID
		b.variant(i, f)
		tasks[i] = f.ToModel()
	}
	return tasks
}

// GenerateIDs 生成批量任务ID
func GenerateIDs(prefix string, count int) []string {
	ids := make([]string, count)
	for i := 0; i < count; i++ {
		ids[i] = prefix + "-" + time.Now().Format("20060102150405") + "-" + string(rune('A'+i%26)) + string(rune('0'+i/26))
	}
	return ids
}

// PriorityTasks 生成不同优先级的任务
func PriorityTasks() []*TaskFixture {
	return []*TaskFixture{
		NewTaskFixture().WithID("high-priority-task").WithPriority(model.TaskPriorityHigh),
		NewTaskFixture().WithID("normal-priority-task").WithPriority(model.TaskPriorityNormal),
		NewTaskFixture().WithID("low-priority-task").WithPriority(model.TaskPriorityLow),
	}
}

// ModeTasks 生成不同模式的任务
func ModeTasks() []*TaskFixture {
	return []*TaskFixture{
		NewTaskFixture().WithID("once-task").WithMode(model.TaskModeOnce),
		NewTaskFixture().WithID("repeated-task").WithMode(model.TaskModeRepeated),
		NewTaskFixture().WithID("fixed-times-task").WithMode(model.TaskModeFixedTimes).WithTimes(3),
	}
}

// SeverityTasks 生成不同告警级别的任务
func SeverityTasks() []*TaskFixture {
	return []*TaskFixture{
		NewTaskFixture().WithID("critical-task").WithSeverity(model.SeverityCritical),
		NewTaskFixture().WithID("warning-task").WithSeverity(model.SeverityWarning),
		NewTaskFixture().WithID("info-task").WithSeverity(model.SeverityInfo),
	}
}
