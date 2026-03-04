package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"timewheel/internal/model/dto"
	"timewheel/internal/repository"
	"timewheel/internal/service"
)

// TaskHandler 任务处理器
type TaskHandler struct {
	taskSvc service.TaskService
	logger  *zap.Logger
}

// NewTaskHandler 创建任务处理器
func NewTaskHandler(taskSvc service.TaskService, logger *zap.Logger) *TaskHandler {
	return &TaskHandler{
		taskSvc: taskSvc,
		logger:  logger,
	}
}

// Create 创建任务
// @Summary 创建任务
// @Description 创建一个新的定时任务
// @Tags 任务管理
// @Accept json
// @Produce json
// @Param task body dto.TaskCreateRequest true "任务创建请求"
// @Success 200 {object} dto.CommonResponse{data=dto.TaskResponse}
// @Failure 400 {object} dto.CommonResponse
// @Failure 500 {object} dto.CommonResponse
// @Router /api/v1/tasks [post]
func (h *TaskHandler) Create(c *gin.Context) {
	var req dto.TaskCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse(400, "Invalid request: "+err.Error()))
		return
	}

	resp, err := h.taskSvc.Create(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("Failed to create task",
			zap.String("task_id", req.ID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.SuccessResponse(resp))
}

// Get 获取任务详情
// @Summary 获取任务详情
// @Description 根据 ID 获取任务详情
// @Tags 任务管理
// @Produce json
// @Param id path string true "任务 ID"
// @Success 200 {object} dto.CommonResponse{data=dto.TaskResponse}
// @Failure 404 {object} dto.CommonResponse
// @Router /api/v1/tasks/{id} [get]
func (h *TaskHandler) Get(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse(400, "Task ID is required"))
		return
	}

	resp, err := h.taskSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse(404, "Task not found"))
		return
	}

	c.JSON(http.StatusOK, dto.SuccessResponse(resp))
}

// List 获取任务列表
// @Summary 获取任务列表
// @Description 分页获取任务列表
// @Tags 任务管理
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Param name query string false "任务名称（模糊搜索）"
// @Param enabled query bool false "是否启用"
// @Param paused query bool false "是否暂停"
// @Success 200 {object} dto.CommonResponse{data=dto.TaskListResponse}
// @Router /api/v1/tasks [get]
func (h *TaskHandler) List(c *gin.Context) {
	query := &repository.TaskQuery{
		Page:     parseInt(c.Query("page"), 1),
		PageSize: parseInt(c.Query("page_size"), 20),
		Name:     c.Query("name"),
	}

	if enabled := c.Query("enabled"); enabled != "" {
		val := enabled == "true"
		query.Enabled = &val
	}
	if paused := c.Query("paused"); paused != "" {
		val := paused == "true"
		query.Paused = &val
	}
	if priority := c.Query("priority"); priority != "" {
		val := parseInt(priority, -1)
		if val >= 0 {
			query.Priority = &val
		}
	}
	if mode := c.Query("mode"); mode != "" {
		val := parseInt(mode, -1)
		if val >= 0 {
			query.Mode = &val
		}
	}

	resp, err := h.taskSvc.List(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.SuccessResponse(resp))
}

// Update 更新任务
// @Summary 更新任务
// @Description 更新任务信息
// @Tags 任务管理
// @Accept json
// @Produce json
// @Param id path string true "任务 ID"
// @Param task body dto.TaskUpdateRequest true "任务更新请求"
// @Success 200 {object} dto.CommonResponse{data=dto.TaskResponse}
// @Failure 400 {object} dto.CommonResponse
// @Failure 404 {object} dto.CommonResponse
// @Router /api/v1/tasks/{id} [put]
func (h *TaskHandler) Update(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse(400, "Task ID is required"))
		return
	}

	var req dto.TaskUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse(400, "Invalid request: "+err.Error()))
		return
	}

	resp, err := h.taskSvc.Update(c.Request.Context(), id, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.SuccessResponse(resp))
}

// Delete 删除任务
// @Summary 删除任务
// @Description 删除指定任务
// @Tags 任务管理
// @Param id path string true "任务 ID"
// @Success 200 {object} dto.CommonResponse
// @Failure 404 {object} dto.CommonResponse
// @Router /api/v1/tasks/{id} [delete]
func (h *TaskHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse(400, "Task ID is required"))
		return
	}

	if err := h.taskSvc.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse(404, "Task not found"))
		return
	}

	c.JSON(http.StatusOK, dto.SuccessResponse(nil))
}

// Enable 启用任务
// @Summary 启用任务
// @Description 启用指定任务
// @Tags 任务管理
// @Param id path string true "任务 ID"
// @Success 200 {object} dto.CommonResponse
// @Router /api/v1/tasks/{id}/enable [post]
func (h *TaskHandler) Enable(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse(400, "Task ID is required"))
		return
	}

	if err := h.taskSvc.Enable(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.SuccessResponse(nil))
}

// Disable 禁用任务
// @Summary 禁用任务
// @Description 禁用指定任务
// @Tags 任务管理
// @Param id path string true "任务 ID"
// @Success 200 {object} dto.CommonResponse
// @Router /api/v1/tasks/{id}/disable [post]
func (h *TaskHandler) Disable(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse(400, "Task ID is required"))
		return
	}

	if err := h.taskSvc.Disable(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.SuccessResponse(nil))
}

// Pause 暂停任务
// @Summary 暂停任务
// @Description 暂停指定任务
// @Tags 任务管理
// @Param id path string true "任务 ID"
// @Success 200 {object} dto.CommonResponse
// @Router /api/v1/tasks/{id}/pause [post]
func (h *TaskHandler) Pause(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse(400, "Task ID is required"))
		return
	}

	if err := h.taskSvc.Pause(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.SuccessResponse(nil))
}

// Resume 恢复任务
// @Summary 恢复任务
// @Description 恢复指定任务
// @Tags 任务管理
// @Param id path string true "任务 ID"
// @Success 200 {object} dto.CommonResponse
// @Router /api/v1/tasks/{id}/resume [post]
func (h *TaskHandler) Resume(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse(400, "Task ID is required"))
		return
	}

	if err := h.taskSvc.Resume(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, dto.SuccessResponse(nil))
}

// parseInt 解析整数，失败返回默认值
func parseInt(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return val
}
