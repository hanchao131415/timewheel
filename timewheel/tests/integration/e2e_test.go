package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"timewheel/internal/config"
	"timewheel/internal/handler"
	"timewheel/internal/model/dto"
	"timewheel/internal/repository"
	"timewheel/internal/repository/model"
	"timewheel/internal/service"
	timewheelCore "timewheel/pkg/timewheel"
	"timewheel/tests/fixtures"
	"timewheel/tests/testutil"
)

// E2ETestFixture 端到端测试固件
type E2ETestFixture struct {
	DB          *gorm.DB
	DBFixture   *fixtures.DBFixture
	TaskRepo    repository.TaskRepository
	OpLogRepo   repository.OperationLogRepository
	AlertRepo   repository.AlertHistoryRepository
	TimeWheel   *timewheelCore.MultiLevelTimeWheel
	TaskService service.TaskService
	Logger      *zap.Logger
	Router      *gin.Engine
	HTTPServer  *httptest.Server
	Cleanup     func()
}

// NewE2ETestFixture 创建端到端测试固件
func NewE2ETestFixture(t *testing.T) *E2ETestFixture {
	gin.SetMode(gin.TestMode)

	// 创建数据库
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database fixture: %v", err)
	}

	// 创建日志
	logger := zap.NewNop()

	// 创建时间轮
	tw, err := timewheelCore.NewMultiLevelTimeWheel()
	if err != nil {
		dbFixture.Close()
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}

	// 创建仓储
	taskRepo := repository.NewTaskRepository(dbFixture.DB)
	opLogRepo := repository.NewOperationLogRepository(dbFixture.DB)
	alertRepo := repository.NewAlertHistoryRepository(dbFixture.DB)

	// 创建配置
	cfg := &config.Config{}

	// 创建服务
	atomicSvc := service.NewAtomicTaskService(taskRepo, opLogRepo, tw, cfg, logger)

	// 创建 Handler
	taskHandler := handler.NewTaskHandler(atomicSvc, logger)
	alertHandler := handler.NewAlertHandler(alertRepo, tw, logger)

	// 创建路由
	router := gin.New()
	api := router.Group("/api/v1")
	{
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

		alerts := api.Group("/alerts")
		{
			alerts.GET("", alertHandler.List)
			alerts.GET("/firing", alertHandler.GetFiring)
		}
	}

	// 启动时间轮
	if err := tw.Start(); err != nil {
		t.Fatalf("Failed to start TimeWheel: %v", err)
	}

	// 创建 HTTP 测试服务器
	httpServer := httptest.NewServer(router)

	return &E2ETestFixture{
		DB:          dbFixture.DB,
		DBFixture:   dbFixture,
		TaskRepo:    taskRepo,
		OpLogRepo:   opLogRepo,
		AlertRepo:   alertRepo,
		TimeWheel:   tw,
		TaskService: atomicSvc,
		Logger:      logger,
		Router:      router,
		HTTPServer:  httpServer,
		Cleanup: func() {
			httpServer.Close()
			tw.Stop()
			dbFixture.Close()
		},
	}
}

// TestE2E_CreateTaskViaAPI_FullFlow 测试 API -> DB -> TimeWheel 完整流程
func TestE2E_CreateTaskViaAPI_FullFlow(t *testing.T) {
	f := NewE2ETestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)

	// 1. 通过 API 创建任务
	createReq := `{
		"name": "E2E Test Task",
		"description": "End-to-end test task",
		"mode": 0,
		"interval_ms": 1000,
		"priority": 1,
		"enabled": true,
		"labels": {"env": "test", "type": "e2e"}
	}`

	resp := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/tasks", strings.NewReader(createReq))
	req.Header.Set("Content-Type", "application/json")
	f.Router.ServeHTTP(resp, req)

	// 2. 验证 API 响应
	assert.Equal(http.StatusOK, resp.Code, "Create should return 200")
	assert.Contains(resp.Body.String(), "id", "Response should contain task ID")

	// 3. 从数据库验证任务存在
	ctx := context.Background()
	tasks, total, err := f.TaskRepo.List(ctx, &repository.TaskQuery{Page: 1, PageSize: 10})
	assert.NoError(err, "List should succeed")
	assert.Equal(int64(1), total, "Should have 1 task")
	assert.Equal("E2E Test Task", tasks[0].Name, "Task name should match")

	// 4. 验证时间轮中有任务
	taskID := tasks[0].ID
	time.Sleep(100 * time.Millisecond) // 等待同步
	twTask := f.TimeWheel.GetTask(taskID)
	assert.NotNil(twTask, "Task should be in TimeWheel")

	t.Logf("E2E flow verified: API -> DB -> TimeWheel, TaskID=%s", taskID)
}

// TestE2E_TaskExecution 测试任务执行
func TestE2E_TaskExecution(t *testing.T) {
	f := NewE2ETestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)

	// 创建一个会快速执行的任务
	// 使用高优先级（优先级 0）使任务被路由到高优先级时间轮（10ms 间隔）
	// 任务间隔 100ms 是时间轮间隔 10ms 的整数倍，确保正确调度
	taskID := "exec-test-001"
	var executionCount atomic.Int32

	task := fixtures.NewTaskFixture().
		WithID(taskID).
		WithName("Execution Test").
		WithPriority(model.TaskPriorityHigh). // 高优先级 -> 高优先级时间轮 (10ms)
		WithInterval(100).                     // 100ms 间隔 (是 10ms 的整数倍)
		ToTimeWheelTask(func(ctx context.Context) timewheelCore.AlarmResult {
			executionCount.Add(1)
			return timewheelCore.AlarmResult{
				Value:     85.0,
				Threshold: 80.0,
				IsFiring:  true,
			}
		})

	err := f.TimeWheel.AddTask(task)
	assert.NoError(err, "AddTask should succeed")

	// 等待任务执行 (等待足够长的时间让任务执行多次)
	time.Sleep(600 * time.Millisecond)

	// 验证任务执行了多次 (100ms 间隔，600ms 内应该执行 4-6 次)
	execCount := executionCount.Load()
	assert.GreaterThan(int(execCount), 2, "Task should have executed multiple times")

	t.Logf("Task executed %d times", execCount)
}

// TestE2E_ServerRestart_TaskRecovery 测试服务器重启任务恢复
// 注意：此测试只验证任务元数据的存储和加载，不验证任务执行
// 因为 Run 函数无法序列化，恢复的任务需要通过执行器注册机制来提供执行函数
func TestE2E_ServerRestart_TaskRecovery(t *testing.T) {
	// 使用文件数据库以便测试恢复
	dbFixture, err := fixtures.NewSQLiteFixture()
	if err != nil {
		t.Fatalf("Failed to create database fixture: %v", err)
	}
	defer dbFixture.Cleanup()

	assert := testutil.NewAssertion(t)

	// 1. 创建 SQLite 存储并保存任务（使用 timewheel 的存储接口）
	taskStore, _, err := timewheelCore.NewSQLiteStore(dbFixture.Path)
	if err != nil {
		t.Fatalf("Failed to create SQLite store: %v", err)
	}

	// 保存测试任务到 TimeWheel 的存储
	// 注意：Run 函数不会被序列化，所以只保存任务元数据
	for i := 0; i < 5; i++ {
		task := fixtures.NewTaskFixture().
			WithID(fmt.Sprintf("recovery-task-%d", i)).
			WithName(fmt.Sprintf("Recovery Task %d", i)).
			WithInterval(1000).
			ToTimeWheelTask(nil) // Run 函数不会被序列化
		err := taskStore.Save(task)
		assert.NoError(err, "Save should succeed")
	}

	// 2. 验证任务可以从存储加载
	tasks, err := taskStore.LoadEnabled()
	assert.NoError(err, "LoadEnabled should succeed")
	t.Logf("Loaded %d tasks from store", len(tasks))
	assert.Equal(len(tasks), 5, "Should load 5 tasks")

	// 3. 验证加载的任务元数据正确
	for i, task := range tasks {
		expectedID := fmt.Sprintf("recovery-task-%d", i)
		assert.Equal(task.ID, expectedID, "Task ID should match")
		assert.Equal(task.Interval, 1000*time.Millisecond, "Task interval should match")
		// Run 函数为 nil，因为函数无法序列化
		assert.Equal(task.Run, nil, "Run function should be nil (not serializable)")
	}

	// 注意：由于 Run 函数无法序列化，恢复的任务无法直接添加到时间轮执行
	// 实际应用中需要通过执行器注册机制来提供执行函数

	taskStore.Close()
}

// TestE2E_HighLoad_1000Tasks 测试 1000 任务负载
func TestE2E_HighLoad_1000Tasks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	f := NewE2ETestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()
	const taskCount = 1000

	// 批量创建任务
	var wg sync.WaitGroup
	var successCount atomic.Int32
	start := time.Now()

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			req := &dto.TaskCreateRequest{
				ID:         fmt.Sprintf("load-task-%d", index),
				Name:       fmt.Sprintf("Load Task %d", index),
				Mode:       int(model.TaskModeRepeated),
				IntervalMs: 10000, // 10秒间隔，避免频繁执行
				Enabled:    index%2 == 0,
			}

			_, err := f.TaskService.Create(ctx, req)
			if err == nil {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	// 验证
	successRate := float64(successCount.Load()) / float64(taskCount) * 100
	t.Logf("Created %d/%d tasks in %v (%.1f%% success)", successCount.Load(), taskCount, duration, successRate)

	assert.GreaterThan(successRate, 95.0, "Success rate should be at least 95%")

	// 检查时间轮中的任务
	twTasks := f.TimeWheel.GetAllTasks()
	t.Logf("TimeWheel tasks: %d", len(twTasks))

	// 清理
	for i := 0; i < taskCount; i++ {
		f.TaskService.Delete(ctx, fmt.Sprintf("load-task-%d", i))
	}
}

// TestE2E_ConcurrentAPIRequests 测试并发 API 请求
func TestE2E_ConcurrentAPIRequests(t *testing.T) {
	f := NewE2ETestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)
	const requestCount = 100

	var wg sync.WaitGroup
	statusCodes := make([]int, requestCount)

	for i := 0; i < requestCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// 交替创建和查询
			if index%2 == 0 {
				// 创建任务
				req := httptest.NewRequest("POST", "/api/v1/tasks",
					strings.NewReader(fmt.Sprintf(`{"name":"Concurrent Task %d","mode":0,"interval_ms":1000}`, index)))
				req.Header.Set("Content-Type", "application/json")
				resp := httptest.NewRecorder()
				f.Router.ServeHTTP(resp, req)
				statusCodes[index] = resp.Code
			} else {
				// 查询任务列表
				req := httptest.NewRequest("GET", "/api/v1/tasks", nil)
				resp := httptest.NewRecorder()
				f.Router.ServeHTTP(resp, req)
				statusCodes[index] = resp.Code
			}
		}(i)
	}

	wg.Wait()

	// 统计状态码
	successCount := 0
	for _, code := range statusCodes {
		if code >= 200 && code < 300 {
			successCount++
		}
	}

	successRate := float64(successCount) / float64(requestCount) * 100
	t.Logf("Success rate: %.1f%% (%d/%d)", successRate, successCount, requestCount)

	assert.GreaterThan(successRate, 90.0, "Success rate should be at least 90%")
}

// TestE2E_TaskLifecycle 测试任务生命周期
// 注意: 使用高优先级（优先级 0）使任务路由到高优先级时间轮（10ms 间隔）
// 这样可以避免任务间隔与时间轮间隔不匹配的问题
func TestE2E_TaskLifecycle(t *testing.T) {
	f := NewE2ETestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	taskID := "lifecycle-test"

	// 1. 创建 (使用普通优先级，避免 GORM 零值问题)
	_, err := f.TaskService.Create(ctx, &dto.TaskCreateRequest{
		ID:         taskID,
		Name:       "Lifecycle Test",
		Mode:       int(model.TaskModeRepeated),
		IntervalMs: 1000,
		// 不指定 Priority，使用默认值 1 (普通优先级)
		Enabled: false, // 初始禁用
	})
	assert.NoError(err, "Create should succeed")

	// 2. 启用
	err = f.TaskService.Enable(ctx, taskID)
	assert.NoError(err, "Enable should succeed")

	// 3. 暂停
	err = f.TaskService.Pause(ctx, taskID)
	assert.NoError(err, "Pause should succeed")

	// 4. 恢复
	err = f.TaskService.Resume(ctx, taskID)
	assert.NoError(err, "Resume should succeed")

	// 5. 禁用
	err = f.TaskService.Disable(ctx, taskID)
	assert.NoError(err, "Disable should succeed")

	// 6. 更新
	newName := "Updated Lifecycle Test"
	_, err = f.TaskService.Update(ctx, taskID, &dto.TaskUpdateRequest{
		Name: &newName,
	})
	assert.NoError(err, "Update should succeed")

	// 7. 删除
	err = f.TaskService.Delete(ctx, taskID)
	assert.NoError(err, "Delete should succeed")

	// 8. 验证删除
	_, err = f.TaskService.GetByID(ctx, taskID)
	assert.Error(err, "GetByID should fail after delete")

	t.Log("Task lifecycle test passed")
}

// TestE2E_PauseResume 测试暂停恢复
func TestE2E_PauseResume(t *testing.T) {
	f := NewE2ETestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)

	// 创建任务
	// 使用高优先级（优先级 0）使任务被路由到高优先级时间轮（10ms 间隔）
	// 任务间隔 100ms 是时间轮间隔 10ms 的整数倍，确保正确调度
	taskID := "pause-resume-test"
	var execCount atomic.Int32

	task := fixtures.NewTaskFixture().
		WithID(taskID).
		WithName("Pause Resume Test").
		WithPriority(model.TaskPriorityHigh). // 高优先级 -> 高优先级时间轮 (10ms)
		WithInterval(100).                     // 100ms 间隔 (是 10ms 的整数倍)
		ToTimeWheelTask(func(ctx context.Context) timewheelCore.AlarmResult {
			execCount.Add(1)
			return timewheelCore.AlarmResult{}
		})

	err := f.TimeWheel.AddTask(task)
	assert.NoError(err, "AddTask should succeed")

	// 等待执行 (等待足够长的时间让任务执行)
	time.Sleep(350 * time.Millisecond)
	countBeforePause := execCount.Load()

	// 暂停任务
	err = f.TimeWheel.PauseTask(taskID)
	assert.NoError(err, "PauseTask should succeed")

	// 等待并验证不执行
	time.Sleep(300 * time.Millisecond)
	countAfterPause := execCount.Load()

	// 恢复任务
	err = f.TimeWheel.ResumeTask(taskID)
	assert.NoError(err, "ResumeTask should succeed")

	// 等待执行
	time.Sleep(350 * time.Millisecond)
	countAfterResume := execCount.Load()

	t.Logf("Exec counts: before_pause=%d, after_pause=%d, after_resume=%d",
		countBeforePause, countAfterPause, countAfterResume)

	// 暂停后应该没有执行，恢复后应该继续执行
	assert.Equal(int(countAfterPause), int(countBeforePause), "Should not execute during pause")
	assert.GreaterThan(int(countAfterResume), int(countAfterPause), "Should execute after resume")
}

// TestE2E_ErrorHandling 测试错误处理
func TestE2E_ErrorHandling(t *testing.T) {
	f := NewE2ETestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)

	// 测试获取不存在的任务
	resp := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/tasks/nonexistent", nil)
	f.Router.ServeHTTP(resp, req)
	assert.Equal(http.StatusNotFound, resp.Code, "Should return 404 for nonexistent task")
}

// TestE2E_MultipleTasks 测试多任务
func TestE2E_MultipleTasks(t *testing.T) {
	f := NewE2ETestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// 创建多个任务
	const taskCount = 10
	for i := 0; i < taskCount; i++ {
		_, err := f.TaskService.Create(ctx, &dto.TaskCreateRequest{
			ID:         fmt.Sprintf("multi-task-%d", i),
			Name:       fmt.Sprintf("Multi Task %d", i),
			Mode:       int(model.TaskModeRepeated),
			IntervalMs: 1000,
			Enabled:    true,
		})
		assert.NoError(err, "Create should succeed")
	}

	// 验证所有任务在时间轮中
	twTasks := f.TimeWheel.GetAllTasks()
	assert.Equal(taskCount, len(twTasks), "All tasks should be in TimeWheel")

	t.Logf("Created and verified %d tasks", taskCount)
}
