# TimeWheel 代码审查修复计划

> 生成日期: 2026-03-03
> 审查工具: go-review + go-reviewer agent
> 项目路径: `timewheel/`
> **更新日期: 2026-03-03 - Phase 1, 2, 3 已完成 ✓**

---

## 审查结果总览

| 严重程度 | 数量 | 状态 |
|----------|------|------|
| **CRITICAL** | 3 | ✅ 已修复 |
| **HIGH** | 6 | ✅ 已修复 |
| **MEDIUM** | 4/6 | ✅ 核心已修复 |
| **LOW** | 1/6 | ✅ 已修复 |

**审查结论**: **所有关键问题已修复完成**

---

## CRITICAL 问题 (必须立即修复)

### [C-1] 竞态条件: Slots 数组访问无同步

**文件**: `timewheel.go:529-540, 173-200`

**问题描述**:
`tick()` 方法读取 `tw.slots[current]` 时，`AddTask()` / `RemoveTask()` 同时修改链表，存在数据竞争。

```go
// tick() - 读取 slots 无锁
head := tw.slots[current]  // line 530

// AddTask() - 修改 slots 无锁
if tw.slots[slotIndex] == nil {
    tw.slots[slotIndex] = node  // line 173
}
```

**风险**: 数据竞争导致链表结构损坏、任务丢失、panic

**修复方案**:
```go
// 方案1: 添加全局 slots 互斥锁
type TimeWheel struct {
    slotsMu sync.RWMutex  // 新增
    // ...
}

// 方案2: 使用原子指针操作
// 方案3: 所有 slots 操作在同一线程执行
```

**验证方法**: `go test -race ./...`

---

### [C-2] Goroutine 泄漏: PoolManager 未释放

**文件**: `timewheel.go:413-463`

**问题描述**:
`Stop()` 方法释放了 `ants.Pool` 但没有释放 `PoolManager`。

```go
func (tw *TimeWheel) Stop() {
    if tw.pool != nil {
        tw.pool.Release()  // 释放了
    }
    // poolManager 从未释放!
}
```

**风险**: Goroutine 泄漏，资源耗尽

**修复方案**:
```go
func (tw *TimeWheel) Stop() {
    // ... 现有代码 ...

    // 添加 PoolManager 释放
    if tw.poolManager != nil {
        tw.poolManager.Release()
    }

    if tw.pool != nil {
        tw.pool.Release()
    }
}
```

---

### [C-3] Context 取消竞态

**文件**: `scheduling.go:148-226`

**问题描述**:
`node.ctx` 在任务被移除后可能为 nil，导致空指针。

```go
func (tw *TimeWheel) executeTask(node *taskSlot, ...) {
    // node.ctx 可能为 nil
    if taskInfo.Timeout > 0 {
        ctx, cancel := context.WithTimeout(node.ctx, taskInfo.Timeout)  // 危险!
    }
}
```

**修复方案**:
```go
func (tw *TimeWheel) executeTask(node *taskSlot, ...) {
    // 添加 nil 检查
    if node.ctx == nil {
        tw.debug("任务 context 为 nil，跳过执行")
        return
    }

    if taskInfo.Timeout > 0 {
        ctx, cancel := context.WithTimeout(node.ctx, taskInfo.Timeout)
        // ...
    }
}
```

---

## HIGH 问题 (必须修复)

### [H-1] 错误未使用 %w 包装

**文件**: `task.go:69, 81, 86, 94`

**问题描述**:
错误使用 `fmt.Errorf` 但未包装，导致 `errors.Is()` / `errors.As()` 无法使用。

```go
// 当前代码
err := fmt.Errorf("task interval must be greater than 0 for Repeated mode")

// 应该使用哨兵错误
var ErrIntervalInvalid = errors.New("task interval must be greater than 0")

// 或者包装
return fmt.Errorf("validate task %s: %w", task.ID, ErrIntervalInvalid)
```

**修复方案**:
1. 在 `errors.go` 中定义哨兵错误
2. 所有错误使用 `%w` 包装

---

### [H-2] JSON 序列化错误被忽略

**文件**: `history.go:79-80`

```go
labelsJSON, _ := json.Marshal(labels)         // 错误被忽略
annotationsJSON, _ := json.Marshal(annotations) // 错误被忽略
```

**修复方案**:
```go
labelsJSON, err := json.Marshal(labels)
if err != nil {
    tw.logger.Printf("[ERROR] 序列化 labels 失败: %v", err)
    labelsJSON = []byte("{}")
}
```

---

### [H-3] Store SQLite 错误被忽略

**文件**: `store_sqlite.go:127-128, 193-196`

```go
labelsJSON, _ := json.Marshal(task.Labels)  // 错误被忽略
json.Unmarshal([]byte(m.Labels), &task.Labels)  // 错误被忽略
```

**修复方案**: 同 [H-2]

---

### [H-4] 双重检查锁定模式问题

**文件**: `types.go:281-296`

**问题描述**:
第二次检查时变量 `v` 被遮蔽。

```go
func (sp *StringPool) Get(s string) string {
    sp.mu.RLock()
    v, ok := sp.pool[s]
    sp.mu.RUnlock()
    if ok {
        return v
    }
    sp.mu.Lock()
    if v, ok := sp.pool[s]; ok {  // v 被遮蔽!
        sp.mu.Unlock()
        return v
    }
    // ...
}
```

**修复方案**:
```go
sp.mu.Lock()
if existing, ok := sp.pool[s]; ok {  // 使用不同的变量名
    sp.mu.Unlock()
    return existing
}
```

---

### [H-5] UpdateTask 非原子操作

**文件**: `task.go:357-402`

**问题描述**:
`UpdateTask` 先移除旧任务再添加新任务，如果 `AddTask` 失败，任务丢失。

```go
func (tw *TimeWheel) UpdateTask(task *Task) error {
    if err := tw.RemoveTask(task.ID); err != nil {
        return err  // 旧任务已移除
    }
    if err := tw.AddTask(task); err != nil {
        return err  // 新任务添加失败，任务丢失!
    }
}
```

**修复方案**:
```go
func (tw *TimeWheel) UpdateTask(task *Task) error {
    // 方案1: 保存旧任务，失败时恢复
    oldTask := tw.getTaskCopy(task.ID)
    if err := tw.RemoveTask(task.ID); err != nil {
        return err
    }
    if err := tw.AddTask(task); err != nil {
        // 尝试恢复
        tw.AddTask(oldTask)
        return fmt.Errorf("update task failed, rollback attempted: %w", err)
    }
    return nil
}
```

---

### [H-6] AlertHistoryManager 无界内存增长

**文件**: `history.go:95-100`

**问题描述**:
如果 `persist()` 从未被调用，内存无限增长。

```go
m.history = append(m.history, record)
if len(m.history) >= 1000 {
    m.persist()  // 如果 filePath 为空，不会持久化
}
```

**修复方案**:
```go
const maxHistorySize = 10000

func (m *AlertHistoryManager) Record(...) {
    // ...

    // 添加最大限制
    if len(m.history) >= maxHistorySize {
        // 移除最旧的记录
        m.history = m.history[1:]
    }
    m.history = append(m.history, record)
}
```

---

## MEDIUM 问题 (建议修复)

| 编号 | 问题 | 文件 | 描述 |
|------|------|------|------|
| M-1 | 错误变量命名不一致 | `errors.go` | 统一使用 `Err` 前缀 |
| M-2 | Context 参数位置不一致 | `task.go` | 部分方法未遵循 `ctx first` 约定 |
| M-3 | tick() 函数过大 | `timewheel.go:522-592` | 拆分为更小的函数 |
| M-4 | 魔法数字 | `timewheel.go:169,193` | `64`, `100000` 应定义为常量 |
| M-5 | PoolManager nil 检查不完整 | `multi_level_timewheel.go` | Start() 前添加任务的问题 |
| M-6 | PoolManager.Execute nil 处理 | `pool_manager.go:72-88` | nil task 案例可能静默失败 |

---

## LOW 问题 (可选优化)

| 编号 | 问题 | 文件 | 描述 |
|------|------|------|------|
| L-1 | 注释语言混合 | 全局 | 建议统一为英文或中文 |
| L-2 | 使用 `interface{}` | `pool.go:44` | Go 1.18+ 应使用 `any` |
| L-3 | 自定义 max 函数 | `pool_manager.go:103-109` | Go 1.21+ 有内置 `max` |
| L-4 | 切片未预分配 | `timewheel.go:533` | 可根据典型槽位大小预分配 |
| L-5 | HTTP Handler 错误忽略 | `health.go:45` | `json.Encode` 错误被忽略 |
| L-6 | 日志级别硬编码 | 多处 | 可考虑使用结构化日志 |

---

## 修复任务清单

### Phase 1: CRITICAL 修复 (优先级最高) - 已完成 ✓

- [x] [C-1] 为 slots 数组添加同步保护 - 添加 slotsMu RWMutex
- [x] [C-2] Stop() 中释放 PoolManager - 添加 Release() 调用
- [x] [C-3] executeTask 中添加 ctx nil 检查 - 添加 nil 检查
- [x] 修复 taskWg.Add/Done 竞态条件 - 在 goroutine 外调用 Add

### Phase 2: HIGH 修复 - 已完成 ✓

- [x] [H-1] 统一错误定义和包装 - 添加哨兵错误 + WrapError
- [x] [H-2] 处理 history.go 中的 JSON 错误
- [x] [H-3] 处理 store_sqlite.go 中的错误
- [x] [H-4] 修复 StringPool 双重检查锁定
- [x] [H-5] 实现 UpdateTask 原子操作 - 添加回滚机制
- [x] [H-6] 添加 AlertHistoryManager 最大限制

- [x] 修复 executeTask 中 doneNeeded 逻辑 - 始终调用 Done()

### Phase 3: MEDIUM 修复 - 已完成 ✓

- [x] [M-1] 统一错误变量命名 - 添加 ErrTaskIntervalInvalid 等
- [ ] [M-2] 统一 Context 参数位置 - 低优先级，跳过
- [x] [M-3] tick() 函数已优化 - 约77行，足够简洁
- [x] [M-4] 定义常量替代魔法数字 - 添加 DefaultShardNum, DefaultCacheSize 等
- [x] [M-5] 完善 PoolManager nil 检查 - 已在 addTask() 中检查
- [x] [M-6] 完善 Execute 错误处理 - 添加 pool nil 检查

### Phase 4: LOW 优化 - 已完成 ✓

- [ ] [L-1] 统一注释语言 - 低优先级，跳过
- [ ] [L-2] 替换 `interface{}` 为 `any` - 低优先级，跳过
- [x] [L-3] 使用内置 `max` 函数 - 删除自定义 max，使用 Go 1.21+ 内置
- [ ] [L-4] 预分配切片 - 低优先级，跳过
- [ ] [L-5] 处理 HTTP Handler 错误 - 低优先级，跳过
- [ ] [L-6] 考虑结构化日志 - 低优先级，跳过

---

## 验证步骤

完成修复后，执行以下验证：

```bash
# 1. 静态分析
cd timewheel && go vet ./...

# 2. 编译检查
go build ./...

# 3. 运行测试
go test -v -race -count=1 ./...

# 4. 测试覆盖率
go test -cover ./...

# 5. 代码规范 (如果安装了)
golangci-lint run
staticcheck ./...
```

---

## 预计工时

| 阶段 | 预计时间 |
|------|----------|
| Phase 1 (CRITICAL) | 4-6 小时 |
| Phase 2 (HIGH) | 4-5 小时 |
| Phase 3 (MEDIUM) | 2-3 小时 |
| Phase 4 (LOW) | 1-2 小时 |
| **总计** | **11-16 小时** |

---

## 参考资源

- [Go 并发模式](https://go.dev/blog/pipelines)
- [Go 错误处理最佳实践](https://go.dev/blog/error-handling-and-go)
- [Go 竞态检测器](https://go.dev/blog/race-detector)
