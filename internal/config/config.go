package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config 主配置结构
type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Mode       string           `mapstructure:"mode"`
	TimeWheel  TimeWheelConfig  `mapstructure:"timewheel"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Redis      RedisConfig      `mapstructure:"redis"`
	Alert      AlertConfig      `mapstructure:"alert"`
	Logging    LoggingConfig    `mapstructure:"logging"`
	Auth       AuthConfig       `mapstructure:"auth"`
	RateLimit  RateLimitConfig  `mapstructure:"rate_limit"`
	Metrics    MetricsConfig    `mapstructure:"metrics"`
	Health     HealthConfig     `mapstructure:"health"`
}

// ServerConfig HTTP 服务器配置
type ServerConfig struct {
	Port            int           `mapstructure:"port"`
	Mode            string        `mapstructure:"mode"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// TimeWheelLevelConfig 时间轮层级配置
type TimeWheelLevelConfig struct {
	Interval time.Duration `mapstructure:"interval"`
	Slots    int           `mapstructure:"slots"`
}

// TimeWheelConfig 时间轮配置
type TimeWheelConfig struct {
	Levels             map[string]TimeWheelLevelConfig `mapstructure:"levels"`
	MaxConcurrentTasks int                             `mapstructure:"max_concurrent_tasks"`
	CacheEnabled       bool                            `mapstructure:"cache_enabled"`
	CacheSize          int                             `mapstructure:"cache_size"`
}

// MySQLConfig MySQL 配置
type MySQLConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	Username        string        `mapstructure:"username"`
	Password        string        `mapstructure:"password"`
	Database        string        `mapstructure:"database"`
	Charset         string        `mapstructure:"charset"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// SQLiteConfig SQLite 配置
type SQLiteConfig struct {
	Path string `mapstructure:"path"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Type   string       `mapstructure:"type"`
	MySQL  MySQLConfig  `mapstructure:"mysql"`
	SQLite SQLiteConfig `mapstructure:"sqlite"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	Addr         string `mapstructure:"addr"`
	Password     string `mapstructure:"password"`
	DB           int    `mapstructure:"db"`
	PoolSize     int    `mapstructure:"pool_size"`
	MinIdleConns int    `mapstructure:"min_idle_conns"`
}

// WebhookRetryConfig Webhook 重试配置
type WebhookRetryConfig struct {
	MaxAttempts     int           `mapstructure:"max_attempts"`
	InitialInterval time.Duration `mapstructure:"initial_interval"`
	MaxInterval     time.Duration `mapstructure:"max_interval"`
	Multiplier      float64       `mapstructure:"multiplier"`
}

// WebhookBatchConfig Webhook 批量配置
type WebhookBatchConfig struct {
	Enabled  bool          `mapstructure:"enabled"`
	Size     int           `mapstructure:"size"`
	Interval time.Duration `mapstructure:"interval"`
}

// WebhookConfig Webhook 配置
type WebhookConfig struct {
	URL     string            `mapstructure:"url"`
	Timeout time.Duration     `mapstructure:"timeout"`
	Retry   WebhookRetryConfig `mapstructure:"retry"`
	Batch   WebhookBatchConfig `mapstructure:"batch"`
	Headers map[string]string `mapstructure:"headers"`
}

// AlertConfig 告警配置
type AlertConfig struct {
	Webhook WebhookConfig `mapstructure:"webhook"`
}

// LogOutputConfig 日志输出配置
type LogOutputConfig struct {
	Type string `mapstructure:"type"` // stdout / file / both
	Path string `mapstructure:"path"`
}

// LogRotationConfig 日志轮转配置
type LogRotationConfig struct {
	MaxSize    int  `mapstructure:"max_size"`    // MB
	MaxAge     int  `mapstructure:"max_age"`     // days
	MaxBackups int  `mapstructure:"max_backups"`
	Compress   bool `mapstructure:"compress"`
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Level    string           `mapstructure:"level"`
	Format   string           `mapstructure:"format"`
	Output   LogOutputConfig  `mapstructure:"output"`
	Rotation LogRotationConfig `mapstructure:"rotation"`
}

// JWTConfig JWT 配置
type JWTConfig struct {
	Secret string        `mapstructure:"secret"`
	Issuer string        `mapstructure:"issuer"`
	Expiry time.Duration `mapstructure:"expiry"`
}

// APIKeyEntry API Key 条目
type APIKeyEntry struct {
	Name string `mapstructure:"name"`
	Key  string `mapstructure:"key"`
}

// APIKeyConfig API Key 配置
type APIKeyConfig struct {
	Header string        `mapstructure:"header"`
	Keys   []APIKeyEntry `mapstructure:"keys"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	Enabled bool        `mapstructure:"enabled"`
	Type    string      `mapstructure:"type"`
	JWT     JWTConfig   `mapstructure:"jwt"`
	APIKey  APIKeyConfig `mapstructure:"api_key"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	Enabled           bool    `mapstructure:"enabled"`
	Type              string  `mapstructure:"type"`
	RequestsPerSecond float64 `mapstructure:"requests_per_second"`
	Burst             int     `mapstructure:"burst"`
	ByIP              bool    `mapstructure:"by_ip"`
	ByAPIKey          bool    `mapstructure:"by_api_key"`
}

// MetricsConfig Prometheus 指标配置
type MetricsConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Path      string `mapstructure:"path"`
	Namespace string `mapstructure:"namespace"`
	Subsystem string `mapstructure:"subsystem"`
}

// HealthConfig 健康检查配置
type HealthConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	LivePath  string `mapstructure:"live_path"`
	ReadyPath string `mapstructure:"ready_path"`
}

// 全局配置实例
var globalConfig *Config

// Load 加载配置文件
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// 设置默认值
	setDefaults(v)

	// 设置配置文件
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/timewheel")
	}

	// 支持环境变量覆盖
	v.AutomaticEnv()
	v.SetEnvPrefix("TIMEWHEEL")

	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// 配置文件不存在，使用默认值
	}

	// 解析配置
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 验证配置
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	globalConfig = &cfg
	return &cfg, nil
}

// setDefaults 设置默认值
func setDefaults(v *viper.Viper) {
	// Server 默认值
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "release")
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.shutdown_timeout", "10s")

	// Mode 默认值
	v.SetDefault("mode", "standalone")

	// TimeWheel 默认值
	v.SetDefault("timewheel.levels.high.interval", "100ms")
	v.SetDefault("timewheel.levels.high.slots", 60)
	v.SetDefault("timewheel.levels.normal.interval", "1s")
	v.SetDefault("timewheel.levels.normal.slots", 60)
	v.SetDefault("timewheel.levels.low.interval", "10s")
	v.SetDefault("timewheel.levels.low.slots", 60)
	v.SetDefault("timewheel.max_concurrent_tasks", 1000)
	v.SetDefault("timewheel.cache_enabled", true)
	v.SetDefault("timewheel.cache_size", 100000)

	// Database 默认值
	v.SetDefault("database.type", "mysql")
	v.SetDefault("database.mysql.host", "127.0.0.1")
	v.SetDefault("database.mysql.port", 3306)
	v.SetDefault("database.mysql.charset", "utf8mb4")
	v.SetDefault("database.mysql.max_open_conns", 100)
	v.SetDefault("database.mysql.max_idle_conns", 10)
	v.SetDefault("database.mysql.conn_max_lifetime", "1h")
	v.SetDefault("database.sqlite.path", "./data/timewheel.db")

	// Redis 默认值
	v.SetDefault("redis.enabled", false)
	v.SetDefault("redis.addr", "127.0.0.1:6379")
	v.SetDefault("redis.pool_size", 100)
	v.SetDefault("redis.min_idle_conns", 10)

	// Alert 默认值
	v.SetDefault("alert.webhook.timeout", "10s")
	v.SetDefault("alert.webhook.retry.max_attempts", 3)
	v.SetDefault("alert.webhook.retry.initial_interval", "1s")
	v.SetDefault("alert.webhook.retry.max_interval", "30s")
	v.SetDefault("alert.webhook.retry.multiplier", 2.0)
	v.SetDefault("alert.webhook.batch.enabled", true)
	v.SetDefault("alert.webhook.batch.size", 100)
	v.SetDefault("alert.webhook.batch.interval", "5s")

	// Logging 默认值
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output.type", "file")
	v.SetDefault("logging.output.path", "./logs/timewheel.log")
	v.SetDefault("logging.rotation.max_size", 100)
	v.SetDefault("logging.rotation.max_age", 30)
	v.SetDefault("logging.rotation.max_backups", 10)
	v.SetDefault("logging.rotation.compress", true)

	// Auth 默认值
	v.SetDefault("auth.enabled", false)
	v.SetDefault("auth.type", "jwt")
	v.SetDefault("auth.jwt.issuer", "timewheel")
	v.SetDefault("auth.jwt.expiry", "24h")
	v.SetDefault("auth.api_key.header", "X-API-Key")

	// RateLimit 默认值
	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.type", "token_bucket")
	v.SetDefault("rate_limit.requests_per_second", 1000)
	v.SetDefault("rate_limit.burst", 2000)
	v.SetDefault("rate_limit.by_ip", true)
	v.SetDefault("rate_limit.by_api_key", true)

	// Metrics 默认值
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.path", "/metrics")
	v.SetDefault("metrics.namespace", "timewheel")
	v.SetDefault("metrics.subsystem", "scheduler")

	// Health 默认值
	v.SetDefault("health.enabled", true)
	v.SetDefault("health.live_path", "/health/live")
	v.SetDefault("health.ready_path", "/health/ready")
}

// validate 验证配置
func validate(cfg *Config) error {
	// 验证运行模式
	if cfg.Mode != "standalone" && cfg.Mode != "cluster" {
		return fmt.Errorf("invalid mode: %s, must be standalone or cluster", cfg.Mode)
	}

	// 集群模式必须启用 Redis
	if cfg.Mode == "cluster" && !cfg.Redis.Enabled {
		return fmt.Errorf("redis must be enabled in cluster mode")
	}

	// 验证数据库类型
	if cfg.Database.Type != "mysql" && cfg.Database.Type != "sqlite" {
		return fmt.Errorf("invalid database type: %s, must be mysql or sqlite", cfg.Database.Type)
	}

	// 验证认证类型
	if cfg.Auth.Enabled {
		if cfg.Auth.Type != "jwt" && cfg.Auth.Type != "api_key" {
			return fmt.Errorf("invalid auth type: %s, must be jwt or api_key", cfg.Auth.Type)
		}
	}

	return nil
}

// Get 获取全局配置
func Get() *Config {
	return globalConfig
}

// IsCluster 是否集群模式
func IsCluster() bool {
	return globalConfig != nil && globalConfig.Mode == "cluster"
}

// IsStandalone 是否单机模式
func IsStandalone() bool {
	return globalConfig == nil || globalConfig.Mode == "standalone"
}
