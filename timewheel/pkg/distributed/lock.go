package distributed

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// DistributedLock 分布式锁接口
type DistributedLock interface {
	// TryLock 尝试获取锁
	TryLock(ctx context.Context, key string, ttl time.Duration) (bool, error)
	// Lock 获取锁（阻塞）
	Lock(ctx context.Context, key string, ttl time.Duration, timeout time.Duration) error
	// Unlock 释放锁
	Unlock(ctx context.Context, key string) error
	// Renew 续租锁
	Renew(ctx context.Context, key string, ttl time.Duration) error
}

// RedisLock Redis 分布式锁实现
type RedisLock struct {
	client *redis.Client
	prefix string
	logger *zap.Logger
}

// NewRedisLock 创建 Redis 分布式锁
func NewRedisLock(client *redis.Client, prefix string, logger *zap.Logger) *RedisLock {
	return &RedisLock{
		client: client,
		prefix: prefix,
		logger: logger,
	}
}

// lockKey 获取锁的完整 key
func (rl *RedisLock) lockKey(key string) string {
	return rl.prefix + ":" + key
}

// TryLock 尝试获取锁
func (rl *RedisLock) TryLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	fullKey := rl.lockKey(key)

	// 使用 SET NX EX 原子操作
	ok, err := rl.client.SetNX(ctx, fullKey, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}

	if ok {
		rl.logger.Debug("Lock acquired",
			zap.String("key", fullKey),
			zap.Duration("ttl", ttl),
		)
	}

	return ok, nil
}

// Lock 获取锁（阻塞，带超时）
func (rl *RedisLock) Lock(ctx context.Context, key string, ttl time.Duration, timeout time.Duration) error {
	fullKey := rl.lockKey(key)
	deadline := time.Now().Add(timeout)
	retryInterval := 100 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("lock acquisition timeout for key: %s", fullKey)
		}

		ok, err := rl.TryLock(ctx, key, ttl)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}

		time.Sleep(retryInterval)
	}
}

// Unlock 释放锁
func (rl *RedisLock) Unlock(ctx context.Context, key string) error {
	fullKey := rl.lockKey(key)

	deleted, err := rl.client.Del(ctx, fullKey).Result()
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	if deleted > 0 {
		rl.logger.Debug("Lock released", zap.String("key", fullKey))
	}

	return nil
}

// Renew 续租锁
func (rl *RedisLock) Renew(ctx context.Context, key string, ttl time.Duration) error {
	fullKey := rl.lockKey(key)

	ok, err := rl.client.Expire(ctx, fullKey, ttl).Result()
	if err != nil {
		return fmt.Errorf("failed to renew lock: %w", err)
	}

	if !ok {
		return fmt.Errorf("lock does not exist: %s", fullKey)
	}

	rl.logger.Debug("Lock renewed",
		zap.String("key", fullKey),
		zap.Duration("ttl", ttl),
	)

	return nil
}

// LocalLock 本地锁（单机模式）
type LocalLock struct {
	locks  map[string]time.Time
	done   chan struct{}
	logger *zap.Logger
}

// NewLocalLock 创建本地锁
func NewLocalLock(logger *zap.Logger) *LocalLock {
	ll := &LocalLock{
		locks:  make(map[string]time.Time),
		done:   make(chan struct{}),
		logger: logger,
	}

	// 启动清理协程
	go ll.cleanup()

	return ll
}

// cleanup 定期清理过期锁
func (ll *LocalLock) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ll.done:
			return
		case <-ticker.C:
			now := time.Now()
			for key, expiry := range ll.locks {
				if now.After(expiry) {
					delete(ll.locks, key)
				}
			}
		}
	}
}

// Close 关闭
func (ll *LocalLock) Close() {
	close(ll.done)
}

// TryLock 尝试获取锁
func (ll *LocalLock) TryLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	now := time.Now()
	expiry := now.Add(ttl)

	if expiryTime, exists := ll.locks[key]; exists {
		if now.Before(expiryTime) {
			return false, nil
		}
	}

	ll.locks[key] = expiry
	ll.logger.Debug("Local lock acquired", zap.String("key", key))
	return true, nil
}

// Lock 获取锁（阻塞）
func (ll *LocalLock) Lock(ctx context.Context, key string, ttl time.Duration, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	retryInterval := 100 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("lock acquisition timeout for key: %s", key)
		}

		ok, err := ll.TryLock(ctx, key, ttl)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}

		time.Sleep(retryInterval)
	}
}

// Unlock 释放锁
func (ll *LocalLock) Unlock(ctx context.Context, key string) error {
	delete(ll.locks, key)
	ll.logger.Debug("Local lock released", zap.String("key", key))
	return nil
}

// Renew 续租锁
func (ll *LocalLock) Renew(ctx context.Context, key string, ttl time.Duration) error {
	if _, exists := ll.locks[key]; !exists {
		return fmt.Errorf("lock does not exist: %s", key)
	}

	ll.locks[key] = time.Now().Add(ttl)
	ll.logger.Debug("Local lock renewed", zap.String("key", key))
	return nil
}
