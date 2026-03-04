# TimeWheel 项目指南

> 高性能、低延迟的分布式时间轮任务调度系统

## 项目概述

这是一个 Go 语言编写的时间轮（TimeWheel）任务调度系统，设计目标：

- **高并发** - 支持百万级任务调度
- **低延迟** - 毫秒级任务触发精度
- **零 GC** - 对象池复用，减少 GC 压力
- **高可用** - 分布式锁 + 数据库持久化

## 技术栈

| 组件 | 技术 |
|------|------|
| Web 框架 | Gin |
| ORM | GORM (MySQL/SQLite) |
| 日志 | Zap + Lumberjack |
| 配置 | Viper (YAML) |
| 协程池 | ants |
| 缓存 | Redis |
| ID 生成 | Snowflake |

## 项目结构

```
timewheel/
├── cmd/server/           # 入口程序
├── internal/
│   ├── config/           # 配置加载
│   ├── handler/          # HTTP 处理器
│   ├── model/dto/        # 数据传输对象
│   ├── repository/       # 数据访问层
│   │   ├── db/           # 数据库连接
│   │   └── model/        # 数据模型
│   ├── server/           # HTTP 服务器 & 中间件
│   └── service/          # 业务逻辑层
├── pkg/
│   ├── timewheel/        # 核心时间轮实现
│   ├── distributed/      # 分布式锁
│   ├── snowflake/        # 雪花算法 ID 生成
│   ├── webhook/          # Webhook 通知
│   └── logger/           # 日志封装
├── tests/
│   ├── fixtures/         # 测试固件
│   ├── testutil/         # 测试工具
│   ├── integration/      # 集成测试
│   ├── performance/      # 性能测试
│   └── chaos/            # 混沌测试
├── configs/              # 配置文件
├── migrations/           # 数据库迁移
└── deployments/          # 部署配置 (Docker/K8s)
```

## 开发规范

### 代码风格

- 遵循 Go 官方代码规范
- 使用 `gofmt` 格式化
- 错误处理必须显式，禁止忽略
- 函数复杂度控制在 15 以内

### 测试要求

```bash
# 运行所有测试
go test ./...

# 运行带覆盖率的测试
go test -cover ./...

# 运行性能测试
go test -bench=. ./tests/performance/...
```

**测试覆盖率要求：80%+**

### 提交规范

```
<type>: <description>

Types: feat, fix, refactor, docs, test, chore, perf
```

## 常用命令

```bash
# 启动开发服务器
cd timewheel && go run cmd/server/main.go

# 构建生产版本
cd timewheel && go build -o bin/timewheel cmd/server/main.go

# 运行 Docker
docker-compose up -d

# 数据库迁移
mysql -u root -p < migrations/001_init.sql
```

## 性能目标

| 指标 | 目标值 |
|------|--------|
| 任务吞吐量 | 100K+/s |
| 触发延迟 | < 10ms |
| 内存占用 | < 100MB (100万任务) |
| GC 暂停 | < 1ms |

## 关键设计决策

1. **雪花算法主键** - 支持多节点分布式部署
2. **字符串格式主键** - 便于 API 传输和日志追踪
3. **原子性保证** - 内存操作失败时数据库回滚
4. **对象池复用** - 减少 GC 压力，追求零 GC 目标

## 相关文档

- [架构设计](./docs/architecture.md)
- [API 文档](./docs/api.md)
- [部署指南](./docs/deployment.md)
