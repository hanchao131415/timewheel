# TimeWheel 问题追踪

本文档记录 TimeWheel 项目的已知问题和待修复项。

---

## 问题状态说明

- 🔴 **严重** - 影响功能或性能，需立即修复
- 🟡 **中等** - 影响体验，短期修复
- 🟢 **轻微** - 小问题，可延后处理

---

## 性能问题

| ID | 问题 | 位置 | 严重程度 | 状态 |
|----|------|------|----------|------|
| P1 | tick() 中重复加锁 | `timewheel.go` | 🔴 | ✅ 已修复 |
| P2 | 任务执行排序开销 O(n log n) | `timewheel.go:1602-1612` | 🔴 | open |
| P3 | history.go 同步IO阻塞 | `history.go` | 🔴 | ✅ 已修复 |
| P4 | history 切片内存无限增长 | `history.go:95` | 🟡 | open |
| P5 | 缓存穿透风险 | `timewheel.go:462-477` | 🟡 | open |
| P6 | StringPool 无容量限制 | `timewheel.go:309-335` | 🟡 | open |
| P7 | time.Now() 过度调用 | 多处 | 🟢 | open |

---

## 产品功能问题

| ID | 问题 | 严重程度 | 状态 |
|----|------|----------|------|
| D1 | 缺少 Prometheus Metrics 导出 | 🔴 | open |
| D2 | 缺少分布式支持 | 🔴 | open |
| D3 | 任务持久化缺失 | 🔴 | ✅ 已修复 |
| D4 | 缺少任务依赖/DAG | 🟡 | open |
| D5 | 优先级设计不灵活（只有3级） | 🟡 | open |
| D6 | 缺少任务去重机制 | 🟡 | open |
| D7 | 缺少流量控制/限流 | 🟡 | open |
| D8 | 缺少健康检查接口 | 🟢 | ✅ 已修复 |

---

## 工程质量问题

| ID | 问题 | 位置 | 严重程度 | 状态 |
|----|------|------|----------|------|
| E1 | 测试失败 | `multi_level_timewheel_extended_test.go` | 🔴 | ✅ 已修复 |
| E2 | 单文件过大（2177行） | `timewheel.go` | 🔴 | ✅ 已修复 |
| E3 | 缺少接口抽象 | 多处 | 🔴 | ✅ 已修复 |
| E4 | 错误处理不一致 | 多处 | 🟡 | open |
| E5 | 缺少 context 传递 | `pool_manager.go` | 🟡 | open |
| E6 | 日志格式不统一 | 多处 | 🟡 | open |
| E7 | 缺少英文注释 | 多处 | 🟢 | open |
| E8 | 魔法数字 | 多处 | 🟢 | open |

---

## 架构设计问题

| ID | 问题 | 严重程度 | 状态 |
|----|------|----------|------|
| A1 | 单层时间轮精度限制 | 🟡 | 已有 MultiLevelTimeWheel |
| A2 | 告警与调度耦合 | 🟡 | open |
| A3 | 配置不可热更新 | 🟡 | open |
| A4 | 缺少插件机制 | 🟢 | open |

---

## 修复优先级

### P0 - 立即修复（本周）

```
[ ] P1 - 修复双重锁问题
    └─ 合并 handleAlertState 和 handleTaskScheduling 的锁操作

[ ] E1 - 修复测试失败
    └─ 检查 multi_level_timewheel_extended_test.go:71

[ ] P3 - 异步持久化
    └─ history.go 使用 channel 异步写入
```

### P1 - 短期修复（1-2周）

```
[ ] D1 - Prometheus Metrics
    └─ 添加 /metrics 端点

[ ] D3 - 任务持久化
    └─ 实现 TaskStore 接口

[ ] E2 - 拆分大文件
    └─ timewheel.go → 多文件

[ ] E3 - 接口抽象
    └─ GoroutinePool/Logger/Store 接口
```

### P2 - 中期规划（1-2月）

```
[ ] D2 - 分布式支持
    └─ etcd/Redis 协调器

[ ] D4 - 任务依赖/DAG
    └─ DAGScheduler 实现

[ ] P2 - 优化排序算法
    └─ 使用跳表或分桶

[ ] A2 - 告警模块解耦
    └─ 独立 AlertService
```

---

## 问题详情

### P1: tick() 中重复加锁

**影响**：高并发下性能下降 30-50%

**复现步骤**：
1. 创建时间轮并启动
2. 添加 10000 个任务
3. 观察锁竞争指标

**修复方案**：
```go
// 在 executeTask 中一次性获取锁
shard := tw.getShard(taskInfo.ID)
shard.mu.Lock()
defer shard.mu.Unlock()

tw.handleAlertStateLocked(...)
tw.handleTaskSchedulingLocked(...)
```

---

### P3: history.go 同步IO阻塞

**影响**：告警记录延迟，可能阻塞任务执行

**复现步骤**：
1. 启用告警历史持久化
2. 触发大量告警（>1000条）
3. 观察任务执行延迟

**修复方案**：
```go
type AlertHistoryManager struct {
    recordCh chan AlertHistory  // 异步通道
}

func (m *AlertHistoryManager) runWriter() {
    for record := range m.recordCh {
        // 异步处理
    }
}
```

---

### E1: 测试失败

**位置**：`multi_level_timewheel_extended_test.go:71`

**错误信息**：
```
multi_level_timewheel_extended_test.go:71: 任务执行次数不足: 期望>=2, 实际=0
```

**可能原因**：
1. 任务添加后立即停止，未等待执行
2. 时间轮启动时序问题

**修复方案**：
```go
// 增加等待时间
time.Sleep(200 * time.Millisecond)
// 或使用同步机制
```

---

### E2: 单文件过大

**当前状态**：`timewheel.go` 2177 行

**目标**：拆分为多个小文件，每个 < 500 行

**拆分计划**：
```
timewheel.go (2177行) →
├── timewheel.go    (~500行) 核心结构
├── options.go      (~200行) 配置选项
├── task.go         (~300行) 任务操作
├── alert.go        (~300行) 告警逻辑
├── metrics.go      (~200行) 监控指标
├── cache.go        (~200行) 缓存
├── pool.go         (~100行) 协程池
├── errors.go       (~50行)  错误定义
└── interfaces.go   (~100行) 接口
```

---

## 已解决问题

### 2026-03-02 (v1.1.0)

| ID | 问题 | 修复方案 |
|----|------|----------|
| P1 | 双重锁问题 | 合并 handleAlertStateLocked 和 handleTaskSchedulingLocked，一次性获取锁 |
| P3 | 同步IO阻塞 | 使用 channel 异步写入历史记录 |
| D3 | 任务持久化缺失 | 实现 SQLite/MySQL 存储，支持自动恢复 |
| D8 | 缺少健康检查接口 | 添加 /health 和 /ready 端点 |
| E1 | 测试失败 | 修复测试等待时间和同步机制 |
| E2 | 单文件过大 | 拆分为 errors.go, types.go, options.go, pool.go, task.go, alert.go, scheduling.go, health.go |
| E3 | 缺少接口抽象 | 定义 GoroutinePool, TaskStore, HistoryStore 接口 |

---

## 更新记录

| 日期 | 更新内容 |
|------|----------|
| 2026-03-02 | v1.1.0 发布：修复双重锁、添加 SQLite 持久化、文件拆分、健康检查 |
| 2026-03-02 | 创建问题追踪文档 |
