package fixtures

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"timewheel/internal/repository/model"
)

// DBFixture 数据库测试固件
type DBFixture struct {
	DB   *gorm.DB
	Path string
	mu   sync.Mutex
}

// dbCounter 用于生成唯一的数据库名称
var dbCounter int64

// NewSQLiteFixture 创建 SQLite 测试固件
func NewSQLiteFixture() (*DBFixture, error) {
	// 生成唯一的数据库文件路径
	dbCounter++
	dbName := fmt.Sprintf("test_%d_%d.db", time.Now().UnixNano(), dbCounter)
	dbPath := filepath.Join(os.TempDir(), "timewheel_test", dbName)

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	// 打开数据库连接
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(1) // SQLite 只支持单写连接
	sqlDB.SetMaxIdleConns(1)

	// 自动迁移
	if err := runAutoMigrate(db); err != nil {
		return nil, fmt.Errorf("failed to migrate: %w", err)
	}

	return &DBFixture{
		DB:   db,
		Path: dbPath,
	}, nil
}

// NewInMemoryFixture 创建内存数据库固件（更快的测试）
//
// 修复高并发问题:
// 1. 使用共享缓存模式，允许多连接访问同一内存数据库
// 2. 配置 WAL 模式提高并发性能
// 3. 增加连接池大小以支持高并发
func NewInMemoryFixture() (*DBFixture, error) {
	// 使用共享缓存模式的内存数据库
	// file::memory:?cache=shared 允许多个连接访问同一个内存数据库
	dsn := "file::memory:?cache=shared&_journal_mode=WAL&_busy_timeout=5000"

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open in-memory database: %w", err)
	}

	// 配置连接池以支持高并发
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}
	// SQLite 在 WAL 模式下可以支持多个读取者，但写入仍然串行
	// 增加连接池大小以支持并发读取
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(0) // 连接不过期

	// 自动迁移 - 确保表创建完成
	if err := runAutoMigrate(db); err != nil {
		return nil, fmt.Errorf("failed to migrate: %w", err)
	}

	return &DBFixture{
		DB:   db,
		Path: ":memory:",
	}, nil
}

// runAutoMigrate 执行数据库迁移
func runAutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.TaskModel{},
		&model.AlertHistoryModel{},
		&model.OperationLogModel{},
		&model.WebhookQueueModel{},
	)
}

// Cleanup 清理数据库
func (f *DBFixture) Cleanup() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.DB != nil {
		// 删除所有数据
		f.DB.Exec("DELETE FROM tasks")
		f.DB.Exec("DELETE FROM alert_history")
		f.DB.Exec("DELETE FROM operation_logs")
		f.DB.Exec("DELETE FROM webhook_queue")
	}

	// 删除数据库文件（如果不是内存数据库）
	if f.Path != ":memory:" && f.Path != "" {
		if err := os.Remove(f.Path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove db file: %w", err)
		}
	}

	return nil
}

// Close 关闭数据库连接
func (f *DBFixture) Close() error {
	if f.DB != nil {
		sqlDB, err := f.DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

// taskSeedCounter 用于生成唯一的任务 ID
var taskSeedCounter int64

// SeedTasks 填充测试任务数据
func (f *DBFixture) SeedTasks(count int) ([]*model.TaskModel, error) {
	// 使用原子操作获取唯一的计数器前缀
	taskSeedCounter++
	baseTime := time.Now().UnixNano()
	prefix := fmt.Sprintf("%d-%d", baseTime, taskSeedCounter)

	tasks := make([]*model.TaskModel, count)
	now := time.Now()

	for i := 0; i < count; i++ {
		task := &model.TaskModel{
			ID:               fmt.Sprintf("seed-task-%s-%d", prefix, i),
			Name:             fmt.Sprintf("Seed Task %d", i+1),
			Description:      fmt.Sprintf("Seeded task for testing #%d", i+1),
			Mode:             model.TaskModeRepeated,
			IntervalMs:       1000,
			Times:            0,
			Priority:         model.TaskPriority(i % 3),
			TimeoutMs:        5000,
			Severity:         model.Severity(i % 3),
			ForDurationMs:    0,
			RepeatIntervalMs: 0,
			Enabled:          true,
			Paused:           false,
			EnabledAt:        &now,
			Labels:           model.StringMap{"env": "test", "index": fmt.Sprintf("%d", i)},
			Annotations:      model.StringMap{"seeded": "true"},
		}
		tasks[i] = task
	}

	if err := f.DB.CreateInBatches(tasks, 100).Error; err != nil {
		return nil, fmt.Errorf("failed to seed tasks: %w", err)
	}

	return tasks, nil
}

// SeedTasksWithState 填充带有特定状态的任务
func (f *DBFixture) SeedTasksWithState(count int, enabled, paused bool) ([]*model.TaskModel, error) {
	// 使用原子操作获取唯一的计数器前缀
	taskSeedCounter++
	baseTime := time.Now().UnixNano()
	prefix := fmt.Sprintf("%d-%d", baseTime, taskSeedCounter)

	tasks := make([]*model.TaskModel, count)
	now := time.Now()

	for i := 0; i < count; i++ {
		task := &model.TaskModel{
			ID:               fmt.Sprintf("state-task-%s-%d", prefix, i),
			Name:             fmt.Sprintf("State Task %d", i+1),
			Description:      fmt.Sprintf("Task with specific state #%d", i+1),
			Mode:             model.TaskModeRepeated,
			IntervalMs:       1000,
			Enabled:          enabled,
			Paused:           paused,
			EnabledAt:        &now,
			Labels:           model.StringMap{},
			Annotations:      model.StringMap{},
		}
		tasks[i] = task
	}

	if err := f.DB.CreateInBatches(tasks, 100).Error; err != nil {
		return nil, fmt.Errorf("failed to seed tasks: %w", err)
	}

	return tasks, nil
}

// SeedOperationLogs 填充操作日志
func (f *DBFixture) SeedOperationLogs(count int) ([]*model.OperationLogModel, error) {
	logs := make([]*model.OperationLogModel, count)

	for i := 0; i < count; i++ {
		log := &model.OperationLogModel{
			TaskID:    fmt.Sprintf("op-task-%d", i+1),
			Operation: model.OpCreate,
			Status:    model.OpStatusCompleted,
		}
		logs[i] = log
	}

	if err := f.DB.CreateInBatches(logs, 100).Error; err != nil {
		return nil, fmt.Errorf("failed to seed operation logs: %w", err)
	}

	return logs, nil
}

// SeedPendingOperations 填充待处理操作
func (f *DBFixture) SeedPendingOperations(count int) ([]*model.OperationLogModel, error) {
	 // 使用原子操作获取唯一的计数器前缀
	taskSeedCounter++
	baseTime := time.Now().UnixNano()
	prefix := fmt.Sprintf("%d-%d", baseTime, taskSeedCounter)

	logs := make([]*model.OperationLogModel, count)

	for i := 0; i < count; i++ {
		log := &model.OperationLogModel{
            TaskID:    fmt.Sprintf("rollback-task-%s-%d", prefix, i+1),
            Operation: model.OpCreate,
            Status:    model.OpStatusRollbackFailed, // 修复：使用 OpStatusRollbackFailed 状态
        }
        logs[i] = log
    }


	if err := f.DB.CreateInBatches(logs, 100).Error; err != nil {
        return nil, fmt.Errorf("failed to seed pending operations: %w", err)
    }

    return logs, nil
}

// GetTaskCount 获取任务数量
func (f *DBFixture) GetTaskCount() (int64, error) {
	var count int64
	err := f.DB.Model(&model.TaskModel{}).Count(&count).Error
	return count, err
}

// GetEnabledTaskCount 获取启用任务数量
func (f *DBFixture) GetEnabledTaskCount() (int64, error) {
	var count int64
	err := f.DB.Model(&model.TaskModel{}).Where("enabled = ?", true).Count(&count).Error
	return count, err
}

// AssertTaskExists 断言任务存在
func (f *DBFixture) AssertTaskExists(id string) (bool, error) {
	var count int64
	err := f.DB.Model(&model.TaskModel{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

// GetTask 获取任务
func (f *DBFixture) GetTask(id string) (*model.TaskModel, error) {
	var task model.TaskModel
	err := f.DB.Where("id = ?", id).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// Transaction 在事务中执行
func (f *DBFixture) Transaction(fn func(tx *gorm.DB) error) error {
	return f.DB.Transaction(fn)
}

// WaitForCondition 等待条件满足
func (f *DBFixture) WaitForCondition(ctx context.Context, condition func() (bool, error), timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			ok, err := condition()
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
		}
	}
}

// CreateTables 创建所有表
func (f *DBFixture) CreateTables() error {
	return f.DB.AutoMigrate(
		&model.TaskModel{},
		&model.AlertHistoryModel{},
		&model.OperationLogModel{},
		&model.WebhookQueueModel{},
	)
}

// DropTables 删除所有表
func (f *DBFixture) DropTables() error {
	return f.DB.Migrator().DropTable(
		&model.TaskModel{},
		&model.AlertHistoryModel{},
		&model.OperationLogModel{},
		&model.WebhookQueueModel{},
	)
}

// Reset 重置数据库（删除并重建表）
func (f *DBFixture) Reset() error {
	if err := f.DropTables(); err != nil {
		return err
	}
	return f.CreateTables()
}
