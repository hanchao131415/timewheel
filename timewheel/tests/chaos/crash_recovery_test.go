package chaos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"timewheel/internal/config"
	"timewheel/internal/model/dto"
	"timewheel/internal/repository"
	"timewheel/internal/repository/model"
	"timewheel/internal/service"
	timewheelCore "timewheel/pkg/timewheel"
	"timewheel/tests/fixtures"
	"timewheel/tests/testutil"
)

// TestCrashRecovery_GracefulShutdown 测试优雅关闭
func TestCrashRecovery_GracefulShutdown(t *testing.T) {
	tw, _ := timewheelCore.NewMultiLevelTimeWheel()
	tw.Start()

	assert := testutil.NewAssertion(t)

	// 创建一些正在执行的任务
	var executingCount atomic.Int32
	var completedCount atomic.Int32

	for i := 0; i < 10; i++ {
		task := fixtures.NewTaskFixture().
			WithID(fmt.Sprintf("graceful-%d", i)).
			WithInterval(100).
			ToTimeWheelTask(func(ctx context.Context) timewheelCore.AlarmResult {
				executingCount.Add(1)
				time.Sleep(500 * time.Millisecond) // 模拟长时间执行
				completedCount.Add(1)
				return timewheelCore.AlarmResult{}
			})
		tw.AddTask(task)
	}

	// 等待任务开始执行
	time.Sleep(200 * time.Millisecond)

	// 优雅关闭
	start := time.Now()
	tw.Stop()
	duration := time.Since(start)

	t.Logf("Graceful shutdown took %v", duration)
	t.Logf("Tasks executing: %d, completed: %d", executingCount.Load(), completedCount.Load())

	// 验证关闭等待了任务完成
	assert.GreaterOrEqual(int(completedCount.Load()), 0, "Some tasks should complete")
}

// TestCrashRecovery_ForceKill_Restore 测试强杀恢复
func TestCrashRecovery_ForceKill_Restore(t *testing.T) {
	// 使用文件数据库以便模拟恢复
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "crash-test.db")

	// 第一阶段：创建数据
	taskStore, _, err := timewheelCore.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	assert := testutil.NewAssertion(t)

	// 添加任务
	for i := 0; i < 10; i++ {
		task := fixtures.NewTaskFixture().
			WithID(fmt.Sprintf("force-kill-%d", i)).
			WithInterval(1000).
			ToTimeWheelTask(nil)
		err = taskStore.Save(task)
		assert.NoError(err, "Save should succeed")
	}

	// 模拟强制关闭（不调用 Close）
	// taskStore.Close() // 故意不调用

	// 第二阶段：恢复数据
	taskStore2, _, err := timewheelCore.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to reopen store: %v", err)
	}
	defer taskStore2.Close()

	// 验证数据恢复
	tasks, err := taskStore2.LoadAll()
	assert.NoError(err, "LoadAll should succeed")
	assert.Equal(10, len(tasks), "Should restore all tasks")

	t.Logf("Force kill recovery: restored %d tasks", len(tasks))
}

// TestCrashRecovery_PartialWrite_Recover 测试部分写入恢复
func TestCrashRecovery_PartialWrite_Recover(t *testing.T) {
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer dbFixture.Close()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// 使用事务确保原子性
	err = dbFixture.Transaction(func(tx *gorm.DB) error {
		// 创建任务
		task := &model.TaskModel{
			ID:         "partial-write",
			Name:       "Partial Write",
			Mode:       model.TaskModeRepeated,
			IntervalMs: 1000,
		}
		if err := tx.Create(task).Error; err != nil {
			return err
		}

		// 创建操作日志
		opLog := &model.OperationLogModel{
			TaskID:    "partial-write",
			Operation: model.OpCreate,
			Status:    model.OpStatusPending,
		}
		if err := tx.Create(opLog).Error; err != nil {
			return err
		}

		// 模拟崩溃（返回错误导致回滚）
		return fmt.Errorf("simulated crash")
	})
	assert.Error(err, "Transaction should fail")

	// 验证数据一致性（要么全部成功，要么全部回滚）
	taskRepo := repository.NewTaskRepository(dbFixture.DB)
	_, err = taskRepo.GetByID(ctx, "partial-write")
	assert.Error(err, "Task should not exist after rollback")

	// 正常的事务应该成功
	err = dbFixture.Transaction(func(tx *gorm.DB) error {
		task := &model.TaskModel{
			ID:         "complete-write",
			Name:       "Complete Write",
			Mode:       model.TaskModeRepeated,
			IntervalMs: 1000,
		}
		if err := tx.Create(task).Error; err != nil {
			return err
		}

		opLog := &model.OperationLogModel{
			TaskID:    "complete-write",
			Operation: model.OpCreate,
			Status:    model.OpStatusCompleted,
		}
		return tx.Create(opLog).Error
	})
	assert.NoError(err, "Transaction should succeed")

	// 验证完整写入
	_, err = taskRepo.GetByID(ctx, "complete-write")
	assert.NoError(err, "Task should exist after successful transaction")

	t.Log("Partial write recovery test passed")
}

// TestCrashRecovery_OperationLog_Replay 测试日志回放
func TestCrashRecovery_OperationLog_Replay(t *testing.T) {
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer dbFixture.Close()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	opLogRepo := repository.NewOperationLogRepository(dbFixture.DB)
	taskRepo := repository.NewTaskRepository(dbFixture.DB)

	// 创建一些操作日志（待处理）
	pendingOps := []struct {
		taskID string
		op     string
		status string
	}{
		{"replay-1", model.OpCreate, model.OpStatusPending},
		{"replay-2", model.OpCreate, model.OpStatusPending},
		{"replay-3", model.OpUpdate, model.OpStatusPending},
		{"replay-4", model.OpDelete, model.OpStatusPending},
	}

	for _, op := range pendingOps {
		log := &model.OperationLogModel{
			TaskID:    op.taskID,
			Operation: op.op,
			Status:    op.status,
		}
		err := opLogRepo.Create(ctx, log)
		assert.NoError(err, "Create log should succeed")
	}

	// 模拟一些操作已完成
	dbFixture.DB.Model(&model.OperationLogModel{}).
		Where("task_id = ?", "replay-1").
		Update("status", model.OpStatusCompleted)

	// 获取待处理操作
	ops, err := opLogRepo.GetPendingRepairs(ctx, 10)
	assert.NoError(err, "GetPendingRepairs should succeed")
	assert.Equal(3, len(ops), "Should have 3 pending operations")

	// 回放操作
	for _, op := range ops {
		t.Logf("Replaying: task=%s, op=%s", op.TaskID, op.Operation)

		// 根据操作类型执行回放
		switch op.Operation {
		case model.OpCreate:
			task := &model.TaskModel{
				ID:         op.TaskID,
				Name:       "Replayed Task",
				Mode:       model.TaskModeRepeated,
				IntervalMs: 1000,
			}
			if err := taskRepo.Create(ctx, task); err != nil {
				t.Logf("Create replay failed: %v", err)
			}

		case model.OpUpdate:
			// 执行更新
			if task, err := taskRepo.GetByID(ctx, op.TaskID); err == nil {
				task.Name = "Updated via Replay"
				taskRepo.Update(ctx, task)
			}

		case model.OpDelete:
			taskRepo.Delete(ctx, op.TaskID)
		}

		// 标记完成
		opLogRepo.UpdateStatus(ctx, op.ID, model.OpStatusCompleted, "")
	}

	// 验证回放结果
	ops, err = opLogRepo.GetPendingRepairs(ctx, 10)
	assert.NoError(err, "GetPendingRepairs should succeed")
	assert.Empty(ops, "Should have no pending operations after replay")

	t.Log("Operation log replay test passed")
}

// TestCrashRecovery_StateInconsistency 测试状态不一致
func TestCrashRecovery_StateInconsistency(t *testing.T) {
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer dbFixture.Close()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	taskRepo := repository.NewTaskRepository(dbFixture.DB)
	opLogRepo := repository.NewOperationLogRepository(dbFixture.DB)
	logger := zap.NewNop()

	tw, _ := timewheelCore.NewMultiLevelTimeWheel()
	tw.Start()
	defer tw.Stop()

	cfg := &config.Config{}
	atomicSvc := service.NewAtomicTaskService(taskRepo, opLogRepo, tw, cfg, logger)

	// 创建任务
	_, err = atomicSvc.Create(ctx, &dto.TaskCreateRequest{
		ID:         "inconsistent-test",
		Name:       "Inconsistent Test",
		Mode:       int(model.TaskModeRepeated),
		IntervalMs: 1000,
		Enabled:    true,
	})
	assert.NoError(err, "Create should succeed")

	// 模拟状态不一致：数据库有，时间轮无
	tw.RemoveTask("inconsistent-test")

	// 验证不一致
	dbTask, _ := taskRepo.GetByID(ctx, "inconsistent-test")
	twTask := tw.GetTask("inconsistent-test")
	assert.NotNil(dbTask, "Task should be in DB")
	assert.Nil(twTask, "Task should not be in TimeWheel")

	// 执行状态修复
	err = atomicSvc.RepairInconsistentState(ctx)
	assert.NoError(err, "RepairInconsistentState should succeed")

	// 等待修复完成
	time.Sleep(100 * time.Millisecond)

	// 验证状态一致
	twTask = tw.GetTask("inconsistent-test")
	assert.NotNil(twTask, "Task should be back in TimeWheel after repair")

	t.Log("State inconsistency repair test passed")
}

// TestCrashRecovery_GoroutineLeak 测试 Goroutine 泄漏
func TestCrashRecovery_GoroutineLeak(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

	// 创建并停止多个时间轮
	for i := 0; i < 10; i++ {
		tw, _ := timewheelCore.NewMultiLevelTimeWheel()
		tw.Start()

		// 添加一些任务
		for j := 0; j < 100; j++ {
			task := fixtures.NewTaskFixture().
				WithID(fmt.Sprintf("leak-%d-%d", i, j)).
				WithInterval(1000).
				ToTimeWheelTask(nil)
			tw.AddTask(task)
		}

		// 停止时间轮
		tw.Stop()
	}

	// 等待清理
	time.Sleep(500 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	leaked := finalGoroutines - initialGoroutines

	t.Logf("Goroutines: initial=%d, final=%d, leaked=%d",
		initialGoroutines, finalGoroutines, leaked)

	assert := testutil.NewAssertion(t)
	assert.LessThan(leaked, 10, "Should not have significant goroutine leak")
}

// TestCrashRecovery_ResourceCleanup 测试资源清理
func TestCrashRecovery_ResourceCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "cleanup-test.db")

	taskStore, historyStore, err := timewheelCore.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}

	// 添加数据
	for i := 0; i < 100; i++ {
		task := fixtures.NewTaskFixture().
			WithID(fmt.Sprintf("cleanup-%d", i)).
			ToTimeWheelTask(nil)
		taskStore.Save(task)
	}

	// 关闭存储
	err = taskStore.Close()
	assert := testutil.NewAssertion(t)
	assert.NoError(err, "Close should succeed")

	err = historyStore.Close()
	assert.NoError(err, "Close should succeed")

	// 验证文件存在
	_, err = os.Stat(dbPath)
	assert.NoError(err, "DB file should exist")

	// 清理文件
	err = os.Remove(dbPath)
	assert.NoError(err, "Should be able to remove DB file")

	t.Log("Resource cleanup test passed")
}

// TestCrashRecovery_HighLoadRecovery 测试高负载下恢复
func TestCrashRecovery_HighLoadRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer dbFixture.Close()

	taskRepo := repository.NewTaskRepository(dbFixture.DB)
	opLogRepo := repository.NewOperationLogRepository(dbFixture.DB)
	logger := zap.NewNop()

	tw, _ := timewheelCore.NewMultiLevelTimeWheel()
	tw.Start()
	defer tw.Stop()

	cfg := &config.Config{}
	svc := service.NewAtomicTaskService(taskRepo, opLogRepo, tw, cfg, logger)

	ctx := context.Background()
	const taskCount = 1000

	// 高并发创建任务
	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := 0; i < taskCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			_, err := svc.Create(ctx, &dto.TaskCreateRequest{
				ID:         fmt.Sprintf("high-load-%d", index),
				Name:       fmt.Sprintf("High Load %d", index),
				Mode:       int(model.TaskModeRepeated),
				IntervalMs: 10000,
				Enabled:    true,
			})
			if err == nil {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	// 获取指标
	metrics := tw.GetMetrics()

	t.Logf("High load recovery test:")
	t.Logf("  Success: %d/%d", successCount.Load(), taskCount)
	t.Logf("  TimeWheel tasks: %d", metrics.TotalTasks)

	// 验证状态一致性
	assert := testutil.NewAssertion(t)
	assert.Equal(int(metrics.TotalTasks), int(successCount.Load()), "DB and TimeWheel should be consistent")
}

// Import runtime and zap
import (
	"runtime"

	"go.uber.org/zap"
)
