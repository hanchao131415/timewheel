# TimeWheel 测试修复设计文档

> **目标**: 修复 7 个失败测试，确保 100% 测试通过

**日期**: 2026-03-02
**状态**: 待实施

---

## 问题概述

当前测试结果：2 通过，7 失败

| 测试名称 | 状态 | 根因分析 |
|----------|------|----------|
| TestMultiLevelTimeWheel_PriorityBoundary | FAIL | 任务未执行 |
| TestMultiLevelTimeWheel_IntervalBoundary | FAIL | 参数校验/边界问题 |
| TestMultiLevelTimeWheel_TaskTimeout | FAIL | 任务未执行 |
| TestMultiLevelTimeWheel_HighConcurrency | FAIL | 任务未执行 |
| TestMultiLevelTimeWheel_DynamicPriority | FAIL | 任务未执行 |
| TestMultiLevelTimeWheel_MixedPriority | FAIL | 任务未执行 |
| TestMultiLevelTimeWheel_InvalidTask | FAIL | 参数校验问题 |

**核心问题**: `MultiLevelTimeWheel` 的任务执行次数为 0，任务未被正确触发。

---

## 修复方案

采用**根因修复 + 回归验证**策略，按问题类型分组修复。

---

## Phase 1: 核心问题修复

### 目标
修复 `MultiLevelTimeWheel` 任务执行逻辑

### 修改文件
- `timewheel/pkg/timewheel/multi_level_timewheel.go`

### 修复点

#### 1.1 任务分发
- 确保 `AddTask()` 正确路由到对应优先级的时间轮
- 检查优先级映射逻辑

#### 1.2 启动顺序
- 确保 `Start()` 正确启动所有时间轮
- 确保每个时间轮都设置了 `poolManager`

#### 1.3 调试日志
- 添加关键路径的调试日志
- 确认任务被正确添加和触发

---

## Phase 2: 边界问题修复

### 目标
修复参数校验和边界处理

### 修改文件
- `timewheel/pkg/timewheel/multi_level_timewheel.go`
- `timewheel/pkg/timewheel/multi_level_timewheel_extended_test.go`

### 修复点

#### 2.1 参数校验
在 `AddTask()` 中添加：
- nil 检查
- 必填字段校验（ID, Run）
- 间隔有效性校验

#### 2.2 边界处理
- 处理 0 或负数间隔
- 处理超范围优先级

#### 2.3 测试调整
- 确保测试期望与实际行为一致
- 适当延长等待时间确保任务执行

---

## Phase 3: 验证通过

### 验证命令

```bash
# Step 1: 运行失败测试
go test -v -run "PriorityBoundary|IntervalBoundary|TaskTimeout|HighConcurrency|DynamicPriority|MixedPriority|InvalidTask" ./...

# Step 2: 运行全量测试
go test -v -count=1 ./...

# Step 3: 确认结果
# 期望：PASS (100%)
```

### 回归检查

| 检查项 | 要求 |
|--------|------|
| 已通过测试 | `TestNew_InvalidParams`, `TestMultiLevelTimeWheel_ConcurrentAdd` 等仍通过 |
| 代码质量 | 无新增 lint 警告 |
| 性能影响 | 修复不应降低性能 |

---

## 风险评估

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 修复引入新 bug | 中 | 每次修复后运行全量测试 |
| 测试本身有问题 | 低 | 分析测试逻辑，必要时调整 |
| 性能下降 | 低 | 修复仅涉及逻辑，不改变算法 |

---

## 预计工作量

| Phase | 预计时间 |
|-------|----------|
| Phase 1 | 30 分钟 |
| Phase 2 | 20 分钟 |
| Phase 3 | 10 分钟 |
| **总计** | **60 分钟** |

---

## 成功标准

- [ ] 所有 7 个失败测试变为 PASS
- [ ] 原有 2 个通过测试保持 PASS
- [ ] 无新增 lint 警告
- [ ] 代码可正常编译运行
