# 告警服务功能实现计划

## 概述

为 timewheel 添加 Prometheus 风格的告警服务特性，包括：告警级别、持续时间(for)、重复告警间隔、告警历史持久化。

## 现有代码问题

1. 任务无返回值，无法获取告警评估结果
2. 缺少告警状态管理（FIRING/RESOLVED/PENDING）
3. 缺少告警级别（Critical/Warning/Info）
4. 缺少持续时间（for）支持
5. 缺少重复告警间隔
6. 缺少告警历史和持久化

---

## 实现计划

### 阶段 1: 类型定义（新增类型）

#### 1.1 添加告警状态和级别类型

**文件**: `timewheel.go`

**新增内容**:
- `AlertState` 类型：Pending / Firing / Resolved
- `Severity` 类型：Critical / Warning / Info
- `AlarmResult` 结构体：Value / Threshold / IsFiring
- `AlertHistory` 结构体：历史记录结构

**验证**: 运行 `go build` 确认无编译错误

---

#### 1.2 扩展 Task 结构体

**文件**: `timewheel.go`

**新增字段**:
- `Severity` - 告警级别
- `For` - 持续时间
- `RepeatInterval` - 重复告警间隔
- `Labels` - 标签
- `Annotations` - 描述

**验证**: 运行 `go build` 确认无编译错误

---

### 阶段 2: 告警状态管理

#### 2.1 添加内部状态跟踪字段

**文件**: `taskSlot` 结构体

**新增字段**:
- `alertState` - 当前告警状态
- `pendingSince` - 进入pending的时间
- `lastFiredAt` - 上次触发时间
- `lastResult` - 上次评估结果

**验证**: 运行 `go build`

---

#### 2.2 修改 Task.Run 签名为返回 AlarmResult

**文件**: `timewheel.go`

**修改**:
```go
// 修改前
Run func(ctx context.Context) error

// 修改后
Run func(ctx context.Context) AlarmResult
```

**验证**: 更新现有测试用例，确保编译通过

---

### 阶段 3: 告警状态机逻辑

#### 3.1 实现状态转换逻辑

**文件**: `executeTask` 方法

**逻辑**:
- Pending: 条件满足但未达到 For 持续时间
- Firing: 条件满足且达到 For 持续时间
- Resolved: 从 Firing 转为不满足条件

**验证**: 编写单元测试验证状态转换

---

#### 3.2 实现重复告警逻辑

**文件**: `executeTask` 方法

**逻辑**:
- 当状态为 Firing 时，检查是否达到 RepeatInterval
- 如果达到，触发重复告警

**验证**: 编写单元测试验证重复告警

---

### 阶段 4: 告警历史持久化

#### 4.1 创建告警历史管理器

**文件**: 新增 `history.go`

**功能**:
- `AlertHistoryManager` 结构体
- 内存存储 + 定期持久化
- JSON 格式文件存储
- 30 天自动清理

**验证**: 运行单元测试

---

#### 4.2 集成历史管理器到 TimeWheel

**文件**: `timewheel.go`

**新增**:
- `HistoryManager` 字段
- `WithHistoryFile()` 选项
- 在状态转换时记录历史

**验证**: 运行 `go build` 和现有测试

---

### 阶段 5: 配置选项

#### 5.1 添加新的配置选项

**文件**: `timewheel.go`

**新增选项**:
- `WithSeverity()` - 设置默认告警级别
- `WithFor()` - 设置默认持续时间
- `WithRepeatInterval()` - 设置默认重复间隔
- `WithHistoryFile()` - 设置历史文件路径
- `WithHistoryRetention()` - 设置历史保留天数

**验证**: 运行 `go build`

---

### 阶段 6: 测试

#### 6.1 更新现有测试

**文件**: `timewheel_test.go`

**修改**:
- 更新 Task 签名
- 添加新特性测试

**验证**: 运行 `go test`

---

#### 6.2 新增告警功能测试

**文件**: `timewheel_alarm_test.go` (新文件)

**测试内容**:
- 告警状态转换
- For 持续时间
- 重复告警间隔
- 历史记录持久化

**验证**: 运行 `go test -v`

---

## 任务清单

| # | 任务 | 文件 | 验证方式 |
|---|------|------|----------|
| 1 | 添加告警状态和级别类型 | timewheel.go | go build |
| 2 | 扩展 Task 结构体字段 | timewheel.go | go build |
| 3 | 添加内部状态跟踪字段 | timewheel.go | go build |
| 4 | 修改 Task.Run 签名 | timewheel.go | go build |
| 5 | 实现告警状态转换逻辑 | timewheel.go | go test |
| 6 | 实现重复告警逻辑 | timewheel.go | go test |
| 7 | 创建告警历史管理器 | history.go | go test |
| 8 | 集成历史管理器 | timewheel.go | go build |
| 9 | 添加新的配置选项 | timewheel.go | go build |
| 10 | 更新现有测试 | *_test.go | go test |
| 11 | 新增告警功能测试 | timewheel_alarm_test.go | go test -v |

---

## 预计时间

每个任务约 2-5 分钟，总计约 30-60 分钟

---

## 风险和依赖

- Task.Run 签名变更会影响现有用户代码
- 历史文件并发写入需要考虑线程安全
