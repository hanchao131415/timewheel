package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"timewheel/internal/config"
	"timewheel/internal/repository"
	db "timewheel/internal/repository/db"
	"timewheel/internal/server"
	timewheelLogger "timewheel/pkg/logger"
	timewheelCore "timewheel/pkg/timewheel"
)

var (
	configPath = flag.String("config", "", "Path to config file")
	version    = "1.0.0"
)

func main() {
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	if err := timewheelLogger.Init(&cfg.Logging); err != nil {
		fmt.Printf("Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer timewheelLogger.Sync()

	logger := timewheelLogger.L()
	logger.Info("Starting TimeWheel Scheduler",
		zap.String("version", version),
		zap.String("mode", cfg.Mode),
	)

	// 初始化数据库
	if err := db.Init(&cfg.Database); err != nil {
		logger.Fatal("Failed to init database", zap.Error(err))
	}
	defer db.Close()

	// 初始化时间轮
	timeWheel, err := initTimeWheel(cfg)
	if err != nil {
		logger.Fatal("Failed to init time wheel", zap.Error(err))
	}
	defer timeWheel.Stop()

	// 启动时间轮
	if err := timeWheel.Start(); err != nil {
		logger.Fatal("Failed to start time wheel", zap.Error(err))
	}

	// 恢复任务
	if err := restoreTasks(timeWheel, logger); err != nil {
		logger.Warn("Failed to restore tasks", zap.Error(err))
	}

	// 初始化仓储
	taskRepo := repository.NewTaskRepository(db.GetDB())
	alertRepo := repository.NewAlertHistoryRepository(db.GetDB())

	// 创建并启动 HTTP 服务器
	srv := server.New(cfg, logger, timeWheel, taskRepo, alertRepo)
	if err := srv.Setup(); err != nil {
		logger.Fatal("Failed to setup server", zap.Error(err))
	}

	// 启动服务器（在 goroutine 中）
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	logger.Info("Server started successfully",
		zap.Int("port", cfg.Server.Port),
	)

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}

	logger.Info("Server stopped")
}

// initTimeWheel 初始化时间轮
func initTimeWheel(cfg *config.Config) (*timewheelCore.MultiLevelTimeWheel, error) {
	// 创建多层时间轮
	mtw, err := timewheelCore.NewMultiLevelTimeWheel()
	if err != nil {
		return nil, err
	}

	return mtw, nil
}

// restoreTasks 恢复任务
func restoreTasks(timeWheel *timewheelCore.MultiLevelTimeWheel, logger *zap.Logger) error {
	// 从数据库加载启用的任务
	taskRepo := repository.NewTaskRepository(db.GetDB())
	tasks, err := taskRepo.GetEnabled(context.Background())
	if err != nil {
		return err
	}

	restored := 0
	for _, task := range tasks {
		if task.Paused {
			continue
		}

		// 转换为时间轮任务
		twTask := &timewheelCore.Task{
			ID:          task.ID,
			Description: task.Name + " - " + task.Description, // 将名称合并到描述中
			Mode:        timewheelCore.TaskMode(task.Mode),
			Interval:    time.Duration(task.IntervalMs) * time.Millisecond,
			Times:       task.Times,
			Priority:    timewheelCore.TaskPriority(task.Priority),
			Timeout:     time.Duration(task.TimeoutMs) * time.Millisecond,
			Severity:    timewheelCore.Severity(task.Severity),
			For:         time.Duration(task.ForDurationMs) * time.Millisecond,
			RepeatInterval: time.Duration(task.RepeatIntervalMs) * time.Millisecond,
			Labels:      task.Labels,
			Annotations: task.Annotations,
			Run: func(ctx context.Context) timewheelCore.AlarmResult {
				// TODO: 从任务执行器注册表获取实际执行函数
				return timewheelCore.AlarmResult{}
			},
		}

		if err := timeWheel.AddTask(twTask); err != nil {
			logger.Warn("Failed to restore task",
				zap.String("task_id", task.ID),
				zap.Error(err),
			)
			continue
		}

		restored++
	}

	logger.Info("Tasks restored", zap.Int("count", restored))
	return nil
}
