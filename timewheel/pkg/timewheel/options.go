package timewheel

import (
	"log"
	"time"
)

// Option 时间轮配置选项
type Option func(*TimeWheel)

// WithSlotNum 设置槽位数量
//
// 参数：
//   - num: 槽位数量，必须大于0
//
// 返回值：
//   - Option: 配置选项函数
func WithSlotNum(num int) Option {
	return func(tw *TimeWheel) {
		tw.slotNum = num
		tw.requestedSlotNum = num
		tw.requestedSlotNumSet = true
	}
}

// WithInterval 设置时间间隔
//
// 参数：
//   - interval: 时间间隔，必须大于0
//
// 返回值：
//   - Option: 配置选项函数
func WithInterval(interval time.Duration) Option {
	return func(tw *TimeWheel) {
		tw.interval = interval
		tw.requestedInterval = interval
		tw.requestedIntervalSet = true
	}
}

// WithLogger 设置自定义日志记录器
//
// 参数：
//   - logger: 日志记录器
//
// 返回值：
//   - Option: 配置选项
func WithLogger(logger *log.Logger) Option {
	return func(tw *TimeWheel) {
		tw.logger = logger
	}
}

// WithLogLevel 设置日志级别
//
// 参数：
//   - level: 日志级别，0=Debug, 1=Info, 2=Warn, 3=Error
//
// 返回值：
//   - Option: 配置选项
func WithLogLevel(level int) Option {
	return func(tw *TimeWheel) {
		tw.logLevel = level
	}
}

// WithStatusInterval 设置状态打印间隔
//
// 参数：
//   - interval: 状态打印间隔
//
// 返回值：
//   - Option: 配置选项
func WithStatusInterval(interval time.Duration) Option {
	return func(tw *TimeWheel) {
		tw.statusInterval = interval
		tw.statusEnabled = interval > 0
	}
}

// WithErrorCallback 设置错误回调
//
// 参数：
//   - fn: 错误处理回调函数
//
// 返回值：
//   - Option: 配置选项函数
func WithErrorCallback(fn func(error)) Option {
	return func(tw *TimeWheel) {
		if fn != nil {
			tw.onError = fn
		}
	}
}

// WithMaxConcurrentTasks 设置最大并发任务数
//
// 参数：
//   - max: 最大并发任务数，0表示无限制
//
// 返回值：
//   - Option: 配置选项函数
func WithMaxConcurrentTasks(max int) Option {
	return func(tw *TimeWheel) {
		if max > 0 {
			tw.maxConcurrentTasks = max
		}
	}
}

// WithHistoryFile 设置告警历史文件
//
// 参数：
//   - filePath: 历史文件路径
//
// 返回值：
//   - Option: 配置选项
func WithHistoryFile(filePath string) Option {
	return func(tw *TimeWheel) {
		// 支持空路径的内存模式
		tw.historyManager = NewAlertHistoryManager(filePath, 30)
	}
}

// WithHistoryRetention 设置告警历史保留天数
//
// 参数：
//   - days: 保留天数
//
// 返回值：
//   - Option: 配置选项
func WithHistoryRetention(days int) Option {
	return func(tw *TimeWheel) {
		if days > 0 && tw.historyManager != nil {
			tw.historyManager.retentionDays = days
		}
	}
}

// WithCache 启用任务缓存（优化GetTask性能）
//
// 参数：
//   - enabled: 是否启用缓存
//
// 返回值：
//   - Option: 配置选项
func WithCache(enabled bool) Option {
	return func(tw *TimeWheel) {
		tw.cacheEnabled = enabled
	}
}

// WithSQLiteStore 配置 SQLite 存储
//
// 参数：
//   - dbPath: 数据库文件路径
//
// 返回值：
//   - Option: 配置选项
func WithSQLiteStore(dbPath string) Option {
	return func(tw *TimeWheel) {
		taskStore, historyStore, err := NewSQLiteStore(dbPath)
		if err == nil {
			tw.taskStore = taskStore
			tw.historyStore = historyStore
		}
	}
}

// WithMySQLStore 配置 MySQL 存储
//
// 参数：
//   - dsn: MySQL 连接字符串 (e.g., "user:password@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=True&loc=Local")
//
// 返回值：
//   - Option: 配置选项
func WithMySQLStore(dsn string) Option {
	return func(tw *TimeWheel) {
		taskStore, historyStore, err := NewMySQLStore(dsn)
		if err == nil {
			tw.taskStore = taskStore
			tw.historyStore = historyStore
		}
	}
}

// WithTaskStore 配置自定义任务存储
//
// 参数：
//   - store: 任务存储实现
//
// 返回值：
//   - Option: 配置选项
func WithTaskStore(store TaskStore) Option {
	return func(tw *TimeWheel) {
		tw.taskStore = store
	}
}

// WithHistoryStore 配置自定义历史存储
//
// 参数：
//   - store: 历史存储实现
//
// 返回值：
//   - Option: 配置选项
func WithHistoryStore(store HistoryStore) Option {
	return func(tw *TimeWheel) {
		tw.historyStore = store
	}
}

// WithAutoRestore 配置启动时自动恢复任务
//
// 参数：
//   - enabled: 是否启用自动恢复
//
// 返回值：
//   - Option: 配置选项
func WithAutoRestore(enabled bool) Option {
	return func(tw *TimeWheel) {
		tw.autoRestore = enabled
	}
}
