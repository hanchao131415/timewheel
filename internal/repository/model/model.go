package model

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// TaskMode 任务执行模式
type TaskMode int

const (
	TaskModeRepeated   TaskMode = 0 // 周期重复执行
	TaskModeOnce       TaskMode = 1 // 执行一次
	TaskModeFixedTimes TaskMode = 2 // 执行固定次数
)

// TaskPriority 任务优先级
type TaskPriority int

const (
	TaskPriorityHigh   TaskPriority = 0 // 高优先级
	TaskPriorityNormal TaskPriority = 1 // 普通优先级
	TaskPriorityLow    TaskPriority = 2 // 低优先级
)

// Severity 告警级别
type Severity int

const (
	SeverityCritical Severity = 0 // 严重告警
	SeverityWarning  Severity = 1 // 警告
	SeverityInfo     Severity = 2 // 信息
)

// AlertState 告警状态
type AlertState int

const (
	AlertStatePending  AlertState = 0 // 待定
	AlertStateFiring   AlertState = 1 // 触发中
	AlertStateResolved AlertState = 2 // 已解决
)

// StringMap 用于存储 map[string]string 类型的 JSON 字段
type StringMap map[string]string

// Value 实现 driver.Valuer 接口
func (m StringMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

// Scan 实现 sql.Scanner 接口
func (m *StringMap) Scan(value interface{}) error {
	if value == nil {
		*m = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("failed to scan StringMap: expected []byte")
	}
	return json.Unmarshal(bytes, m)
}

// TaskModel 任务数据库模型
type TaskModel struct {
	ID          string      `gorm:"primaryKey;type:varchar(64);comment:任务ID"`
	Name        string      `gorm:"type:varchar(255);not null;comment:任务名称"`
	Description string      `gorm:"type:varchar(500);comment:任务描述"`
	Mode        TaskMode    `gorm:"type:tinyint;default:0;comment:执行模式(0:重复,1:单次,2:固定次数)"`
	IntervalMs  int64       `gorm:"type:bigint;not null;comment:执行间隔(毫秒)"`
	Times       int         `gorm:"type:int;default:0;comment:执行次数(固定次数模式)"`
	Priority    TaskPriority `gorm:"type:tinyint;default:1;comment:优先级(0:高,1:普通,2:低)"`
	TimeoutMs   int64       `gorm:"type:bigint;default:0;comment:超时时间(毫秒)"`

	// 告警配置
	Severity       Severity `gorm:"type:tinyint;default:1;comment:告警级别(0:严重,1:警告,2:信息)"`
	ForDurationMs  int64    `gorm:"type:bigint;default:0;comment:持续时间(毫秒)"`
	RepeatIntervalMs int64  `gorm:"type:bigint;default:0;comment:重复告警间隔(毫秒)"`
	Labels         StringMap `gorm:"type:json;comment:告警标签"`
	Annotations    StringMap `gorm:"type:json;comment:告警描述"`

	// 状态
	Enabled  bool           `gorm:"type:boolean;default:true;comment:是否启用"`
	Paused   bool           `gorm:"type:boolean;default:false;comment:是否暂停"`
	EnabledAt *time.Time    `gorm:"type:datetime;comment:启用时间"`
	PausedAt  *time.Time    `gorm:"type:datetime;comment:暂停时间"`

	// 执行统计
	ExecutedCount   int        `gorm:"type:int;default:0;comment:已执行次数"`
	LastExecutedAt  *time.Time `gorm:"type:datetime;comment:最后执行时间"`
	LastResultValue float64    `gorm:"type:double;comment:最后执行结果值"`
	LastIsFiring    bool       `gorm:"type:boolean;comment:最后是否触发告警"`

	// 告警状态
	AlertState   AlertState `gorm:"type:tinyint;default:0;comment:告警状态(0:待定,1:触发,2:解决)"`
	PendingSince *time.Time `gorm:"type:datetime;comment:进入Pending状态时间"`
	LastFiredAt  *time.Time `gorm:"type:datetime;comment:最后触发告警时间"`

	// 元数据
	CreatedAt time.Time      `gorm:"type:datetime;autoCreateTime;comment:创建时间"`
	UpdatedAt time.Time      `gorm:"type:datetime;autoUpdateTime;comment:更新时间"`
	DeletedAt gorm.DeletedAt `gorm:"index;comment:删除时间"`
}

// TableName 指定表名
func (TaskModel) TableName() string {
	return "tasks"
}

// AlertHistoryModel 告警历史数据库模型
type AlertHistoryModel struct {
	ID        uint      `gorm:"primaryKey;autoIncrement;comment:主键ID"`
	TaskID    string    `gorm:"type:varchar(64);not null;index;comment:任务ID"`
	TaskName  string    `gorm:"type:varchar(255);comment:任务名称"`

	// 状态转换
	OldState AlertState `gorm:"type:tinyint;comment:旧状态"`
	NewState AlertState `gorm:"type:tinyint;comment:新状态"`

	// 评估数据
	Value     float64   `gorm:"type:double;comment:当前值"`
	Threshold float64   `gorm:"type:double;comment:阈值"`
	IsFiring  bool      `gorm:"type:boolean;comment:是否触发"`

	// 推送状态
	Notified   bool       `gorm:"type:boolean;default:false;comment:是否已通知"`
	NotifiedAt *time.Time `gorm:"type:datetime;comment:通知时间"`

	// 告警信息
	Severity    Severity   `gorm:"type:tinyint;comment:告警级别"`
	Labels      StringMap  `gorm:"type:json;comment:标签"`
	Annotations StringMap  `gorm:"type:json;comment:描述"`

	CreatedAt time.Time `gorm:"type:datetime;autoCreateTime;comment:创建时间"`
}

// TableName 指定表名
func (AlertHistoryModel) TableName() string {
	return "alert_history"
}

// WebhookQueueModel Webhook 推送队列表
type WebhookQueueModel struct {
	ID        uint   `gorm:"primaryKey;autoIncrement;comment:主键ID"`
	TaskID    string `gorm:"type:varchar(64);index;comment:关联任务ID"`
	URL       string `gorm:"type:varchar(500);not null;comment:Webhook URL"`
	Payload   string `gorm:"type:json;comment:推送载荷"`

	// 推送状态
	Status      string     `gorm:"type:varchar(20);default:'pending';comment:状态(pending/success/failed)"`
	Attempts    int        `gorm:"type:int;default:0;comment:尝试次数"`
	LastAttempt *time.Time `gorm:"type:datetime;comment:最后尝试时间"`
	NextAttempt *time.Time `gorm:"type:datetime;index;comment:下次尝试时间"`
	ErrorMsg    string     `gorm:"type:text;comment:错误信息"`

	CreatedAt time.Time `gorm:"type:datetime;autoCreateTime;comment:创建时间"`
	UpdatedAt time.Time `gorm:"type:datetime;autoUpdateTime;comment:更新时间"`
}

// TableName 指定表名
func (WebhookQueueModel) TableName() string {
	return "webhook_queue"
}

// Webhook 状态常量
const (
	WebhookStatusPending = "pending"
	WebhookStatusSuccess = "success"
	WebhookStatusFailed  = "failed"
)
