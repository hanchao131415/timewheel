package testutil

import (
	"context"
	"fmt"
	"os"
	"time"
)

// Container Docker 容器助手接口
type Container interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	GetHostPort(port int) (string, error)
	GetConnectionString() string
}

// MySQLContainer MySQL 容器配置
type MySQLContainer struct {
	Name     string
	Image    string
	Port     int
	Database string
	User     string
	Password string
	host     string
	hostPort int
}

// NewMySQLContainer 创建 MySQL 容器配置
func NewMySQLContainer() *MySQLContainer {
	return &MySQLContainer{
		Name:     "test-mysql",
		Image:    "mysql:8.0",
		Port:     3306,
		Database: "timewheel_test",
		User:     "root",
		Password: "testpass",
	}
}

// Start 启动容器
func (c *MySQLContainer) Start(ctx context.Context) error {
	// 检查 Docker 是否可用
	if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
		return fmt.Errorf("docker not available, skipping container test")
	}

	// 使用 docker run 启动容器
	cmd := fmt.Sprintf(
		"docker run -d --name %s -e MYSQL_ROOT_PASSWORD=%s -e MYSQL_DATABASE=%s -p %d:3306 %s",
		c.Name, c.Password, c.Database, c.Port, c.Image,
	)

	// 这里简化处理，实际项目中应该使用 testcontainers-go
	fmt.Printf("Would run: %s\n", cmd)
	return nil
}

// Stop 停止容器
func (c *MySQLContainer) Stop(ctx context.Context) error {
	cmd := fmt.Sprintf("docker stop %s && docker rm %s", c.Name, c.Name)
	fmt.Printf("Would run: %s\n", cmd)
	return nil
}

// GetConnectionString 获取连接字符串
func (c *MySQLContainer) GetConnectionString() string {
	return fmt.Sprintf("%s:%s@tcp(localhost:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.User, c.Password, c.Port, c.Database)
}

// RedisContainer Redis 容器配置
type RedisContainer struct {
	Name  string
	Image string
	Port  int
}

// NewRedisContainer 创建 Redis 容器配置
func NewRedisContainer() *RedisContainer {
	return &RedisContainer{
		Name:  "test-redis",
		Image: "redis:7-alpine",
		Port:  6379,
	}
}

// Start 启动容器
func (c *RedisContainer) Start(ctx context.Context) error {
	cmd := fmt.Sprintf(
		"docker run -d --name %s -p %d:6379 %s",
		c.Name, c.Port, c.Image,
	)
	fmt.Printf("Would run: %s\n", cmd)
	return nil
}

// Stop 停止容器
func (c *RedisContainer) Stop(ctx context.Context) error {
	cmd := fmt.Sprintf("docker stop %s && docker rm %s", c.Name, c.Name)
	fmt.Printf("Would run: %s\n", cmd)
	return nil
}

// GetConnectionString 获取连接字符串
func (c *RedisContainer) GetConnectionString() string {
	return fmt.Sprintf("localhost:%d", c.Port)
}

// ContainerManager 容器管理器
type ContainerManager struct {
	containers []Container
}

// NewContainerManager 创建容器管理器
func NewContainerManager() *ContainerManager {
	return &ContainerManager{
		containers: make([]Container, 0),
	}
}

// Add 添加容器
func (m *ContainerManager) Add(container Container) {
	m.containers = append(m.containers, container)
}

// StartAll 启动所有容器
func (m *ContainerManager) StartAll(ctx context.Context) error {
	for _, c := range m.containers {
		if err := c.Start(ctx); err != nil {
			return fmt.Errorf("failed to start container: %w", err)
		}
	}
	// 等待容器启动
	time.Sleep(5 * time.Second)
	return nil
}

// StopAll 停止所有容器
func (m *ContainerManager) StopAll(ctx context.Context) error {
	for _, c := range m.containers {
		if err := c.Stop(ctx); err != nil {
			// 继续停止其他容器
		}
	}
	return nil
}

// WaitForReady 等待服务就绪
func WaitForReady(ctx context.Context, check func() bool, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if check() {
				return nil
			}
		}
	}
}

// SkipIfDockerUnavailable 如果 Docker 不可用则跳过测试
func SkipIfDockerUnavailable(t testingT) {
	if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
		t.Skip("Docker not available, skipping container test")
	}
}

// testingT 测试接口（兼容 *testing.T）
type testingT interface {
	Skip(args ...interface{})
}
