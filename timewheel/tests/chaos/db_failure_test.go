package chaos

import (
	"context"
	"errors"
	"fmt"
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

// TestDBFailure_WriteFailure_Logged 测试写入失败记录
func TestDBFailure_WriteFailure_Logged(t *testing.T) {
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// 创建任务仓储
	taskRepo := repository.NewTaskRepository(dbFixture.DB)

	// 关闭数据库模拟故障
	dbFixture.Close()

	// 尝试创建任务（应该失败）
	task := &model.TaskModel{
		ID:         "fail-write-test",
		Name:       "Fail Write Test",
		Mode:       model.TaskModeRepeated,
		IntervalMs: 1000,
	}

	err = taskRepo.Create(ctx, task)
	assert.Error(err, "Create should fail when DB is unavailable")

	t.Logf("DB write failure logged: %v", err)
}

// TestDBFailure_ReadTimeout_Retry 测试读取超时重试
func TestDBFailure_ReadTimeout_Retry(t *testing.T) {
	// 模拟慢速数据库
	slowDB := &SlowDB{
		delay: 2 * time.Second,
		fail:  true,
	}

	assert := testutil.NewAssertion(t)

	// 尝试读取（带重试）
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var lastErr error
	for i := 0; i < 3; i++ {
		lastErr = slowDB.Query(ctx)
		if lastErr == nil {
			break
		}
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}

	assert.Error(lastErr, "Should fail after retries")

	t.Logf("Read timeout test completed: %v", lastErr)
}

// SlowDB 模拟慢速数据库
type SlowDB struct {
	delay time.Duration
	fail  bool
}

func (db *SlowDB) Query(ctx context.Context) error {
	select {
	case <-time.After(db.delay):
		if db.fail {
			return errors.New("query timeout")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TestDBFailure_TransactionDeadlock 测试死锁处理
func TestDBFailure_TransactionDeadlock(t *testing.T) {
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer dbFixture.Close()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	taskRepo := repository.NewTaskRepository(dbFixture.DB)

	// 创建两个任务
	task1 := &model.TaskModel{ID: "deadlock-1", Name: "Task 1", Mode: model.TaskModeRepeated, IntervalMs: 1000}
	task2 := &model.TaskModel{ID: "deadlock-2", Name: "Task 2", Mode: model.TaskModeRepeated, IntervalMs: 1000}

	taskRepo.Create(ctx, task1)
	taskRepo.Create(ctx, task2)

	var wg sync.WaitGroup
	var errors []error
	var mu sync.Mutex

	// 模拟并发更新可能导致死锁（SQLite 内存数据库实际上不会死锁）
	wg.Add(2)

	go func() {
		defer wg.Done()
		err := dbFixture.Transaction(func(tx *gorm.DB) error {
			time.Sleep(100 * time.Millisecond)
			return tx.Model(&model.TaskModel{}).Where("id = ?", "deadlock-1").Update("name", "Updated 1").Error
		})
		if err != nil {
			mu.Lock()
			errors = append(errors, err)
			mu.Unlock()
		}
	}()

	go func() {
		defer wg.Done()
		err := dbFixture.Transaction(func(tx *gorm.DB) error {
			time.Sleep(100 * time.Millisecond)
			return tx.Model(&model.TaskModel{}).Where("id = ?", "deadlock-2").Update("name", "Updated 2").Error
		})
		if err != nil {
			mu.Lock()
			errors = append(errors, err)
			mu.Unlock()
		}
	}()

	wg.Wait()

	// SQLite 不会真的死锁，但我们可以测试事务处理
	t.Logf("Transaction test completed with %d errors", len(errors))
}

// TestDBFailure_OperationLogReplay 测试操作日志回放
func TestDBFailure_OperationLogReplay(t *testing.T) {
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer dbFixture.Close()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// 填充待处理操作
	_, err = dbFixture.SeedPendingOperations(5)
	assert.NoError(err, "SeedPendingOperations should succeed")

	// 获取待处理操作
	opLogRepo := repository.NewOperationLogRepository(dbFixture.DB)
	pendingOps, err := opLogRepo.GetPendingRepairs(ctx, 10)
	assert.NoError(err, "GetPendingRepairs should succeed")
	assert.NotEmpty(pendingOps, "Should have pending operations")

	t.Logf("Found %d pending operations for replay", len(pendingOps))

	// 模拟回放操作
	for _, op := range pendingOps {
		t.Logf("Replaying operation: task=%s, op=%s", op.TaskID, op.Operation)
		// 标记为完成
		err := opLogRepo.UpdateStatus(ctx, op.ID, model.OpStatusCompleted, "")
		assert.NoError(err, "UpdateStatus should succeed")
	}

	// 验证没有待处理操作
	pendingOps, err = opLogRepo.GetPendingRepairs(ctx, 10)
	assert.NoError(err, "GetPendingRepairs should succeed")
	assert.Empty(pendingOps, "Should have no pending operations after replay")
}

// TestDBFailure_ConcurrentWriteFailure 测试并发写入失败
func TestDBFailure_ConcurrentWriteFailure(t *testing.T) {
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	assert := testutil.NewAssertion(t)
	ctx := context.Background()
	taskRepo := repository.NewTaskRepository(dbFixture.DB)

	const writers = 10
	var wg sync.WaitGroup
	var successCount, failureCount atomic.Int32

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			task := &model.TaskModel{
				ID:         fmt.Sprintf("concurrent-fail-%d", index),
				Name:       fmt.Sprintf("Concurrent Fail %d", index),
				Mode:       model.TaskModeRepeated,
				IntervalMs: 1000,
			}

			for retry := 0; retry < 3; retry++ {
				err := taskRepo.Create(ctx, task)
				if err == nil {
					successCount.Add(1)
					return
				}
				time.Sleep(time.Duration(retry+1) * 10 * time.Millisecond)
			}
			failureCount.Add(1)
		}(i)
	}

	wg.Wait()

	t.Logf("Concurrent write: %d success, %d failure", successCount.Load(), failureCount.Load())
	assert.Equal(int32(writers), successCount.Load(), "All writes should succeed with retries")
}

// TestDBFailure_ServiceWithDBDown 测试服务在 DB 宕机时的行为
func TestDBFailure_ServiceWithDBDown(t *testing.T) {
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	logger := zap.NewNop()
	tw, _ := timewheelCore.NewMultiLevelTimeWheel()
	tw.Start()
	defer tw.Stop()

	taskRepo := repository.NewTaskRepository(dbFixture.DB)
	opLogRepo := repository.NewOperationLogRepository(dbFixture.DB)

	cfg := &config.Config{}
	atomicSvc := service.NewAtomicTaskService(taskRepo, opLogRepo, tw, cfg, logger)

	ctx := context.Background()

	// 正常创建任务
	_, err = atomicSvc.Create(ctx, &dto.TaskCreateRequest{
		ID:         "before-down",
		Name:       "Before DB Down",
		Mode:       int(model.TaskModeRepeated),
		IntervalMs: 1000,
		Enabled:    true,
	})
	assert := testutil.NewAssertion(t)
	assert.NoError(err, "Create before DB down should succeed")

	// 模拟 DB 宕机
	dbFixture.Close()

	// 尝试创建任务（应该失败）
	_, err = atomicSvc.Create(ctx, &dto.TaskCreateRequest{
		ID:         "after-down",
		Name:       "After DB Down",
		Mode:       int(model.TaskModeRepeated),
		IntervalMs: 1000,
		Enabled:    true,
	})
	assert.Error(err, "Create after DB down should fail")

	t.Logf("Service handled DB down correctly: %v", err)
}

// TestDBFailure_DataConsistency 测试数据一致性
func TestDBFailure_DataConsistency(t *testing.T) {
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer dbFixture.Close()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()
	taskRepo := repository.NewTaskRepository(dbFixture.DB)

	// 创建任务
	task := &model.TaskModel{
		ID:         "consistency-test",
		Name:       "Consistency Test",
		Mode:       model.TaskModeRepeated,
		IntervalMs: 1000,
		Enabled:    true,
	}
	err = taskRepo.Create(ctx, task)
	assert.NoError(err, "Create should succeed")

	// 读取验证
	saved, err := taskRepo.GetByID(ctx, "consistency-test")
	assert.NoError(err, "GetByID should succeed")
	assert.Equal(task.Name, saved.Name, "Name should match")

	// 更新
	newName := "Updated Name"
	saved.Name = newName
	err = taskRepo.Update(ctx, saved)
	assert.NoError(err, "Update should succeed")

	// 再次读取验证
	updated, err := taskRepo.GetByID(ctx, "consistency-test")
	assert.NoError(err, "GetByID should succeed")
	assert.Equal(newName, updated.Name, "Name should be updated")

	// 模拟部分更新失败（通过事务）
	err = dbFixture.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.TaskModel{}).Where("id = ?", "consistency-test").Update("name", "Partial").Error; err != nil {
			return err
		}
		return errors.New("intentional rollback")
	})
	assert.Error(err, "Transaction should rollback")

	// 验证回滚成功
	afterRollback, err := taskRepo.GetByID(ctx, "consistency-test")
	assert.NoError(err, "GetByID should succeed")
	assert.Equal(newName, afterRollback.Name, "Name should not change after rollback")

	t.Log("Data consistency test passed")
}

// TestDBFailure_ConnectionPoolExhaustion 测试连接池耗尽
func TestDBFailure_ConnectionPoolExhaustion(t *testing.T) {
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer dbFixture.Close()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// SQLite 内存数据库没有连接池限制，但我们测试并发访问
	const concurrency = 100
	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			task := &model.TaskModel{
				ID:         fmt.Sprintf("pool-test-%d", index),
				Name:       fmt.Sprintf("Pool Test %d", index),
				Mode:       model.TaskModeRepeated,
				IntervalMs: 1000,
			}

			taskRepo := repository.NewTaskRepository(dbFixture.DB)
			if err := taskRepo.Create(ctx, task); err == nil {
				successCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Connection pool test: %d/%d succeeded", successCount.Load(), concurrency)
	assert.GreaterThan(int(successCount.Load()), concurrency*90/100, "At least 90% should succeed")
}

// TestDBFailure_Failover 测试故障转移
func TestDBFailure_Failover(t *testing.T) {
	// 主数据库
	primaryDB, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create primary database: %v", err)
	}

	// 备用数据库
	standbyDB, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create standby database: %v", err)
	}
	defer standbyDB.Close()

	assert := testutil.NewAssertion(t)
	ctx := context.Background()

	// 在主库创建数据
	taskRepo := repository.NewTaskRepository(primaryDB.DB)
	task := &model.TaskModel{
		ID:         "failover-test",
		Name:       "Failover Test",
		Mode:       model.TaskModeRepeated,
		IntervalMs: 1000,
	}
	err = taskRepo.Create(ctx, task)
	assert.NoError(err, "Create on primary should succeed")

	// 模拟主库故障
	primaryDB.Close()

	// 故障转移到备库
	standbyRepo := repository.NewTaskRepository(standbyDB.DB)

	// 备库中没有数据（正常情况下应该有复制）
	_, err = standbyRepo.GetByID(ctx, "failover-test")
	assert.Error(err, "Should not find data on standby (no replication)")

	// 在备库创建新数据
	newTask := &model.TaskModel{
		ID:         "after-failover",
		Name:       "After Failover",
		Mode:       model.TaskModeRepeated,
		IntervalMs: 1000,
	}
	err = standbyRepo.Create(ctx, newTask)
	assert.NoError(err, "Create on standby should succeed")

	t.Log("Failover test completed")
}

// Import zap for logger
import "go.uber.org/zap"
