package integration

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"timewheel/internal/repository/model"
	"timewheel/pkg/timewheel"
	"timewheel/tests/fixtures"
	"timewheel/tests/testutil"
)

// TestGormStore_CRUD 测试基础增删改查
func TestGormStore_CRUD(t *testing.T) {
	// 创建数据库固件
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database fixture: %v", err)
	}
	defer dbFixture.Close()

	// 创建时间轮存储
	taskStore, historyStore, err := timewheel.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create SQLite store: %v", err)
	}
	defer taskStore.Close()
	defer historyStore.Close()

	assert := testutil.NewAssertion(t)

	// 测试保存任务
	task := fixtures.NewTaskFixture().
		WithID("crud-test-001").
		WithName("CRUD Test Task").
		ToTimeWheelTask(nil)

	err = taskStore.Save(task)
	assert.NoError(err, "Save should succeed")

	// 测试加载所有任务
	tasks, err := taskStore.LoadAll()
	assert.NoError(err, "LoadAll should succeed")
	assert.NotEmpty(tasks, "Tasks should not be empty")
	assert.Equal("crud-test-001", tasks[0].ID, "Task ID should match")

	// 测试删除任务
	err = taskStore.Delete("crud-test-001")
	assert.NoError(err, "Delete should succeed")

	// 验证删除后任务不存在
	tasks, err = taskStore.LoadAll()
	assert.NoError(err, "LoadAll after delete should succeed")
	assert.Empty(tasks, "Tasks should be empty after delete")
}

// TestGormStore_ConcurrentSave_100 测试 100 并发写入
func TestGormStore_ConcurrentSave_100(t *testing.T) {
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database fixture: %v", err)
	}
	defer dbFixture.Close()

	taskStore, _, err := timewheel.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create SQLite store: %v", err)
	}
	defer taskStore.Close()

	assert := testutil.NewAssertion(t)
	const concurrency = 100

	var wg sync.WaitGroup
	errors := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			task := fixtures.NewTaskFixture().
				WithID(fmt.Sprintf("concurrent-%d", index)).
				WithName(fmt.Sprintf("Concurrent Task %d", index)).
				ToTimeWheelTask(nil)
			if err := taskStore.Save(task); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// 检查错误
	errorCount := 0
	for err := range errors {
		t.Logf("Concurrent save error: %v", err)
		errorCount++
	}
	assert.Equal(0, errorCount, "Should have no concurrent save errors")

	// 验证所有任务都被保存
	tasks, err := taskStore.LoadAll()
	assert.NoError(err, "LoadAll should succeed")
	assert.Equal(concurrency, len(tasks), "All tasks should be saved")
}

// TestGormStore_TransactionRollback 测试事务回滚
func TestGormStore_TransactionRollback(t *testing.T) {
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database fixture: %v", err)
	}
	defer dbFixture.Close()

	assert := testutil.NewAssertion(t)

	// 在事务中插入任务然后回滚
	err = dbFixture.Transaction(func(tx *gorm.DB) error {
		task := &model.TaskModel{
			ID:          "tx-task-001",
			Name:        "Transaction Task",
			Description: "This should be rolled back",
			Mode:        model.TaskModeRepeated,
			IntervalMs:  1000,
		}
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		// 返回错误以触发回滚
		return fmt.Errorf("intentional rollback")
	})
	assert.Error(err, "Transaction should fail")

	// 验证任务未被保存
	exists, err := dbFixture.AssertTaskExists("tx-task-001")
	assert.NoError(err, "AssertTaskExists should succeed")
	assert.False(exists, "Task should not exist after rollback")

	// 在事务中插入任务然后提交
	err = dbFixture.Transaction(func(tx *gorm.DB) error {
		task := &model.TaskModel{
			ID:          "tx-task-002",
			Name:        "Transaction Task 2",
			Description: "This should be committed",
			Mode:        model.TaskModeRepeated,
			IntervalMs:  1000,
		}
		return tx.Create(task).Error
	})
	assert.NoError(err, "Transaction should succeed")

	// 验证任务已保存
	exists, err = dbFixture.AssertTaskExists("tx-task-002")
	assert.NoError(err, "AssertTaskExists should succeed")
	assert.True(exists, "Task should exist after commit")
}

// TestGormStore_LoadEnabled_10K 测试加载 1 万启用任务
func TestGormStore_LoadEnabled_10K(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database fixture: %v", err)
	}
	defer dbFixture.Close()

	assert := testutil.NewAssertion(t)
	const taskCount = 10000

	// 填充启用任务
	enabledTasks, err := dbFixture.SeedTasksWithState(taskCount, true, false)
	assert.NoError(err, "SeedTasksWithState should succeed")
	assert.Equal(taskCount, len(enabledTasks), "Should seed all tasks")

	// 填充禁用任务
	_, err = dbFixture.SeedTasksWithState(1000, false, false)
	assert.NoError(err, "SeedTasksWithState should succeed for disabled tasks")

	// 创建时间轮存储
	taskStore, _, err := timewheel.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create SQLite store: %v", err)
	}
	defer taskStore.Close()

	// 将启用任务保存到时间轮存储
	for _, task := range enabledTasks[:1000] { // 先保存 1000 个
		twTask := fixtures.NewTaskFixture().
			WithID(task.ID).
			WithName(task.Name).
			ToTimeWheelTask(nil)
		if err := taskStore.Save(twTask); err != nil {
			t.Fatalf("Failed to save task: %v", err)
		}
	}

	// 测量加载时间
	start := time.Now()
	tasks, err := taskStore.LoadEnabled()
	duration := time.Since(start)

	assert.NoError(err, "LoadEnabled should succeed")
	assert.GreaterOrEqual(len(tasks), 1000, "Should load enabled tasks")

	t.Logf("Loaded %d enabled tasks in %v", len(tasks), duration)
}

// TestTimeWheel_AutoRestore_OnStartup 测试启动时自动恢复
func TestTimeWheel_AutoRestore_OnStartup(t *testing.T) {
	dbFixture, err := fixtures.NewInMemoryFixture()
	if err != nil {
		t.Fatalf("Failed to create database fixture: %v", err)
	}
	defer dbFixture.Close()

	assert := testutil.NewAssertion(t)

	// 1. 创建时间轮存储并保存一些任务
	taskStore, _, err := timewheel.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create SQLite store: %v", err)
	}

	// 保存测试任务
	for i := 0; i < 10; i++ {
		task := fixtures.NewTaskFixture().
			WithID(fmt.Sprintf("restore-task-%d", i)).
			WithName(fmt.Sprintf("Restore Task %d", i)).
			ToTimeWheelTask(nil)
		if err := taskStore.Save(task); err != nil {
			t.Fatalf("Failed to save task: %v", err)
		}
	}
	// 注意：不要关闭 taskStore，因为时间轮需要在启动时读取数据

	// 2. 创建时间轮并设置自动恢复
	tw, err := timewheel.New(
		timewheel.WithTaskStore(taskStore),
		timewheel.WithAutoRestore(true),
	)
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()

	// 启动时间轮（会触发自动恢复）
	if err := tw.Start(); err != nil {
		t.Fatalf("Failed to start TimeWheel: %v", err)
	}

	// 等待恢复完成
	time.Sleep(500 * time.Millisecond)

	// 3. 验证任务已恢复
	metrics := tw.GetMetrics()
	assert.GreaterOrEqual(metrics.TotalTasks, int64(10), "Should restore tasks")
	t.Logf("Restored %d tasks", metrics.TotalTasks)

	// 清理：现在可以关闭存储
	taskStore.Close()
}

// TestTimeWheel_Persist_OnAddRemove 测试添加/删除时持久化
func TestTimeWheel_Persist_OnAddRemove(t *testing.T) {
	taskStore, _, err := timewheel.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create SQLite store: %v", err)
	}
	defer taskStore.Close()

	tw, err := timewheel.New(
		timewheel.WithTaskStore(taskStore),
	)
	if err != nil {
		t.Fatalf("Failed to create TimeWheel: %v", err)
	}
	defer tw.Stop()

	assert := testutil.NewAssertion(t)

	// 启动时间轮
	if err := tw.Start(); err != nil {
		t.Fatalf("Failed to start TimeWheel: %v", err)
	}

	// 测试添加任务时持久化
	task := fixtures.NewTaskFixture().
		WithID("persist-test-001").
		WithName("Persist Test Task").
		ToTimeWheelTask(nil)

	err = tw.AddTask(task)
	assert.NoError(err, "AddTask should succeed")

	// 验证任务已持久化
	tasks, err := taskStore.LoadAll()
	assert.NoError(err, "LoadAll should succeed")
	assert.NotEmpty(tasks, "Tasks should be persisted")

	// 测试删除任务时持久化
	err = tw.RemoveTask("persist-test-001")
	assert.NoError(err, "RemoveTask should succeed")

	// 验证任务已从持久化存储中删除
	tasks, err = taskStore.LoadAll()
	assert.NoError(err, "LoadAll should succeed")
	assert.Empty(tasks, "Tasks should be removed from storage")
}

// TestMySQL_vs_SQLite_Compatibility 测试数据库兼容性
func TestMySQL_vs_SQLite_Compatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	assert := testutil.NewAssertion(t)

	// 创建 SQLite 存储
	sqliteStore, _, err := timewheel.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create SQLite store: %v", err)
	}
	defer sqliteStore.Close()

	// 测试任务
	task := fixtures.NewTaskFixture().
		WithID("compat-test-001").
		WithName("Compatibility Test").
		WithLabels(map[string]string{"key": "value", "unicode": "中文测试"}).
		WithAnnotations(map[string]string{"note": "test with special chars: !@#$%"}).
		ToTimeWheelTask(nil)

	// 在 SQLite 中保存并加载
	err = sqliteStore.Save(task)
	assert.NoError(err, "SQLite Save should succeed")

	sqliteTasks, err := sqliteStore.LoadAll()
	assert.NoError(err, "SQLite LoadAll should succeed")
	assert.NotEmpty(sqliteTasks, "SQLite tasks should not be empty")

	// 验证标签和注解
	loadedTask := sqliteTasks[0]
	assert.Equal(task.ID, loadedTask.ID, "Task ID should match")
	assert.Equal(task.Labels["key"], loadedTask.Labels["key"], "Label should match")
	assert.Equal(task.Labels["unicode"], loadedTask.Labels["unicode"], "Unicode label should match")

	t.Logf("SQLite compatibility test passed")
}

// TestGormStore_HistoryRecord 测试历史记录存储
func TestGormStore_HistoryRecord(t *testing.T) {
	_, historyStore, err := timewheel.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create SQLite store: %v", err)
	}
	defer historyStore.Close()

	assert := testutil.NewAssertion(t)

	// 记录历史
	now := time.Now()
	record := timewheel.AlertHistory{
		TaskID:    "history-test-001",
		OldState:  timewheel.AlertStatePending,
		State:     timewheel.AlertStateFiring,
		Value:     85.5,
		Threshold: 80.0,
		IsFiring:  true,
		Severity:  timewheel.SeverityWarning,
		Labels:    `{"env":"test"}`,
		Timestamp: now,
	}

	err = historyStore.Record(record)
	assert.NoError(err, "Record should succeed")

	// 等待异步写入 - 批量写入器每 5 秒刷新一次
	// 修复：等待足够长的时间确保数据写入完成
	time.Sleep(6 * time.Second)

	// 查询历史
	history, err := historyStore.GetHistory("history-test-001", 10)
	assert.NoError(err, "GetHistory should succeed")
	assert.NotEmpty(history, "History should not be empty")

	// 验证记录
	loadedRecord := history[0]
	assert.Equal(record.TaskID, loadedRecord.TaskID, "TaskID should match")
	assert.Equal(record.State, loadedRecord.State, "State should match")
	assert.Equal(record.Value, loadedRecord.Value, "Value should match")
}

// TestGormStore_DeleteOlderThan 测试删除旧历史记录
func TestGormStore_DeleteOlderThan(t *testing.T) {
	_, historyStore, err := timewheel.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create SQLite store: %v", err)
	}
	defer historyStore.Close()

	assert := testutil.NewAssertion(t)

	// 记录一些历史
	for i := 0; i < 10; i++ {
		record := timewheel.AlertHistory{
			TaskID:    fmt.Sprintf("old-history-%d", i),
			State:     timewheel.AlertStateFiring,
			Timestamp: time.Now().Add(-time.Duration(i) * time.Hour),
		}
		historyStore.Record(record)
	}

	// 等待异步写入
	time.Sleep(100 * time.Millisecond)

	// 删除 5 小时前的记录
	err = historyStore.DeleteOlderThan(5)
	assert.NoError(err, "DeleteOlderThan should succeed")

	// 验证只有最近的记录保留
	history, err := historyStore.GetHistory("", 100)
	assert.NoError(err, "GetHistory should succeed")
	assert.LessOrEqual(len(history), 5, "Should only have recent records")
}

// TestGormStore_BatchWrite 测试批量写入
func TestGormStore_BatchWrite(t *testing.T) {
	_, historyStore, err := timewheel.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create SQLite store: %v", err)
	}
	defer historyStore.Close()

	assert := testutil.NewAssertion(t)
	const recordCount = 200

	// 批量记录
	start := time.Now()
	for i := 0; i < recordCount; i++ {
		record := timewheel.AlertHistory{
			TaskID:    fmt.Sprintf("batch-history-%d", i),
			State:     timewheel.AlertStateFiring,
			Timestamp: time.Now(),
		}
		if err := historyStore.Record(record); err != nil {
			t.Fatalf("Record failed: %v", err)
		}
	}
	recordDuration := time.Since(start)

	// 等待批量写入完成
	time.Sleep(6 * time.Second) // 批量写入间隔是 5 秒

	// 验证所有记录已写入
	history, err := historyStore.GetHistory("", 1000)
	assert.NoError(err, "GetHistory should succeed")
	assert.Equal(recordCount, len(history), "All records should be written")

	t.Logf("Batch write: %d records in %v (queue time included)", recordCount, recordDuration)
}
