# 任务优先级设计文档

## 1. 项目背景

时间轮是一个用于调度任务的系统，目前已实现基本的任务调度功能。为了满足不同类型任务的优先级需求，特别是告警系统中不同级别的告警处理，需要实现任务优先级功能。

## 2. 需求分析

### 2.1 优先级级别
- 3级优先级：高、中、低
- 高优先级：如火警等紧急告警
- 普通优先级：如一般告警
- 低优先级：如温湿度检测等非紧急告警

### 2.2 优先级影响
- 槽位内优先级：同一槽位内的任务按优先级排序
- 不同槽位按时间顺序：不同槽位的任务仍按时间顺序执行

### 2.3 实时性要求
- 软实时：高优先级任务优先执行，但允许一定延迟

## 3. 设计方案

### 3.1 方案选择
选择**方案二：多层时间轮**，为不同优先级的任务创建独立的时间轮。

### 3.2 架构设计

#### 3.2.1 核心组件
- **MultiLevelTimeWheel**：多层时间轮管理器，负责管理不同优先级的时间轮
- **TimeWheel**：单个时间轮实现，保持现有的时间轮功能
- **PoolManager**：协程池管理器，为不同优先级的任务提供不同的协程池

#### 3.2.2 优先级级别
- **TaskPriorityHigh**：高优先级（如火警等紧急告警）
- **TaskPriorityNormal**：普通优先级（如一般告警）
- **TaskPriorityLow**：低优先级（如温湿度检测等非紧急告警）

#### 3.2.3 时间轮配置

| 优先级 | 时间间隔 | 槽位数量 | 协程池大小 |
|--------|---------|---------|-----------|
| 高     | 10ms    | 60      | CPU核心数×3 |
| 普通   | 100ms   | 60      | CPU核心数   |
| 低     | 1s      | 60      | CPU核心数/2 |

### 3.3 数据流程

1. **任务添加**：
   - 客户端调用 `AddTask()` 方法添加任务
   - `MultiLevelTimeWheel` 根据任务优先级将其分配到相应的时间轮
   - 各时间轮独立管理自己的任务

2. **任务执行**：
   - 各时间轮独立运行，按照自己的时间间隔转动
   - 高优先级时间轮转动频率更高，确保紧急任务更快执行
   - 任务执行时使用对应优先级的协程池

3. **任务调度**：
   - 高优先级任务在高优先级时间轮中执行，响应更快
   - 低优先级任务在低优先级时间轮中执行，节省系统资源

### 3.4 关键类设计

#### 3.4.1 MultiLevelTimeWheel

```go
// MultiLevelTimeWheel 多层时间轮管理器
type MultiLevelTimeWheel struct {
    highPriorityTW   *TimeWheel // 高优先级时间轮
    normalPriorityTW *TimeWheel // 普通优先级时间轮
    lowPriorityTW    *TimeWheel // 低优先级时间轮
    poolManager      *PoolManager // 协程池管理器
    mu               sync.RWMutex // 读写锁
}

// NewMultiLevelTimeWheel 创建多层时间轮
func NewMultiLevelTimeWheel() (*MultiLevelTimeWheel, error) {
    // 初始化各优先级时间轮
    // 初始化协程池管理器
    // 返回多层时间轮实例
}

// AddTask 添加任务
func (mltw *MultiLevelTimeWheel) AddTask(task *Task) error {
    // 根据任务优先级将其分配到相应的时间轮
}

// Start 启动所有时间轮
func (mltw *MultiLevelTimeWheel) Start() error {
    // 启动各优先级时间轮
}

// Stop 停止所有时间轮
func (mltw *MultiLevelTimeWheel) Stop() {
    // 停止各优先级时间轮
    // 释放协程池资源
}
```

#### 3.4.2 PoolManager

```go
// PoolManager 协程池管理器
type PoolManager struct {
    highPriorityPool   *ants.Pool
    normalPriorityPool *ants.Pool
    lowPriorityPool    *ants.Pool
}

// NewPoolManager 创建协程池管理器
func NewPoolManager() (*PoolManager, error) {
    // 初始化各优先级协程池
}

// Execute 执行任务
func (pm *PoolManager) Execute(task *Task, f func()) error {
    // 根据任务优先级选择相应的协程池执行任务
}
```

## 4. 实现计划

### 4.1 实现步骤

1. **修改Task结构体**：添加Priority字段
2. **实现PoolManager**：为不同优先级任务提供不同的协程池
3. **实现MultiLevelTimeWheel**：管理不同优先级的时间轮
4. **修改现有代码**：确保与现有系统的兼容性
5. **编写测试**：测试任务优先级功能
6. **代码review**：检查代码质量和性能

### 4.2 时间估计

| 任务 | 时间估计 |
|------|----------|
| 修改Task结构体 | 0.5小时 |
| 实现PoolManager | 1小时 |
| 实现MultiLevelTimeWheel | 2小时 |
| 修改现有代码 | 1小时 |
| 编写测试 | 1.5小时 |
| 代码review | 1小时 |
| 总计 | 7小时 |

## 5. 测试策略

### 5.1 单元测试
- 测试各时间轮的基本功能
- 测试协程池管理器的功能
- 测试多层时间轮的基本功能

### 5.2 集成测试
- 测试多层时间轮的协同工作
- 测试不同优先级任务的执行顺序
- 测试系统在高负载下的表现

### 5.3 性能测试
- 测试不同优先级任务的响应时间
- 测试系统的吞吐量
- 测试系统的资源使用情况

## 6. 部署与监控

### 6.1 部署
- 作为独立组件部署
- 集成到现有系统中

### 6.2 监控
- 监控各时间轮的任务执行情况
- 监控协程池的使用情况
- 设置系统级告警，监控时间轮的健康状态

## 7. 风险评估

### 7.1 潜在风险
- **资源消耗增加**：多个时间轮和协程池会增加系统资源消耗
- **实现复杂度增加**：需要管理多个组件的生命周期
- **配置复杂度增加**：需要合理配置各时间轮的参数

### 7.2 风险缓解
- **资源管理**：合理配置协程池大小，避免资源浪费
- **错误处理**：添加完善的错误处理机制
- **监控告警**：设置系统级告警，及时发现和处理问题

## 8. 结论

多层时间轮方案虽然实现复杂度较高，但能够提供更严格的优先级保证和更合理的资源分配，适合对实时性要求较高的告警系统。通过合理配置各优先级时间轮的参数，可以在保证高优先级任务响应速度的同时，有效利用系统资源。

## 9. 附录

### 9.1 参考资料
- 时间轮调度算法
- ants协程池库文档
- Go并发编程最佳实践

### 9.2 相关代码
- `timewheel.go`：时间轮核心实现
- `pool_manager.go`：协程池管理器实现
- `multi_level_timewheel.go`：多层时间轮实现
- `timewheel_test.go`：测试代码