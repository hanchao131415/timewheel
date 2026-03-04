package timewheel

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// AlertHistory 告警历史记录
type AlertHistory struct {
	TaskID      string     `json:"task_id"`     // 任务ID
	State       AlertState `json:"state"`       // 告警状态
	OldState    AlertState `json:"old_state"`   // 之前的状态
	Timestamp   time.Time  `json:"timestamp"`   // 时间戳
	Value       float64    `json:"value"`       // 当前值
	Threshold   float64    `json:"threshold"`   // 阈值
	IsFiring    bool       `json:"is_firing"`   // 是否触发
	Severity    Severity   `json:"severity"`    // 告警级别
	Labels      string     `json:"labels"`      // 标签(JSON)
	Annotations string     `json:"annotations"` // 描述(JSON)
}

// AlertHistoryManager 告警历史管理器
type AlertHistoryManager struct {
	mu            sync.RWMutex
	history       []AlertHistory // 内存存储
	filePath      string         // 文件路径
	retentionDays int            // 保留天数
	persisted     bool           // 是否已持久化
	maxSize       int            // 最大记录数（修复：无界内存增长）
}

const (
	// DefaultMaxHistorySize 默认最大历史记录数
	DefaultMaxHistorySize = 10000
	// AutoPersistThreshold 自动持久化阈值
	AutoPersistThreshold = 1000
)

// NewAlertHistoryManager 创建告警历史管理器
//
// 参数：
//   - filePath: 历史文件路径
//   - retentionDays: 保留天数
//
// 返回值：
//   - *AlertHistoryManager: 历史管理器实例
func NewAlertHistoryManager(filePath string, retentionDays int) *AlertHistoryManager {
	mgr := &AlertHistoryManager{
		history:       make([]AlertHistory, 0),
		filePath:      filePath,
		retentionDays: retentionDays,
		persisted:     false,
		maxSize:       DefaultMaxHistorySize, // 修复：设置最大限制
	}

	// 如果文件存在，加载历史记录
	if filePath != "" {
		mgr.load()
	}

	return mgr
}

// Record 记录告警状态变化
//
// 参数：
//   - taskID: 任务ID
//   - oldState: 旧状态
//   - newState: 新状态
//   - result: 告警结果
//   - severity: 告警级别
//   - labels: 标签
//   - annotations: 描述
func (m *AlertHistoryManager) Record(
	taskID string,
	oldState, newState AlertState,
	result AlarmResult,
	severity Severity,
	labels, annotations map[string]string,
) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 序列化 labels 和 annotations（修复：处理错误）
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		fmt.Printf("[WARN] 序列化 labels 失败: %v\n", err)
		labelsJSON = []byte("{}")
	}
	annotationsJSON, err := json.Marshal(annotations)
	if err != nil {
		fmt.Printf("[WARN] 序列化 annotations 失败: %v\n", err)
		annotationsJSON = []byte("{}")
	}

	record := AlertHistory{
		TaskID:      taskID,
		State:       newState,
		OldState:    oldState,
		Timestamp:   time.Now(),
		Value:       result.Value,
		Threshold:   result.Threshold,
		IsFiring:    result.IsFiring,
		Severity:    severity,
		Labels:      string(labelsJSON),
		Annotations: string(annotationsJSON),
	}

	// 修复：检查最大限制，防止无界内存增长
	if m.maxSize > 0 && len(m.history) >= m.maxSize {
		// 移除最旧的记录
		m.history = m.history[1:]
	}

	m.history = append(m.history, record)

	// 超过阈值时自动持久化
	if len(m.history) >= AutoPersistThreshold {
		m.persist()
	}
}

// GetHistory 获取告警历史
//
// 参数：
//   - taskID: 任务ID（空表示所有）
//   - limit: 返回数量（0表示所有）
//
// 返回值：
//   - []AlertHistory: 历史记录
func (m *AlertHistoryManager) GetHistory(taskID string, limit int) []AlertHistory {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []AlertHistory
	for _, h := range m.history {
		if taskID == "" || h.TaskID == taskID {
			result = append(result, h)
		}
	}

	if limit > 0 && len(result) > limit {
		return result[len(result)-limit:]
	}

	return result
}

// persist 持久化到文件
func (m *AlertHistoryManager) persist() {
	if m.filePath == "" {
		return
	}

	m.persisted = true

	// 清理过期记录
	cutoff := time.Now().AddDate(0, 0, -m.retentionDays)
	var valid []AlertHistory
	for _, h := range m.history {
		if h.Timestamp.After(cutoff) {
			valid = append(valid, h)
		}
	}
	m.history = valid

	// 写入文件
	data, err := json.MarshalIndent(m.history, "", "  ")
	if err != nil {
		fmt.Printf("[ERROR] 序列化告警历史失败: %v\n", err)
		return
	}

	err = os.WriteFile(m.filePath, data, 0644)
	if err != nil {
		fmt.Printf("[ERROR] 写入告警历史文件失败: %v\n", err)
	}
}

// load 从文件加载
func (m *AlertHistoryManager) load() {
	if m.filePath == "" {
		return
	}

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Printf("[ERROR] 读取告警历史文件失败: %v\n", err)
		}
		return
	}

	err = json.Unmarshal(data, &m.history)
	if err != nil {
		fmt.Printf("[ERROR] 解析告警历史文件失败: %v\n", err)
		m.history = make([]AlertHistory, 0)
	}

	m.persisted = true
}

// Close 关闭并持久化
func (m *AlertHistoryManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.persist()
}

// Count 获取历史记录数量
func (m *AlertHistoryManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.history)
}
