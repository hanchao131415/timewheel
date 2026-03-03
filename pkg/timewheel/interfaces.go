package timewheel

import "time"

// GoroutinePool 协程池接口
type GoroutinePool interface {
	Submit(func()) error
	Release()
}

// TaskStore 任务存储接口
type TaskStore interface {
	Save(task *Task) error
	Delete(taskID string) error
	LoadAll() ([]*Task, error)
	LoadEnabled() ([]*Task, error)
	Close() error
}

// HistoryStore 历史存储接口
type HistoryStore interface {
	Record(record AlertHistory) error
	GetHistory(taskID string, limit int) ([]AlertHistory, error)
	DeleteOlderThan(days int) error
	Close() error
}

// HealthStatus 健康状态
type HealthStatus struct {
	Status     string            `json:"status"`
	Running    bool              `json:"running"`
	TaskCount  int64             `json:"task_count"`
	Uptime     string            `json:"uptime"`
	StartTime  time.Time         `json:"start_time"`
	Components map[string]string `json:"components"`
}

// HealthChecker 健康检查接口
type HealthChecker interface {
	Health() *HealthStatus
}
