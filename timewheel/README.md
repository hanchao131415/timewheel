# TimeWheel - 高性能时间轮定时任务调度系统

[![Go Version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

一个生产级的 Go 语言时间轮实现，支持高并发、任务优先级、Prometheus 风格告警服务。

---

## 目录

- [项目概述](#项目概述)
- [核心特性](#核心特性)
- [架构设计](#架构设计)
- [安装](#安装)
- [快速开始](#快速开始)
- [API 文档](#api-文档)
- [配置选项](#配置选项)
- [任务模式](#任务模式)
- [告警服务](#告警服务)
- [多层时间轮](#多层时间轮)
- [性能指标](#性能指标)
- [最佳实践](#最佳实践)
- [常见问题](#常见问题)

**相关文档**：
- [改进路线图](./ROADMAP.md) - 版本规划和改进计划
- [问题追踪](./ISSUES.md) - 已知问题和待修复项

---

## 项目概述

TimeWheel 是一个基于时间轮算法的高性能定时任务调度系统，专为需要处理大量定时任务的场景设计。采用分片锁、对象池、协程池等技术优化，支持万级并发任务调度。

### 设计目标

| 目标 | 说明 |
|------|------|
| **高并发** | 支持 10,000+ 任务并发调度 |
| **低延迟** | 毫秒级任务触发精度 |
| **高可用** | 优雅停机、panic 恢复、错误回调 |
| **可观测** | 丰富的监控指标和日志 |
| **灵活性** | 多种任务模式、优先级支持 |

### 适用场景

- 定时任务调度
- 延迟队列
- 告警监控系统（Prometheus AlertManager 风格）
- 心跳检测
- 超时控制
- 周期性数据同步

---

## 核心特性

### 任务调度

| 特性 | 描述 |
|------|------|
| **多种执行模式** | Once（单次）、FixedTimes（固定次数）、Repeated（周期重复） |
| **任务优先级** | High（高）、Normal（普通）、Low（低）三级优先级 |
| **动态管理** | 运行时添加、删除、更新、暂停、恢复任务 |
| **批量操作** | 支持批量添加和删除任务 |

### 告警服务

| 特性 | 描述 |
|------|------|
| **状态机** | Pending → Firing → Resolved 状态转换 |
| **持续时间（For）** | 条件需持续满足指定时间才触发告警 |
| **重复告警** | 支持 RepeatInterval 控制重复告警频率 |
| **告警级别** | Critical、Warning、Info 三级告警 |
| **历史持久化** | 告警记录持久化存储，支持自动清理 |

### 性能优化

| 特性 | 描述 |
|------|------|
| **分片锁** | 64 分片减少锁竞争，提升并发性能 |
| **对象池** | sync.Pool 复用任务对象，减少 GC 压力 |
| **协程池** | ants 协程池管理 goroutine，避免无限制创建 |
| **字符串池** | 复用字符串，减少内存分配 |
| **任务缓存** | LRU 缓存加速任务查找 |

### 可观测性

| 特性 | 描述 |
|------|------|
| **丰富指标** | 任务数、执行数、失败数、缓存命中率等 |
| **日志级别** | Debug、Info、Warn、Error 四级日志 |
| **状态监控** | 定时打印时间轮运行状态 |
| **错误回调** | 自定义错误处理函数 |

---

## 架构设计

### 整体架构

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           MultiLevelTimeWheel                           │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                      PoolManager (协程池管理器)                   │   │
│  │   ┌─────────────┐   ┌─────────────┐   ┌─────────────┐          │   │
│  │   │ High Pool   │   │ Normal Pool │   │  Low Pool   │          │   │
│  │   │ (CPU×3)     │   │ (CPU)       │   │ (CPU/2)     │          │   │
│  │   └─────────────┘   └─────────────┘   └─────────────┘          │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐     │
│  │  High Priority   │  │  Normal Priority │  │   Low Priority   │     │
│  │    TimeWheel     │  │    TimeWheel     │  │    TimeWheel     │     │
│  │  ┌────────────┐  │  │  ┌────────────┐  │  │  ┌────────────┐  │     │
│  │  │ 10ms Tick  │  │  │  │ 100ms Tick │  │  │  │  1s Tick   │  │     │
│  │  │ 60 Slots   │  │  │  │ 60 Slots   │  │  │  │ 60 Slots   │  │     │
│  │  └────────────┘  │  │  └────────────┘  │  │  └────────────┘  │     │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘     │
└─────────────────────────────────────────────────────────────────────────┘
```

### 单层时间轮结构

```
┌─────────────────────────────────────────────────────────────────┐
│                        TimeWheel                                │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                    Slots Array (60)                      │   │
│  │  ┌─────┐ ┌─────┐ ┌─────┐         ┌─────┐ ┌─────┐       │   │
│  │  │Slot0│→│Slot1│→│Slot2│ ... ... │Slot58│→│Slot59│      │   │
│  │  └──┬──┘ └──┬──┘ └──┬──┘         └──┬──┘ └──┬──┘       │   │
│  │     ↓       ↓       ↓               ↓       ↓          │   │
│  │   Task    Task    Task            Task    Task         │   │
│  │   List    List    List            List    List         │   │
│  │  (优先级排序的双向链表)                                   │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              Shard Map (64分片锁)                        │   │
│  │  ┌───────┐ ┌───────┐ ┌───────┐     ┌───────┐           │   │
│  │  │Shard 0│ │Shard 1│ │Shard 2│ ... │Shard63│           │   │
│  │  │ task1 │ │ task4 │ │ task7 │     │taskN  │           │   │
│  │  │ task2 │ │ task5 │ │ task8 │     │       │           │   │
│  │  │ task3 │ │ task6 │ │       │     │       │           │   │
│  │  └───────┘ └───────┘ └───────┘     └───────┘           │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                    Task Cache                            │   │
│  │            (LRU缓存，加速任务查找)                         │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### 告警状态机

```
                    ┌─────────────────┐
                    │                 │
                    │    PENDING      │ ← 条件满足但未达到For时间
                    │                 │
                    └────────┬────────┘
                             │
              达到For持续时间  │  条件不再满足
                             │  (重置pendingSince)
                             ↓
                    ┌─────────────────┐
         ┌──────────│                 │──────────┐
         │          │    FIRING       │          │
         │          │                 │          │
         │          └─────────────────┘          │
         │                                       │
  RepeatInterval │                          条件不再满足
         │                                       │
         │                                       ↓
         │                               ┌─────────────────┐
         └───────────────────────────────│                 │
                                         │    RESOLVED     │
                                         │                 │
                                         └─────────────────┘
```

### 核心组件

| 组件 | 文件 | 职责 |
|------|------|------|
| **TimeWheel** | `timewheel.go` | 单层时间轮核心实现 |
| **MultiLevelTimeWheel** | `multi_level_timewheel.go` | 多层时间轮管理器 |
| **PoolManager** | `pool_manager.go` | 协程池管理，按优先级分配资源 |
| **AlertHistoryManager** | `history.go` | 告警历史持久化管理 |

---

## 安装

### 要求

- Go 1.25+
- 依赖：`github.com/panjf2000/ants/v2`、`golang.org/x/sync`

### 安装方式

```bash
# 方式一：直接引用
import "your-module-path/timewheel/pkg/timewheel"

# 方式二：复制到项目
cp -r timewheel/pkg/timewheel your-project/pkg/
```

### 依赖安装

```bash
go mod tidy
```

---

## 快速开始

### 基础示例

```go
package main

import (
    "context"
    "fmt"
    "time"
    
    "timewheel/pkg/timewheel"
)

func main() {
    // 1. 创建时间轮
    tw, err := timewheel.New(
        timewheel.WithSlotNum(60),           // 60个槽位
        timewheel.WithInterval(100*time.Millisecond), // 100ms轮转间隔
        timewheel.WithLogLevel(timewheel.LogLevelInfo), // 日志级别
    )
    if err != nil {
        panic(err)
    }
    
    // 2. 启动时间轮
    if err := tw.Start(); err != nil {
        panic(err)
    }
    defer tw.Stop()
    
    // 3. 添加周期性任务
    err = tw.AddTask(&timewheel.Task{
        ID:          "task-1",
        Mode:        timewheel.TaskModeRepeated,
        Interval:    1 * time.Second,
        Priority:    timewheel.TaskPriorityNormal,
        Description: "每秒执行的测试任务",
        Run: func(ctx context.Context) timewheel.AlarmResult {
            fmt.Println("任务执行:", time.Now().Format("15:04:05"))
            return timewheel.AlarmResult{}
        },
    })
    if err != nil {
        panic(err)
    }
    
    // 4. 等待任务执行
    time.Sleep(10 * time.Second)
}
```

### 告警任务示例

```go
// Prometheus 风格的告警任务
err = tw.AddTask(&timewheel.Task{
    ID:             "cpu-alert",
    Mode:           timewheel.TaskModeRepeated,
    Interval:       10 * time.Second,      // 每10秒检查一次
    Priority:       timewheel.TaskPriorityHigh,
    Severity:       timewheel.SeverityCritical,
    For:            1 * time.Minute,       // 持续1分钟才告警
    RepeatInterval: 5 * time.Minute,       // 每5分钟重复告警
    Labels: map[string]string{
        "alertname": "HighCPUUsage",
        "instance":  "server-1",
    },
    Annotations: map[string]string{
        "summary":     "CPU使用率过高",
        "description": "CPU使用率超过80%",
    },
    Run: func(ctx context.Context) timewheel.AlarmResult {
        cpuUsage := getCPUUsage() // 获取CPU使用率
        
        return timewheel.AlarmResult{
            Value:     cpuUsage,
            Threshold: 80.0,
            IsFiring:  cpuUsage > 80.0,
        }
    },
})
```

### 多层时间轮示例

```go
// 创建多层时间轮（推荐用于生产环境）
mltw, err := timewheel.NewMultiLevelTimeWheel()
if err != nil {
    panic(err)
}

// 启动
if err := mltw.Start(); err != nil {
    panic(err)
}
defer mltw.Stop()

// 高优先级任务（10ms精度）
mltw.AddTask(&timewheel.Task{
    ID:        "emergency-alert",
    Priority:  timewheel.TaskPriorityHigh,
    Interval:  100 * time.Millisecond,
    Mode:      timewheel.TaskModeRepeated,
    Run:       emergencyCheck,
})

// 普通优先级任务（100ms精度）
mltw.AddTask(&timewheel.Task{
    ID:        "normal-task",
    Priority:  timewheel.TaskPriorityNormal,
    Interval:  1 * time.Second,
    Mode:      timewheel.TaskModeRepeated,
    Run:       normalTask,
})

// 低优先级任务（1s精度）
mltw.AddTask(&timewheel.Task{
    ID:        "cleanup-task",
    Priority:  timewheel.TaskPriorityLow,
    Interval:  1 * time.Minute,
    Mode:      timewheel.TaskModeRepeated,
    Run:       cleanupTask,
})
```

---

## API 文档

### TimeWheel

#### 构造函数

```go
func New(opts ...Option) (*TimeWheel, error)
```

创建时间轮实例。

**参数**：
- `opts`: 可选配置项，参见[配置选项](#配置选项)

**返回**：
- `*TimeWheel`: 时间轮实例
- `error`: 创建失败时返回错误

**错误**：
- `ErrSlotsTooFew`: 槽位数量必须大于0
- `ErrIntervalTooSmall`: 时间间隔必须大于0

---

#### Start

```go
func (tw *TimeWheel) Start() error
```

启动时间轮。

**注意**：必须先启动时间轮，才能添加任务。

---

#### Stop

```go
func (tw *TimeWheel) Stop()
```

优雅停止时间轮。

**行为**：
- 停止接收新任务
- 等待所有正在执行的任务完成
- 释放协程池资源

---

#### AddTask

```go
func (tw *TimeWheel) AddTask(task *Task) error
```

添加定时任务。

**参数**：
- `task`: 任务结构体

**错误**：
- `ErrInvalidParam`: 任务参数无效
- `ErrWheelNotRunning`: 时间轮未运行
- `ErrTaskAlreadyExists`: 任务ID已存在

---

#### RemoveTask

```go
func (tw *TimeWheel) RemoveTask(taskID string) error
```

移除定时任务。

**错误**：
- `ErrTaskNotFound`: 任务不存在

---

#### UpdateTask

```go
func (tw *TimeWheel) UpdateTask(task *Task) error
```

更新任务。先删除旧任务，再添加新任务。

---

#### PauseTask / ResumeTask

```go
func (tw *TimeWheel) PauseTask(taskID string) error
func (tw *TimeWheel) ResumeTask(taskID string) error
```

暂停/恢复任务。

---

#### GetTask

```go
func (tw *TimeWheel) GetTask(id string) *taskSlot
```

获取任务信息（内部使用）。

---

#### GetAllTasks

```go
func (tw *TimeWheel) GetAllTasks() []*Task
```

获取所有任务列表。

---

#### GetTasksByState

```go
func (tw *TimeWheel) GetTasksByState(state AlertState) []*Task
```

根据告警状态获取任务列表。

**参数**：
- `state`: `AlertStatePending`、`AlertStateFiring`、`AlertStateResolved`

---

#### ClearAllTasks

```go
func (tw *TimeWheel) ClearAllTasks() int
```

清空所有任务，返回清空数量。

---

#### Stats

```go
type Metrics struct {
    TotalTasks        int64          // 当前任务总数
    RunningTasks      int32          // 正在执行的任务数
    Executed          int64          // 已执行任务总数
    TotalTasksAdded   int64          // 总添加任务数
    TotalTasksRemoved int64          // 总移除任务数
    TotalTasksFailed  int64          // 总失败任务数
    TotalTaskPanics   int64          // 总panic任务数
    TotalTaskTime     int64          // 总任务执行时间（微秒）
    TotalCacheHits    int64          // 总缓存命中数
    TotalCacheMisses  int64          // 总缓存未命中数
    TotalAlertsFired  int64          // 总告警触发数
    CacheStats        TaskCacheStats // 缓存统计
    // ...
}

func (tw *TimeWheel) Stats() Metrics
```

获取时间轮统计指标。

---

### MultiLevelTimeWheel

#### 构造函数

```go
func NewMultiLevelTimeWheel() (*MultiLevelTimeWheel, error)
```

创建多层时间轮。

**配置**：
| 优先级 | 时间间隔 | 槽位数量 | 协程池大小 |
|--------|---------|---------|-----------|
| High   | 10ms    | 60      | CPU核心数×3 |
| Normal | 100ms   | 60      | CPU核心数   |
| Low    | 1s      | 60      | CPU核心数/2 |

---

#### 方法

```go
func (mltw *MultiLevelTimeWheel) Start() error
func (mltw *MultiLevelTimeWheel) Stop()
func (mltw *MultiLevelTimeWheel) AddTask(task *Task) error
func (mltw *MultiLevelTimeWheel) RemoveTask(taskID string) error
func (mltw *MultiLevelTimeWheel) UpdateTask(task *Task) error
func (mltw *MultiLevelTimeWheel) PauseTask(taskID string) error
func (mltw *MultiLevelTimeWheel) ResumeTask(taskID string) error
func (mltw *MultiLevelTimeWheel) GetAllTasks() []*Task
func (mltw *MultiLevelTimeWheel) ClearAllTasks() int
```

---

## 配置选项

### WithSlotNum

```go
func WithSlotNum(num int) Option
```

设置槽位数量，默认 60。

**建议**：槽位数量 × 时间间隔 = 时间轮覆盖的时间范围

---

### WithInterval

```go
func WithInterval(interval time.Duration) Option
```

设置时间轮转间隔，默认 100ms。

---

### WithLogger

```go
func WithLogger(logger *log.Logger) Option
```

设置自定义日志记录器。

---

### WithLogLevel

```go
func WithLogLevel(level int) Option
```

设置日志级别：
- `LogLevelDebug = 0`
- `LogLevelInfo = 1`
- `LogLevelWarn = 2`
- `LogLevelError = 3`

---

### WithStatusInterval

```go
func WithStatusInterval(interval time.Duration) Option
```

设置状态打印间隔。启用后会定时打印时间轮运行状态。

---

### WithMaxConcurrentTasks

```go
func WithMaxConcurrentTasks(max int) Option
```

设置最大并发任务数，0 表示无限制。

---

### WithErrorCallback

```go
func WithErrorCallback(fn func(error)) Option
```

设置错误回调函数。

---

### WithCache

```go
func WithCache(enabled bool) Option
```

启用/禁用任务缓存。

---

### WithHistoryFile

```go
func WithHistoryFile(filePath string) Option
```

设置告警历史文件路径。

---

### WithHistoryRetention

```go
func WithHistoryRetention(days int) Option
```

设置告警历史保留天数，默认 30 天。

---

### WithSQLiteStore

```go
func WithSQLiteStore(dbPath string) Option
```

配置 SQLite 持久化存储，任务和历史记录将保存到 SQLite 数据库。

**示例**：
```go
tw, _ := timewheel.New(
    timewheel.WithSQLiteStore("./data/timewheel.db"),
    timewheel.WithAutoRestore(true),
)
```

---

### WithMySQLStore

```go
func WithMySQLStore(dsn string) Option
```

配置 MySQL 持久化存储。

---

### WithAutoRestore

```go
func WithAutoRestore(enabled bool) Option
```

配置启动时是否自动从存储恢复任务。需要配合 `WithSQLiteStore` 或 `WithMySQLStore` 使用。

---

## 任务模式

### TaskModeOnce - 单次执行

```go
tw.AddTask(&timewheel.Task{
    ID:       "once-task",
    Mode:     timewheel.TaskModeOnce,
    Interval: 5 * time.Second, // 5秒后执行
    Run: func(ctx context.Context) timewheel.AlarmResult {
        fmt.Println("执行一次")
        return timewheel.AlarmResult{}
    },
})
```

---

### TaskModeFixedTimes - 固定次数

```go
tw.AddTask(&timewheel.Task{
    ID:       "fixed-task",
    Mode:     timewheel.TaskModeFixedTimes,
    Interval: 1 * time.Second,
    Times:    3, // 执行3次
    Run: func(ctx context.Context) timewheel.AlarmResult {
        fmt.Println("执行固定次数")
        return timewheel.AlarmResult{}
    },
})
```

---

### TaskModeRepeated - 周期重复（默认）

```go
tw.AddTask(&timewheel.Task{
    ID:       "repeated-task",
    Mode:     timewheel.TaskModeRepeated,
    Interval: 1 * time.Second,
    Run: func(ctx context.Context) timewheel.AlarmResult {
        fmt.Println("周期执行")
        return timewheel.AlarmResult{}
    },
})
```

---

## 告警服务

### 告警级别

```go
const (
    SeverityCritical Severity = iota // 严重告警
    SeverityWarning                  // 警告
    SeverityInfo                     // 信息
)
```

---

### 告警状态

```go
const (
    AlertStatePending  AlertState = iota // 条件满足但未达到持续时间
    AlertStateFiring                     // 条件满足且达到持续时间
    AlertStateResolved                   // 从 Firing 转为不满足条件
)
```

---

### For 持续时间

条件需要持续满足指定时间才会触发告警：

```go
tw.AddTask(&timewheel.Task{
    ID:       "cpu-alert",
    Interval: 10 * time.Second,
    For:      1 * time.Minute, // CPU使用率持续1分钟超过阈值才告警
    Run: func(ctx context.Context) timewheel.AlarmResult {
        return timewheel.AlarmResult{
            Value:     getCPUUsage(),
            Threshold: 80.0,
            IsFiring:  getCPUUsage() > 80.0,
        }
    },
})
```

---

### 重复告警间隔

告警触发后，每隔指定时间重复告警：

```go
tw.AddTask(&timewheel.Task{
    ID:             "cpu-alert",
    Interval:       10 * time.Second,
    For:            1 * time.Minute,
    RepeatInterval: 5 * time.Minute, // 每5分钟重复告警
    Run:            cpuCheck,
})
```

---

### 状态变化回调

```go
tw, _ := timewheel.New(
    timewheel.WithAlertStateChangeCallback(func(taskID string, oldState, newState timewheel.AlertState, result timewheel.AlarmResult) {
        fmt.Printf("任务 %s 状态变化: %v -> %v, 当前值: %.2f\n",
            taskID, oldState, newState, result.Value)
    }),
)
```

---

### 告警历史持久化

```go
tw, _ := timewheel.New(
    timewheel.WithHistoryFile("/var/log/timewheel/alerts.json"),
    timewheel.WithHistoryRetention(30), // 保留30天
)
```

---

## 多层时间轮

### 为什么需要多层时间轮？

| 场景 | 单层时间轮 | 多层时间轮 |
|------|-----------|-----------|
| 任务精度 | 所有任务同一精度 | 不同优先级不同精度 |
| 资源消耗 | 不区分任务重要性 | 按优先级分配资源 |
| 响应速度 | 无优先级保证 | 高优先级更快响应 |

### 使用建议

```
┌─────────────────────────────────────────────────────────────┐
│                    任务优先级选择指南                        │
├─────────────────────────────────────────────────────────────┤
│  High (10ms精度)                                             │
│  ├── 紧急告警（火灾、入侵）                                   │
│  ├── 心跳检测                                                │
│  └── 实时监控                                                │
├─────────────────────────────────────────────────────────────┤
│  Normal (100ms精度)                                          │
│  ├── 普通告警                                                │
│  ├── 数据同步                                                │
│  └── 定期检查                                                │
├─────────────────────────────────────────────────────────────┤
│  Low (1s精度)                                                │
│  ├── 日志清理                                                │
│  ├── 统计报表                                                │
│  └── 缓存刷新                                                │
└─────────────────────────────────────────────────────────────┘
```

---

## 性能指标

### 基准测试

```bash
cd timewheel
go test -bench=. -benchmem ./...
```

### 预期性能

| 指标 | 数值 | 说明 |
|------|------|------|
| **任务容量** | 100,000+ | 单时间轮支持任务数 |
| **触发精度** | ±1ms | 100ms间隔下的精度 |
| **吞吐量** | 50,000+ tasks/s | 任务执行吞吐量 |
| **内存占用** | ~10MB | 10,000任务时 |
| **CPU占用** | <5% | 空载时 |

### 性能优化技术

| 技术 | 说明 | 收益 |
|------|------|------|
| 分片锁 | 64分片减少锁竞争 | 并发性能提升 10x |
| 对象池 | sync.Pool 复用对象 | GC 压力减少 50% |
| 协程池 | ants 管理 goroutine | goroutine 数量可控 |
| 任务缓存 | LRU 缓存加速查找 | 查找性能提升 100x |

---

## 最佳实践

### 1. 合理设置时间轮参数

```go
// 推荐：根据业务需求选择参数
tw, _ := timewheel.New(
    timewheel.WithSlotNum(60),              // 槽位数量
    timewheel.WithInterval(100*time.Millisecond), // 轮转间隔
    timewheel.WithMaxConcurrentTasks(1000), // 并发限制
)
```

### 2. 使用 Context 控制任务生命周期

```go
tw.AddTask(&timewheel.Task{
    Run: func(ctx context.Context) timewheel.AlarmResult {
        select {
        case <-ctx.Done():
            return timewheel.AlarmResult{} // 任务被取消
        default:
            // 执行任务逻辑
        }
        return timewheel.AlarmResult{}
    },
})
```

### 3. 设置任务超时

```go
tw.AddTask(&timewheel.Task{
    Timeout: 5 * time.Second, // 任务超时时间
    Run: func(ctx context.Context) timewheel.AlarmResult {
        // 任务会在超时后自动取消
        return timewheel.AlarmResult{}
    },
})
```

### 4. 处理任务错误

```go
tw, _ := timewheel.New(
    timewheel.WithErrorCallback(func(err error) {
        log.Printf("任务执行错误: %v", err)
        // 发送到监控系统
    }),
)
```

### 5. 监控时间轮状态

```go
tw, _ := timewheel.New(
    timewheel.WithStatusInterval(1*time.Minute), // 每分钟打印状态
)

// 或手动获取指标
metrics := tw.Stats()
log.Printf("任务数: %d, 已执行: %d, 失败: %d",
    metrics.TotalTasks, metrics.Executed, metrics.TotalTasksFailed)
```

### 6. 优雅停机

```go
// 使用 defer 确保资源释放
defer tw.Stop()

// 或监听系统信号
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
<-sigCh
tw.Stop()
```

---

## 常见问题

### Q: 任务执行时间超过了轮转间隔怎么办？

A: 任务在独立 goroutine 中执行，不会阻塞时间轮轮转。但要注意控制并发任务数量，避免资源耗尽。

```go
timewheel.WithMaxConcurrentTasks(1000) // 限制最大并发数
```

### Q: 如何保证任务不丢失？

A: 
1. 使用 `defer tw.Stop()` 确保优雅停机
2. 停机时会等待所有正在执行的任务完成
3. 对于关键任务，建议配合持久化队列使用

### Q: 时间轮精度不够怎么办？

A: 减小轮转间隔或使用多层时间轮：

```go
// 方式一：减小间隔
timewheel.WithInterval(10*time.Millisecond)

// 方式二：使用多层时间轮（高优先级10ms精度）
mltw, _ := timewheel.NewMultiLevelTimeWheel()
```

### Q: 如何处理任务 panic？

A: 时间轮会自动捕获 panic 并恢复，通过错误回调通知：

```go
timewheel.WithErrorCallback(func(err error) {
    log.Printf("任务panic: %v", err)
})
```

### Q: 缓存有什么作用？

A: 启用缓存可以显著加速 `GetTask` 操作：

```go
timewheel.WithCache(true)
```

---

## 贡献指南

1. Fork 项目
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

### 代码规范

- 遵循 [Effective Go](https://golang.org/doc/effective_go)
- 添加单元测试
- 更新相关文档

---

## 许可证

[MIT License](LICENSE)

---

## 更新日志

### v1.0.0 (2026-03-01)
- 初始版本发布
- 支持多种任务模式
- 支持任务优先级
- 支持 Prometheus 风格告警
- 多层时间轮支持
- 告警历史持久化
