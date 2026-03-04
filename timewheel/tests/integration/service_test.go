package integration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"timewheel/internal/model/dto"
	"timewheel/internal/repository"
	"timewheel/internal/repository/model"
	"timewheel/internal/service"
	"timewheel/pkg/snowflake"
	timewheelCore "timewheel/pkg/timewheel"
	"timewheel/tests/fixtures"
	"timewheel/tests/testutil"
)

// ServiceTestFixture 服务层测试固件
type ServiceTestFixture struct {
	DB          *gorm.DB
	DBFixture   *fixtures.DBFixture
	TaskRepo    repository.TaskRepository
	OpLogRepo   repository.OperationLogRepository
	TimeWheel   *timewheelCore.MultiLevelTimeWheel
	AtomicSvc   service.AtomicTaskService
	Logger      *zap.Logger
	Cleanup     func()
}

// NewServiceTestFixture 创建服务层测试固件
func NewServiceTestFixture(t *testing.T) *ServiceTestFixture {
	// 创建数据库
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database fixture: %v", err)
	}

	// 创建日志（使用开发日志来调试）
	logger, _ := zap.NewDevelopment()

	// 创建时间轮
	tw, err := timewheelCore.NewMultiLevelTimeWheel()
	if err != nil {
		dbFixture.Close()
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}

	// 创建仓储
	taskRepo := repository.NewTaskRepository(dbFixture.DB)
	opLogRepo := repository.NewOperationLogRepository(dbFixture.DB)

	// 创建原子服务
	atomicSvc := service.NewAtomicTaskService(taskRepo, opLogRepo, tw, nil, logger)

	// 启动时间轮
	if err := tw.Start(); err != nil {
		t.Fatalf("Failed to start TimeWheel: %v", err)
	}

	return &ServiceTestFixture{
		DB:        dbFixture.DB,
		DBFixture: dbFixture,
		TaskRepo:  taskRepo,
		OpLogRepo: opLogRepo,
		TimeWheel: tw,
		AtomicSvc: atomicSvc,
		Logger:    logger,
		Cleanup: func() {
			tw.Stop()
			dbFixture.Close()
		},
	}
}

// TestTaskService_Create_FullFlow 测试创建完整流程
func TestTaskService_Create_FullFlow(t *testing.T) {
	f := NewServiceTestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// 创建任务
	req := &dto.TaskCreateRequest{
		Name:        "Full Flow Test Task",
		Description: "Test task for full flow",
		Mode:        int(model.TaskModeRepeated),
		IntervalMs:  1000,
		Priority:    int(model.TaskPriorityNormal),
		Enabled:     true,
	}

	resp, err := f.AtomicSvc.Create(ctx, req)
	assert.NoError(err, "Create should succeed")
	assert.NotNil(resp, "Response should not be nil")
	assert.NotEmpty(resp.ID, "Task ID should be generated")

	// 验证数据库中有任务
	dbTask, err := f.TaskRepo.GetByID(ctx, resp.ID)
	assert.NoError(err, "GetByID should succeed")
	assert.Equal(req.Name, dbTask.Name, "Name should match")
	assert.True(dbTask.Enabled, "Task should be enabled")

	// 验证时间轮中有任务
	twTask := f.TimeWheel.GetTask(resp.ID)
	assert.NotNil(twTask, "Task should be in TimeWheel")

	t.Logf("Task created: ID=%s, DB exists=%v, TW exists=%v",
		resp.ID, dbTask != nil, twTask != nil)
}

// TestTaskService_EnableDisable_Sync 测试启用/禁用同步
func TestTaskService_EnableDisable_Sync(t *testing.T) {
	f := NewServiceTestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// 创建禁用的任务
	req := &dto.TaskCreateRequest{
		ID:         "enable-disable-test",
		Name:       "Enable Disable Test",
		Mode:       int(model.TaskModeRepeated),
		IntervalMs: 1000,
		Enabled:    false, // 初始禁用
	}
	_, err := f.AtomicSvc.Create(ctx, req)
	assert.NoError(err, "Create should succeed")

	// 验证时间轮中没有任务
	twTask := f.TimeWheel.GetTask("enable-disable-test")
	assert.Nil(twTask, "Task should not be in TimeWheel when disabled")

	// 启用任务
	err = f.AtomicSvc.Enable(ctx, "enable-disable-test")
	assert.NoError(err, "Enable should succeed")

	// 验证时间轮中有任务
	time.Sleep(100 * time.Millisecond) // 等待同步
	twTask = f.TimeWheel.GetTask("enable-disable-test")
	assert.NotNil(twTask, "Task should be in TimeWheel after enable")

	// 禁用任务
	err = f.AtomicSvc.Disable(ctx, "enable-disable-test")
	assert.NoError(err, "Disable should succeed")

	// 验证时间轮中没有任务
	twTask = f.TimeWheel.GetTask("enable-disable-test")
	assert.Nil(twTask, "Task should not be in TimeWheel after disable")
}

// TestTaskService_PauseResume_Execution 测试暂停/恢复执行
func TestTaskService_PauseResume_Execution(t *testing.T) {
	f := NewServiceTestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// 创建启用的任务
	req := &dto.TaskCreateRequest{
		ID:         "pause-resume-test",
		Name:       "Pause Resume Test",
		Mode:       int(model.TaskModeRepeated),
		IntervalMs: 100, // 100ms 间隔
		Enabled:    true,
	}
	_, err := f.AtomicSvc.Create(ctx, req)
	assert.NoError(err, "Create should succeed")

	// 等待任务执行几次
	time.Sleep(300 * time.Millisecond)

	// 暂停任务
	err = f.AtomicSvc.Pause(ctx, "pause-resume-test")
	assert.NoError(err, "Pause should succeed")

	// 验证数据库中暂停状态
	dbTask, err := f.TaskRepo.GetByID(ctx, "pause-resume-test")
	assert.NoError(err, "GetByID should succeed")
	assert.True(dbTask.Paused, "Task should be paused in DB")

	// 恢复任务
	err = f.AtomicSvc.Resume(ctx, "pause-resume-test")
	assert.NoError(err, "Resume should succeed")

	// 验证数据库中恢复状态
	dbTask, err = f.TaskRepo.GetByID(ctx, "pause-resume-test")
	assert.NoError(err, "GetByID should succeed")
	assert.False(dbTask.Paused, "Task should not be paused after resume")
}

// TestTaskService_ConcurrentOperations_100 测试 100 并发操作
func TestTaskService_ConcurrentOperations_100(t *testing.T) {
	f := NewServiceTestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()
	const opCount = 100

	var wg sync.WaitGroup
	errors := make(chan error, opCount)

	for i := 0; i < opCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			taskID := fmt.Sprintf("concurrent-task-%d", index)

			// 创建任务
			_, err := f.AtomicSvc.Create(ctx, &dto.TaskCreateRequest{
				ID:         taskID,
				Name:       fmt.Sprintf("Concurrent Task %d", index),
				Mode:       int(model.TaskModeRepeated),
				IntervalMs: 1000,
				Enabled:    true,
			})
			if err != nil {
				errors <- fmt.Errorf("create %d: %w", index, err)
				return
			}

			// 更新任务
			newName := fmt.Sprintf("Updated Task %d", index)
			_, err = f.AtomicSvc.Update(ctx, taskID, &dto.TaskUpdateRequest{
				Name: &newName,
			})
			if err != nil {
				errors <- fmt.Errorf("update %d: %w", index, err)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// 统计错误
	errorCount := 0
	for err := range errors {
		t.Logf("Concurrent operation error: %v", err)
		errorCount++
	}

	assert.LessThan(errorCount, opCount/10, "Error rate should be less than 10%")
	t.Logf("Completed %d concurrent operations with %d errors", opCount, errorCount)
}

// TestAtomicService_RepairInconsistentState 测试状态修复
func TestAtomicService_RepairInconsistentState(t *testing.T) {
	f := NewServiceTestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// 创建一些任务
	for i := 0; i < 5; i++ {
		req := &dto.TaskCreateRequest{
			ID:         fmt.Sprintf("repair-test-%d", i),
			Name:       fmt.Sprintf("Repair Test %d", i),
			Mode:       int(model.TaskModeRepeated),
			IntervalMs: 1000,
			Enabled:    true,
		}
		_, err := f.AtomicSvc.Create(ctx, req)
		assert.NoError(err, "Create should succeed")
	}

	// 手动制造不一致：从时间轮中移除一个任务但不从数据库移除
	f.TimeWheel.RemoveTask("repair-test-0")

	// 验证不一致状态
	twTask := f.TimeWheel.GetTask("repair-test-0")
	assert.Nil(twTask, "Task should not be in TimeWheel")
	dbTask, _ := f.TaskRepo.GetByID(ctx, "repair-test-0")
	assert.NotNil(dbTask, "Task should still be in DB")

	// 执行修复
	err := f.AtomicSvc.RepairInconsistentState(ctx)
	assert.NoError(err, "RepairInconsistentState should succeed")

	// 验证状态已修复
	time.Sleep(100 * time.Millisecond)
	twTask = f.TimeWheel.GetTask("repair-test-0")
	assert.NotNil(twTask, "Task should be back in TimeWheel after repair")

	t.Log("State repair successful")
}

// TestAtomicService_GetPendingOperations 测试获取待处理操作
func TestAtomicService_GetPendingOperations(t *testing.T) {
	f := NewServiceTestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// 创建一些待处理的操作日志
	_, err := f.DBFixture.SeedPendingOperations(5)
	assert.NoError(err, "SeedPendingOperations should succeed")

	// 获取待处理操作
	ops, err := f.AtomicSvc.GetPendingOperations(ctx)
	assert.NoError(err, "GetPendingOperations should succeed")
	assert.NotEmpty(ops, "Should have pending operations")

	t.Logf("Found %d pending operations", len(ops))
}

// TestTaskService_Update 测试更新任务
func TestTaskService_Update(t *testing.T) {
	f := NewServiceTestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// 创建任务
	req := &dto.TaskCreateRequest{
		ID:          "update-test",
		Name:        "Original Name",
		Description: "Original Description",
		Mode:        int(model.TaskModeRepeated),
		IntervalMs:  1000,
		Enabled:     true,
	}
	_, err := f.AtomicSvc.Create(ctx, req)
	assert.NoError(err, "Create should succeed")

	// 更新任务
	newName := "Updated Name"
	newDesc := "Updated Description"
	newInterval := int64(2000)
	resp, err := f.AtomicSvc.Update(ctx, "update-test", &dto.TaskUpdateRequest{
		Name:        &newName,
		Description: &newDesc,
		IntervalMs:  &newInterval,
	})
	assert.NoError(err, "Update should succeed")
	assert.Equal(newName, resp.Name, "Name should be updated")
	assert.Equal(newDesc, resp.Description, "Description should be updated")
	assert.Equal(newInterval, resp.IntervalMs, "IntervalMs should be updated")

	// 验证数据库更新
	dbTask, err := f.TaskRepo.GetByID(ctx, "update-test")
	assert.NoError(err, "GetByID should succeed")
	assert.Equal(newName, dbTask.Name, "DB name should be updated")
}

// TestTaskService_List 测试任务列表
func TestTaskService_List(t *testing.T) {
	f := NewServiceTestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// 创建多个任务
	for i := 0; i < 25; i++ {
		req := &dto.TaskCreateRequest{
			ID:         fmt.Sprintf("list-test-%d", i),
			Name:       fmt.Sprintf("List Test %d", i),
			Mode:       int(model.TaskModeRepeated),
			IntervalMs: 1000,
			Enabled:    i%2 == 0, // 一半启用
		}
		_, err := f.AtomicSvc.Create(ctx, req)
		assert.NoError(err, "Create should succeed")
	}

	// 测试分页
	resp, err := f.AtomicSvc.List(ctx, &repository.TaskQuery{
		Page:     1,
		PageSize: 10,
	})
	assert.NoError(err, "List should succeed")
	assert.Equal(10, len(resp.List), "Should return 10 items")
	assert.Equal(int64(25), resp.Total, "Total should be 25")

	// 测试过滤
	resp, err = f.AtomicSvc.List(ctx, &repository.TaskQuery{
		Page:     1,
		PageSize: 100,
		Enabled:  boolPtr(true),
	})
	assert.NoError(err, "List with filter should succeed")
	assert.Equal(13, len(resp.List), "Should return only enabled tasks (13 enabled out of 25)")
}

// TestTaskService_SnowflakeID 测试雪花算法 ID
func TestTaskService_SnowflakeID(t *testing.T) {
	f := NewServiceTestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// 不提供 ID，应该自动生成
	req := &dto.TaskCreateRequest{
		Name:       "Auto ID Test",
		Mode:       int(model.TaskModeRepeated),
		IntervalMs: 1000,
		Enabled:    true,
	}

	resp, err := f.AtomicSvc.Create(ctx, req)
	assert.NoError(err, "Create should succeed")
	assert.NotEmpty(resp.ID, "ID should be auto-generated")

	// 验证 ID 格式（雪花算法生成的 ID 是数字字符串）
	t.Logf("Auto-generated ID: %s", resp.ID)

	// 创建多个任务，验证 ID 唯一性
	ids := make(map[string]bool)
	ids[resp.ID] = true

	for i := 0; i < 100; i++ {
		req := &dto.TaskCreateRequest{
			Name:       fmt.Sprintf("Unique ID Test %d", i),
			Mode:       int(model.TaskModeRepeated),
			IntervalMs: 1000,
			Enabled:    true,
		}
		resp, err := f.AtomicSvc.Create(ctx, req)
		assert.NoError(err, "Create should succeed")

		if ids[resp.ID] {
			t.Errorf("Duplicate ID detected: %s", resp.ID)
		}
		ids[resp.ID] = true
	}

	t.Logf("Generated %d unique IDs", len(ids))
}

// TestTaskService_DuplicateID 测试重复 ID
func TestTaskService_DuplicateID(t *testing.T) {
	f := NewServiceTestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// 创建任务
	req := &dto.TaskCreateRequest{
		ID:         "duplicate-test",
		Name:       "Duplicate Test",
		Mode:       int(model.TaskModeRepeated),
		IntervalMs: 1000,
		Enabled:    true,
	}
	_, err := f.AtomicSvc.Create(ctx, req)
	assert.NoError(err, "First Create should succeed")

	// 尝试使用相同 ID 创建
	_, err = f.AtomicSvc.Create(ctx, req)
	assert.Error(err, "Second Create with same ID should fail")
	assert.Contains(err.Error(), "already exists", "Error message should mention duplicate")
}

// TestTaskService_HighConcurrency_500 测试高并发 500 个操作
func TestTaskService_HighConcurrency_500(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	f := NewServiceTestFixture(t)
	defer f.Cleanup()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()
	const concurrency = 500

	var wg sync.WaitGroup
	var successCount atomic.Int32
	var errorCount atomic.Int32

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			taskID := snowflake.GenerateString()
			req := &dto.TaskCreateRequest{
				ID:         taskID,
				Name:       fmt.Sprintf("High Concurrency Task %d", index),
				Mode:       int(model.TaskModeRepeated),
				IntervalMs: 1000,
				Enabled:    true,
			}

			_, err := f.AtomicSvc.Create(ctx, req)
			if err != nil {
				errorCount.Add(1)
			} else {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Success: %d, Errors: %d", successCount.Load(), errorCount.Load())
	assert.GreaterThan(int(successCount.Load()), concurrency*90/100, "At least 90% should succeed")
}

// Helper function
func boolPtr(b bool) *bool {
	return &b
}
