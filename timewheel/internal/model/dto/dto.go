package dto

import (
	"time"

	"timewheel/internal/repository/model"
)

// TaskCreateRequest 创建任务请求
type TaskCreateRequest struct {
	ID               string            `json:"id" binding:"omitempty,max=64"` // 可选，不传则自动生成雪花ID
	Name             string            `json:"name" binding:"required,max=255"`
	Description      string            `json:"description" binding:"max=500"`
	Mode             int               `json:"mode" binding:"gte=0,lte=2"` // 0:重复,1:单次,2:固定次数
	IntervalMs       int64             `json:"interval_ms" binding:"required,gt=0"`
	Times            int               `json:"times" binding:"gte=0"`
	Priority         int               `json:"priority" binding:"gte=0,lte=2"` // 0:高,1:普通,2:低
	TimeoutMs        int64             `json:"timeout_ms" binding:"gte=0"`
	Severity         int               `json:"severity" binding:"gte=0,lte=2"` // 0:严重,1:警告,2:信息
	ForDurationMs    int64             `json:"for_duration_ms" binding:"gte=0"`
	RepeatIntervalMs int64             `json:"repeat_interval_ms" binding:"gte=0"`
	Labels           map[string]string `json:"labels"`
	Annotations      map[string]string `json:"annotations"`
	Enabled          bool              `json:"enabled"`
}

// TaskUpdateRequest 更新任务请求
type TaskUpdateRequest struct {
	Name             *string            `json:"name" binding:"omitempty,max=255"`
	Description      *string            `json:"description" binding:"omitempty,max=500"`
	Mode             *int               `json:"mode" binding:"omitempty,gte=0,lte=2"`
	IntervalMs       *int64             `json:"interval_ms" binding:"omitempty,gt=0"`
	Times            *int               `json:"times" binding:"omitempty,gte=0"`
	Priority         *int               `json:"priority" binding:"omitempty,gte=0,lte=2"`
	TimeoutMs        *int64             `json:"timeout_ms" binding:"omitempty,gte=0"`
	Severity         *int               `json:"severity" binding:"omitempty,gte=0,lte=2"`
	ForDurationMs    *int64             `json:"for_duration_ms" binding:"omitempty,gte=0"`
	RepeatIntervalMs *int64             `json:"repeat_interval_ms" binding:"omitempty,gte=0"`
	Labels           *map[string]string `json:"labels"`
	Annotations      *map[string]string `json:"annotations"`
}

// TaskResponse 任务响应
type TaskResponse struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Mode             int               `json:"mode"`
	ModeText         string            `json:"mode_text"`
	IntervalMs       int64             `json:"interval_ms"`
	Times            int               `json:"times"`
	Priority         int               `json:"priority"`
	PriorityText     string            `json:"priority_text"`
	TimeoutMs        int64             `json:"timeout_ms"`
	Severity         int               `json:"severity"`
	SeverityText     string            `json:"severity_text"`
	ForDurationMs    int64             `json:"for_duration_ms"`
	RepeatIntervalMs int64             `json:"repeat_interval_ms"`
	Labels           map[string]string `json:"labels"`
	Annotations      map[string]string `json:"annotations"`
	Enabled          bool              `json:"enabled"`
	Paused           bool              `json:"paused"`
	EnabledAt        *time.Time        `json:"enabled_at,omitempty"`
	PausedAt         *time.Time        `json:"paused_at,omitempty"`
	ExecutedCount    int               `json:"executed_count"`
	LastExecutedAt   *time.Time        `json:"last_executed_at,omitempty"`
	LastResultValue  float64           `json:"last_result_value"`
	LastIsFiring     bool              `json:"last_is_firing"`
	AlertState       int               `json:"alert_state"`
	AlertStateText   string            `json:"alert_state_text"`
	PendingSince     *time.Time        `json:"pending_since,omitempty"`
	LastFiredAt      *time.Time        `json:"last_fired_at,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// TaskListResponse 任务列表响应
type TaskListResponse struct {
	List  []*TaskResponse `json:"list"`
	Total int64           `json:"total"`
	Page  int             `json:"page"`
	Size  int             `json:"size"`
}

// ToTaskModel 转换为数据库模型
func (r *TaskCreateRequest) ToTaskModel() *model.TaskModel {
	task := &model.TaskModel{
		ID:               r.ID,
		Name:             r.Name,
		Description:      r.Description,
		Mode:             model.TaskMode(r.Mode),
		IntervalMs:       r.IntervalMs,
		Times:            r.Times,
		Priority:         model.TaskPriority(r.Priority),
		TimeoutMs:        r.TimeoutMs,
		Severity:         model.Severity(r.Severity),
		ForDurationMs:    r.ForDurationMs,
		RepeatIntervalMs: r.RepeatIntervalMs,
		Labels:           r.Labels,
		Annotations:      r.Annotations,
		Enabled:          r.Enabled,
		AlertState:       model.AlertStatePending,
	}
	if task.Labels == nil {
		task.Labels = make(model.StringMap)
	}
	if task.Annotations == nil {
		task.Annotations = make(model.StringMap)
	}
	return task
}

// ToTaskResponse 转换为响应
func ToTaskResponse(m *model.TaskModel) *TaskResponse {
	return &TaskResponse{
		ID:               m.ID,
		Name:             m.Name,
		Description:      m.Description,
		Mode:             int(m.Mode),
		ModeText:         getModeText(m.Mode),
		IntervalMs:       m.IntervalMs,
		Times:            m.Times,
		Priority:         int(m.Priority),
		PriorityText:     getPriorityText(m.Priority),
		TimeoutMs:        m.TimeoutMs,
		Severity:         int(m.Severity),
		SeverityText:     getSeverityText(m.Severity),
		ForDurationMs:    m.ForDurationMs,
		RepeatIntervalMs: m.RepeatIntervalMs,
		Labels:           m.Labels,
		Annotations:      m.Annotations,
		Enabled:          m.Enabled,
		Paused:           m.Paused,
		EnabledAt:        m.EnabledAt,
		PausedAt:         m.PausedAt,
		ExecutedCount:    m.ExecutedCount,
		LastExecutedAt:   m.LastExecutedAt,
		LastResultValue:  m.LastResultValue,
		LastIsFiring:     m.LastIsFiring,
		AlertState:       int(m.AlertState),
		AlertStateText:   getAlertStateText(m.AlertState),
		PendingSince:     m.PendingSince,
		LastFiredAt:      m.LastFiredAt,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}
}

func getModeText(mode model.TaskMode) string {
	switch mode {
	case model.TaskModeRepeated:
		return "repeated"
	case model.TaskModeOnce:
		return "once"
	case model.TaskModeFixedTimes:
		return "fixed_times"
	default:
		return "unknown"
	}
}

func getPriorityText(priority model.TaskPriority) string {
	switch priority {
	case model.TaskPriorityHigh:
		return "high"
	case model.TaskPriorityNormal:
		return "normal"
	case model.TaskPriorityLow:
		return "low"
	default:
		return "unknown"
	}
}

func getSeverityText(severity model.Severity) string {
	switch severity {
	case model.SeverityCritical:
		return "critical"
	case model.SeverityWarning:
		return "warning"
	case model.SeverityInfo:
		return "info"
	default:
		return "unknown"
	}
}

func getAlertStateText(state model.AlertState) string {
	switch state {
	case model.AlertStatePending:
		return "pending"
	case model.AlertStateFiring:
		return "firing"
	case model.AlertStateResolved:
		return "resolved"
	default:
		return "unknown"
	}
}

// AlertHistoryResponse 告警历史响应
type AlertHistoryResponse struct {
	ID          uint              `json:"id"`
	TaskID      string            `json:"task_id"`
	TaskName    string            `json:"task_name"`
	OldState    int               `json:"old_state"`
	OldStateText string           `json:"old_state_text"`
	NewState    int               `json:"new_state"`
	NewStateText string           `json:"new_state_text"`
	Value       float64           `json:"value"`
	Threshold   float64           `json:"threshold"`
	IsFiring    bool              `json:"is_firing"`
	Notified    bool              `json:"notified"`
	NotifiedAt  *time.Time        `json:"notified_at,omitempty"`
	Severity    int               `json:"severity"`
	SeverityText string           `json:"severity_text"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	CreatedAt   time.Time         `json:"created_at"`
}

// AlertHistoryListResponse 告警历史列表响应
type AlertHistoryListResponse struct {
	List  []*AlertHistoryResponse `json:"list"`
	Total int64                   `json:"total"`
	Page  int                     `json:"page"`
	Size  int                     `json:"size"`
}

// ToAlertHistoryResponse 转换为响应
func ToAlertHistoryResponse(m *model.AlertHistoryModel) *AlertHistoryResponse {
	return &AlertHistoryResponse{
		ID:           m.ID,
		TaskID:       m.TaskID,
		TaskName:     m.TaskName,
		OldState:     int(m.OldState),
		OldStateText: getAlertStateText(m.OldState),
		NewState:     int(m.NewState),
		NewStateText: getAlertStateText(m.NewState),
		Value:        m.Value,
		Threshold:    m.Threshold,
		IsFiring:     m.IsFiring,
		Notified:     m.Notified,
		NotifiedAt:   m.NotifiedAt,
		Severity:     int(m.Severity),
		SeverityText: getSeverityText(m.Severity),
		Labels:       m.Labels,
		Annotations:  m.Annotations,
		CreatedAt:    m.CreatedAt,
	}
}

// CommonResponse 通用响应
type CommonResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// SuccessResponse 成功响应
func SuccessResponse(data interface{}) *CommonResponse {
	return &CommonResponse{
		Code:    0,
		Message: "success",
		Data:    data,
	}
}

// ErrorResponse 错误响应
func ErrorResponse(code int, message string) *CommonResponse {
	return &CommonResponse{
		Code:    code,
		Message: message,
	}
}

// HealthResponse 健康检查响应
type HealthResponse struct {
	Status    string            `json:"status"` // ok / error
	Timestamp time.Time         `json:"timestamp"`
	Components map[string]string `json:"components,omitempty"`
}

// ReadyResponse 就绪检查响应
type ReadyResponse struct {
	Ready     bool              `json:"ready"`
	Timestamp time.Time         `json:"timestamp"`
	Checks    map[string]string `json:"checks"`
}

// MetricsResponse 指标响应
type MetricsResponse struct {
	TotalTasks        int64   `json:"total_tasks"`
	RunningTasks      int32   `json:"running_tasks"`
	ExecutedTasks     int64   `json:"executed_tasks"`
	FailedTasks       int64   `json:"failed_tasks"`
	PendingAlerts     int     `json:"pending_alerts"`
	FiringAlerts      int     `json:"firing_alerts"`
	ResolvedAlerts    int     `json:"resolved_alerts"`
	AvgExecutionTime  float64 `json:"avg_execution_time_us"`
	CacheHitRate      float64 `json:"cache_hit_rate"`
}
