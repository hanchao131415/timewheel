package timewheel

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// AlertRuleModel 告警规则数据库模型
type AlertRuleModel struct {
	ID               string `gorm:"primaryKey"`
	Mode             int
	IntervalMs       int64
	Priority         int
	Severity         int
	ForDurationMs    int64
	RepeatIntervalMs int64
	Times            int
	TimeoutMs        int64
	Description      string
	Labels           string
	Annotations      string
	Enabled          int `gorm:"default:1"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// TableName 指定表名
func (AlertRuleModel) TableName() string {
	return "alert_rules"
}

// AlertHistoryModel 告警历史数据库模型
type AlertHistoryModel struct {
	ID           uint `gorm:"primaryKey;autoIncrement"`
	TaskID       string
	OldState     int
	NewState     int
	Value        float64
	Threshold    float64
	IsFiring     int
	Severity     int
	Labels       string
	Annotations  string
	CreatedAt    time.Time
}

// TableName 指定表名
func (AlertHistoryModel) TableName() string {
	return "alert_history"
}

// GormTaskStore GORM 任务存储
type GormTaskStore struct {
	db *gorm.DB
}

// GormHistoryStore GORM 历史存储
type GormHistoryStore struct {
	db       *gorm.DB
	recordCh chan AlertHistory
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewSQLiteStore 创建 SQLite 存储（使用 GORM）
func NewSQLiteStore(dbPath string) (*GormTaskStore, *GormHistoryStore, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 自动迁移表结构
	if err := db.AutoMigrate(&AlertRuleModel{}, &AlertHistoryModel{}); err != nil {
		return nil, nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	taskStore := &GormTaskStore{db: db}
	historyStore := &GormHistoryStore{
		db:       db,
		recordCh: make(chan AlertHistory, DefaultRecordChannelSize),
		stopCh:   make(chan struct{}),
	}

	// 启动异步写入器
	historyStore.startBatchWriter()

	return taskStore, historyStore, nil
}

// NewMySQLStore 创建 MySQL 存储（使用 GORM）
func NewMySQLStore(dsn string) (*GormTaskStore, *GormHistoryStore, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 自动迁移表结构
	if err := db.AutoMigrate(&AlertRuleModel{}, &AlertHistoryModel{}); err != nil {
		return nil, nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	taskStore := &GormTaskStore{db: db}
	historyStore := &GormHistoryStore{
		db:       db,
		recordCh: make(chan AlertHistory, DefaultRecordChannelSize),
		stopCh:   make(chan struct{}),
	}

	// 启动异步写入器
	historyStore.startBatchWriter()

	return taskStore, historyStore, nil
}

// Save 保存任务
func (s *GormTaskStore) Save(task *Task) error {
	if task == nil {
		return ErrInvalidParam
	}

	// 修复：处理 JSON 序列化错误
	labelsJSON, err := json.Marshal(task.Labels)
	if err != nil {
		return WrapError(err, "marshal labels for task %s", task.ID)
	}
	annotationsJSON, err := json.Marshal(task.Annotations)
	if err != nil {
		return WrapError(err, "marshal annotations for task %s", task.ID)
	}

	model := AlertRuleModel{
		ID:               task.ID,
		Mode:             int(task.Mode),
		IntervalMs:       task.Interval.Milliseconds(),
		Priority:         int(task.Priority),
		Severity:         int(task.Severity),
		ForDurationMs:    task.For.Milliseconds(),
		RepeatIntervalMs: task.RepeatInterval.Milliseconds(),
		Times:            task.Times,
		TimeoutMs:        task.Timeout.Milliseconds(),
		Description:      task.Description,
		Labels:           string(labelsJSON),
		Annotations:      string(annotationsJSON),
		Enabled:          1,
	}

	return s.db.Save(&model).Error
}

// Delete 删除任务
func (s *GormTaskStore) Delete(taskID string) error {
	return s.db.Delete(&AlertRuleModel{}, "id = ?", taskID).Error
}

// LoadAll 加载所有任务
func (s *GormTaskStore) LoadAll() ([]*Task, error) {
	return s.loadTasks(false)
}

// LoadEnabled 只加载启用的任务
func (s *GormTaskStore) LoadEnabled() ([]*Task, error) {
	return s.loadTasks(true)
}

func (s *GormTaskStore) loadTasks(enabledOnly bool) ([]*Task, error) {
	query := s.db.Model(&AlertRuleModel{})
	if enabledOnly {
		query = query.Where("enabled = ?", 1)
	}

	var models []AlertRuleModel
	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}

	tasks := make([]*Task, 0, len(models))
	for _, m := range models {
		task := &Task{
			ID:             m.ID,
			Mode:           TaskMode(m.Mode),
			Interval:       time.Duration(m.IntervalMs) * time.Millisecond,
			Priority:       TaskPriority(m.Priority),
			Severity:       Severity(m.Severity),
			For:            time.Duration(m.ForDurationMs) * time.Millisecond,
			RepeatInterval: time.Duration(m.RepeatIntervalMs) * time.Millisecond,
			Times:          m.Times,
			Timeout:        time.Duration(m.TimeoutMs) * time.Millisecond,
			Description:    m.Description,
			Labels:         make(map[string]string),
			Annotations:    make(map[string]string),
		}

		// 修复：处理 JSON 反序列化错误
		if m.Labels != "" {
			if err := json.Unmarshal([]byte(m.Labels), &task.Labels); err != nil {
				// 记录警告但继续加载
				fmt.Printf("[WARN] 反序列化 labels 失败 (task=%s): %v\n", m.ID, err)
			}
		}
		if m.Annotations != "" {
			if err := json.Unmarshal([]byte(m.Annotations), &task.Annotations); err != nil {
				// 记录警告但继续加载
				fmt.Printf("[WARN] 反序列化 annotations 失败 (task=%s): %v\n", m.ID, err)
			}
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}

// Close 关闭存储
func (s *GormTaskStore) Close() error {
	if s.db != nil {
		sqlDB, err := s.db.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

// Record 异步记录历史（非阻塞）
func (s *GormHistoryStore) Record(record AlertHistory) error {
	select {
	case s.recordCh <- record:
		return nil
	default:
		return fmt.Errorf("history record channel full")
	}
}

// GetHistory 获取历史记录
func (s *GormHistoryStore) GetHistory(taskID string, limit int) ([]AlertHistory, error) {
	query := s.db.Model(&AlertHistoryModel{}).Order("created_at DESC")

	if taskID != "" {
		query = query.Where("task_id = ?", taskID)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	var models []AlertHistoryModel
	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}

	history := make([]AlertHistory, 0, len(models))
	for _, m := range models {
		h := AlertHistory{
			TaskID:    m.TaskID,
			OldState:  AlertState(m.OldState),
			State:     AlertState(m.NewState),
			Value:     m.Value,
			Threshold: m.Threshold,
			IsFiring:  m.IsFiring != 0,
			Severity:  Severity(m.Severity),
			Timestamp: m.CreatedAt,
		}

		// 修复：处理 JSON 反序列化错误
		if m.Labels != "" {
			if err := json.Unmarshal([]byte(m.Labels), &h.Labels); err != nil {
				fmt.Printf("[WARN] 反序列化历史 labels 失败 (task=%s): %v\n", m.TaskID, err)
			}
		}
		if m.Annotations != "" {
			if err := json.Unmarshal([]byte(m.Annotations), &h.Annotations); err != nil {
				fmt.Printf("[WARN] 反序列化历史 annotations 失败 (task=%s): %v\n", m.TaskID, err)
			}
		}

		history = append(history, h)
	}

	return history, nil
}

// DeleteOlderThan 删除指定天数之前的记录
func (s *GormHistoryStore) DeleteOlderThan(days int) error {
	cutoff := time.Now().AddDate(0, 0, -days)
	return s.db.Where("created_at < ?", cutoff).Delete(&AlertHistoryModel{}).Error
}

// Close 关闭存储
func (s *GormHistoryStore) Close() error {
	close(s.stopCh)
	s.wg.Wait()
	if s.db != nil {
		sqlDB, err := s.db.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

// startBatchWriter 启动批量写入器
func (s *GormHistoryStore) startBatchWriter() {
	s.wg.Add(1)
	go s.runBatchWriter()
}

func (s *GormHistoryStore) runBatchWriter() {
	defer s.wg.Done()

	batch := make([]AlertHistory, 0, 100)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}

		models := make([]AlertHistoryModel, 0, len(batch))
		for _, h := range batch {
			// 修复：处理 JSON 序列化错误
			labelsJSON, err := json.Marshal(h.Labels)
			if err != nil {
				fmt.Printf("[WARN] 序列化历史 labels 失败 (task=%s): %v\n", h.TaskID, err)
				labelsJSON = []byte("{}")
			}
			annotationsJSON, err := json.Marshal(h.Annotations)
			if err != nil {
				fmt.Printf("[WARN] 序列化历史 annotations 失败 (task=%s): %v\n", h.TaskID, err)
				annotationsJSON = []byte("{}")
			}
			isFiring := 0
			if h.IsFiring {
				isFiring = 1
			}

			models = append(models, AlertHistoryModel{
				TaskID:      h.TaskID,
				OldState:    int(h.OldState),
				NewState:    int(h.State),
				Value:       h.Value,
				Threshold:   h.Threshold,
				IsFiring:    isFiring,
				Severity:    int(h.Severity),
				Labels:      string(labelsJSON),
				Annotations: string(annotationsJSON),
				CreatedAt:   h.Timestamp,
			})
		}

		// 修复：处理数据库写入错误
		if err := s.db.Create(&models).Error; err != nil {
			fmt.Printf("[ERROR] 批量写入历史记录失败: %v\n", err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case record := <-s.recordCh:
			batch = append(batch, record)
			if len(batch) >= 100 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-s.stopCh:
			flush()
			return
		}
	}
}
