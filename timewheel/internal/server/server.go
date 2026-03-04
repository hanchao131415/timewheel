package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"timewheel/internal/config"
	"timewheel/internal/handler"
	"timewheel/internal/repository"
	"timewheel/internal/server/middleware"
	"timewheel/internal/service"
	timewheelCore "timewheel/pkg/timewheel"
)

// Server HTTP 服务器
type Server struct {
	cfg       *config.Config
	logger    *zap.Logger
	httpSrv   *http.Server
	ginEngine *gin.Engine

	// 依赖组件
	timeWheel  *timewheelCore.MultiLevelTimeWheel
	taskRepo   repository.TaskRepository
	alertRepo  repository.AlertHistoryRepository
}

// New 创建服务器
func New(
	cfg *config.Config,
	logger *zap.Logger,
	timeWheel *timewheelCore.MultiLevelTimeWheel,
	taskRepo repository.TaskRepository,
	alertRepo repository.AlertHistoryRepository,
) *Server {
	return &Server{
		cfg:       cfg,
		logger:    logger,
		timeWheel: timeWheel,
		taskRepo:  taskRepo,
		alertRepo: alertRepo,
	}
}

// Setup 设置路由
func (s *Server) Setup() error {
	// 设置 Gin 模式
	gin.SetMode(s.cfg.Server.Mode)

	// 创建 Gin 引擎
	s.ginEngine = gin.New()

	// 添加中间件
	s.ginEngine.Use(
		middleware.RequestID(),
		middleware.CORS(),
		middleware.Logger(s.logger),
		middleware.Recovery(s.logger),
		middleware.RateLimit(&s.cfg.RateLimit, s.logger),
		middleware.Auth(&s.cfg.Auth, s.logger),
	)

	// 创建服务
	taskSvc := service.NewTaskService(s.taskRepo, s.timeWheel, s.cfg, s.logger)

	// 创建处理器
	taskHandler := handler.NewTaskHandler(taskSvc, s.logger)
	alertHandler := handler.NewAlertHandler(s.alertRepo, s.timeWheel, s.logger)
	healthHandler := handler.NewHealthHandler(s.timeWheel, s.logger)
	metricsHandler := handler.NewMetricsHandler(s.timeWheel, s.logger)

	// 注册路由
	api := s.ginEngine.Group("/api/v1")
	{
		// 任务管理
		tasks := api.Group("/tasks")
		{
			tasks.POST("", taskHandler.Create)
			tasks.GET("", taskHandler.List)
			tasks.GET("/:id", taskHandler.Get)
			tasks.PUT("/:id", taskHandler.Update)
			tasks.DELETE("/:id", taskHandler.Delete)
			tasks.POST("/:id/enable", taskHandler.Enable)
			tasks.POST("/:id/disable", taskHandler.Disable)
			tasks.POST("/:id/pause", taskHandler.Pause)
			tasks.POST("/:id/resume", taskHandler.Resume)
		}

		// 告警管理
		alerts := api.Group("/alerts")
		{
			alerts.GET("", alertHandler.List)
			alerts.GET("/firing", alertHandler.GetFiring)
		}
	}

	// 健康检查
	if s.cfg.Health.Enabled {
		s.ginEngine.GET(s.cfg.Health.LivePath, healthHandler.Live)
		s.ginEngine.GET(s.cfg.Health.ReadyPath, healthHandler.Ready)
	}

	// 指标
	if s.cfg.Metrics.Enabled {
		s.ginEngine.GET(s.cfg.Metrics.Path, metricsHandler.Get)
	}

	// 创建 HTTP 服务器
	s.httpSrv = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.cfg.Server.Port),
		Handler:      s.ginEngine,
		ReadTimeout:  s.cfg.Server.ReadTimeout,
		WriteTimeout: s.cfg.Server.WriteTimeout,
	}

	return nil
}

// Start 启动服务器
func (s *Server) Start() error {
	s.logger.Info("Starting HTTP server",
		zap.Int("port", s.cfg.Server.Port),
		zap.String("mode", s.cfg.Server.Mode),
	)

	if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}
	return nil
}

// Shutdown 优雅关闭
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down HTTP server")

	ctx, cancel := context.WithTimeout(ctx, s.cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := s.httpSrv.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown HTTP server: %w", err)
	}

	s.logger.Info("HTTP server stopped")
	return nil
}

// Engine 获取 Gin 引擎（用于测试）
func (s *Server) Engine() *gin.Engine {
	return s.ginEngine
}
