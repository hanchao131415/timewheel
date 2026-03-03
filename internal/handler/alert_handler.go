package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"timewheel/internal/model/dto"
	"timewheel/internal/repository"
	timewheelCore "timewheel/pkg/timewheel"
)

// AlertHandler 告警处理器
type AlertHandler struct {
	alertRepo repository.AlertHistoryRepository
	timeWheel *timewheelCore.MultiLevelTimeWheel
	logger    *zap.Logger
}

// NewAlertHandler 创建告警处理器
func NewAlertHandler(
	alertRepo repository.AlertHistoryRepository,
	timeWheel *timewheelCore.MultiLevelTimeWheel,
	logger *zap.Logger,
) *AlertHandler {
	return &AlertHandler{
		alertRepo: alertRepo,
		timeWheel: timeWheel,
		logger:    logger,
	}
}

// List 获取告警历史列表
// @Summary 获取告警历史列表
// @Description 分页获取告警历史
// @Tags 告警管理
// @Produce json
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Param task_id query string false "任务 ID"
// @Success 200 {object} dto.CommonResponse{data=dto.AlertHistoryListResponse}
// @Router /api/v1/alerts [get]
func (h *AlertHandler) List(c *gin.Context) {
	query := &repository.AlertHistoryQuery{
		Page:     parseInt(c.Query("page"), 1),
		PageSize: parseInt(c.Query("page_size"), 20),
		TaskID:   c.Query("task_id"),
		TaskName: c.Query("task_name"),
	}

	if isFiring := c.Query("is_firing"); isFiring != "" {
		val := isFiring == "true"
		query.IsFiring = &val
	}

	histories, total, err := h.alertRepo.List(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse(500, err.Error()))
		return
	}

	list := make([]*dto.AlertHistoryResponse, 0, len(histories))
	for _, h := range histories {
		list = append(list, dto.ToAlertHistoryResponse(h))
	}

	c.JSON(http.StatusOK, dto.SuccessResponse(&dto.AlertHistoryListResponse{
		List:  list,
		Total: total,
		Page:  query.Page,
		Size:  query.PageSize,
	}))
}

// GetFiring 获取触发中的告警
// @Summary 获取触发中的告警
// @Description 获取所有触发中的告警
// @Tags 告警管理
// @Produce json
// @Success 200 {object} dto.CommonResponse{data=[]dto.AlertHistoryResponse}
// @Router /api/v1/alerts/firing [get]
func (h *AlertHandler) GetFiring(c *gin.Context) {
	histories, err := h.alertRepo.GetFiring(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse(500, err.Error()))
		return
	}

	list := make([]*dto.AlertHistoryResponse, 0, len(histories))
	for _, h := range histories {
		list = append(list, dto.ToAlertHistoryResponse(h))
	}

	c.JSON(http.StatusOK, dto.SuccessResponse(list))
}

// HealthHandler 健康检查处理器
type HealthHandler struct {
	timeWheel *timewheelCore.MultiLevelTimeWheel
	logger    *zap.Logger
}

// NewHealthHandler 创建健康检查处理器
func NewHealthHandler(
	timeWheel *timewheelCore.MultiLevelTimeWheel,
	logger *zap.Logger,
) *HealthHandler {
	return &HealthHandler{
		timeWheel: timeWheel,
		logger:    logger,
	}
}

// Live 存活检查
// @Summary 存活检查
// @Description 检查服务是否存活
// @Tags 健康检查
// @Produce json
// @Success 200 {object} dto.HealthResponse
// @Router /health/live [get]
func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, &dto.HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
	})
}

// Ready 就绪检查
// @Summary 就绪检查
// @Description 检查服务是否就绪
// @Tags 健康检查
// @Produce json
// @Success 200 {object} dto.ReadyResponse
// @Router /health/ready [get]
func (h *HealthHandler) Ready(c *gin.Context) {
	checks := make(map[string]string)
	ready := true

	// 检查时间轮状态
	if h.timeWheel == nil {
		checks["timewheel"] = "not initialized"
		ready = false
	} else {
		checks["timewheel"] = "ok"
	}

	if !ready {
		c.JSON(http.StatusServiceUnavailable, &dto.ReadyResponse{
			Ready:     ready,
			Timestamp: time.Now(),
			Checks:    checks,
		})
		return
	}

	c.JSON(http.StatusOK, &dto.ReadyResponse{
		Ready:     ready,
		Timestamp: time.Now(),
		Checks:    checks,
	})
}

// MetricsHandler 指标处理器
type MetricsHandler struct {
	timeWheel *timewheelCore.MultiLevelTimeWheel
	logger    *zap.Logger
}

// NewMetricsHandler 创建指标处理器
func NewMetricsHandler(
	timeWheel *timewheelCore.MultiLevelTimeWheel,
	logger *zap.Logger,
) *MetricsHandler {
	return &MetricsHandler{
		timeWheel: timeWheel,
		logger:    logger,
	}
}

// Get 获取指标
// @Summary 获取系统指标
// @Description 获取时间轮和任务相关指标
// @Tags 监控
// @Produce json
// @Success 200 {object} dto.CommonResponse{data=dto.MetricsResponse}
// @Router /metrics [get]
func (h *MetricsHandler) Get(c *gin.Context) {
	if h.timeWheel == nil {
		c.JSON(http.StatusOK, dto.SuccessResponse(&dto.MetricsResponse{}))
		return
	}

	// 获取时间轮指标
	// 注意: MultiLevelTimeWheel 需要提供 GetMetrics 方法
	// 这里简化处理，返回基本信息
	tasks := h.timeWheel.GetAllTasks()

	var pendingAlerts, firingAlerts, resolvedAlerts int
	for range tasks {
		// 这里需要从 task 获取告警状态
		// 简化处理，暂不统计
	}

	var avgExecTime float64
	var cacheHitRate float64

	c.JSON(http.StatusOK, dto.SuccessResponse(&dto.MetricsResponse{
		TotalTasks:       int64(len(tasks)),
		PendingAlerts:    pendingAlerts,
		FiringAlerts:     firingAlerts,
		ResolvedAlerts:   resolvedAlerts,
		AvgExecutionTime: avgExecTime,
		CacheHitRate:     cacheHitRate,
	}))
}
