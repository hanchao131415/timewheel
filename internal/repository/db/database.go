package database

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"timewheel/internal/config"
	"timewheel/internal/repository/model"
)

var (
	// DB 全局数据库实例
	DB *gorm.DB
)

// Init 初始化数据库连接
func Init(cfg *config.DatabaseConfig) error {
	var err error
	var gormConfig *gorm.Config

	// 配置 GORM 日志
	gormConfig = &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	switch cfg.Type {
	case "mysql":
		DB, err = initMySQL(cfg.MySQL, gormConfig)
	case "sqlite":
		DB, err = initSQLite(cfg.SQLite, gormConfig)
	default:
		return fmt.Errorf("unsupported database type: %s", cfg.Type)
	}

	if err != nil {
		return fmt.Errorf("failed to connect database: %w", err)
	}

	// 自动迁移
	if err = autoMigrate(); err != nil {
		return fmt.Errorf("failed to auto migrate: %w", err)
	}

	zap.L().Info("Database initialized successfully",
		zap.String("type", cfg.Type),
	)

	return nil
}

// initMySQL 初始化 MySQL 连接
func initMySQL(cfg config.MySQLConfig, gormConfig *gorm.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local",
		cfg.Username,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
		cfg.Charset,
	)

	db, err := gorm.Open(mysql.Open(dsn), gormConfig)
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// 设置连接池参数
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	return db, nil
}

// initSQLite 初始化 SQLite 连接
func initSQLite(cfg config.SQLiteConfig, gormConfig *gorm.Config) (*gorm.DB, error) {
	// 确保数据库文件目录存在
	dir := filepath.Dir(cfg.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(cfg.Path), gormConfig)
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	// SQLite 连接池设置
	sqlDB.SetMaxOpenConns(1) // SQLite 只支持单个写连接
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// autoMigrate 自动迁移数据库表结构
func autoMigrate() error {
	return DB.AutoMigrate(
		&model.TaskModel{},
		&model.AlertHistoryModel{},
		&model.WebhookQueueModel{},
	)
}

// Close 关闭数据库连接
func Close() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

// GetDB 获取数据库实例
func GetDB() *gorm.DB {
	return DB
}
