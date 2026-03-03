package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"timewheel/internal/config"
)

// Client Webhook 客户端
type Client struct {
	cfg    *config.WebhookConfig
	client *http.Client
	logger *zap.Logger
}

// AlertPayload 告警载荷
type AlertPayload struct {
	TaskID      string            `json:"task_id"`
	TaskName    string            `json:"task_name"`
	State       string            `json:"state"`
	Value       float64           `json:"value"`
	Threshold   float64           `json:"threshold"`
	Severity    string            `json:"severity"`
	Timestamp   time.Time         `json:"timestamp"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// NewClient 创建 Webhook 客户端
func NewClient(cfg *config.WebhookConfig, logger *zap.Logger) *Client {
	return &Client{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger: logger,
	}
}

// Send 发送告警
func (c *Client) Send(ctx context.Context, payload *AlertPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.cfg.Headers {
		req.Header.Set(k, v)
	}

	// 发送请求
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(respBody))
	}

	c.logger.Info("Webhook sent successfully",
		zap.String("task_id", payload.TaskID),
		zap.String("state", payload.State),
	)

	return nil
}

// SendWithRetry 带重试的发送
func (c *Client) SendWithRetry(ctx context.Context, payload *AlertPayload) error {
	var lastErr error
	interval := c.cfg.Retry.InitialInterval

	for attempt := 1; attempt <= c.cfg.Retry.MaxAttempts; attempt++ {
		err := c.Send(ctx, payload)
		if err == nil {
			return nil
		}

		lastErr = err
		c.logger.Warn("Webhook send failed, will retry",
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", c.cfg.Retry.MaxAttempts),
			zap.Error(err),
		)

		// 等待下次重试
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}

		// 计算下次重试间隔（指数退避）
		interval = time.Duration(float64(interval) * c.cfg.Retry.Multiplier)
		if interval > c.cfg.Retry.MaxInterval {
			interval = c.cfg.Retry.MaxInterval
		}
	}

	return fmt.Errorf("webhook failed after %d attempts: %w", c.cfg.Retry.MaxAttempts, lastErr)
}

// BatchSender 批量发送器
type BatchSender struct {
	client *Client
	cfg    *config.WebhookBatchConfig
	queue  chan *AlertPayload
	logger *zap.Logger
}

// NewBatchSender 创建批量发送器
func NewBatchSender(client *Client, cfg *config.WebhookBatchConfig, logger *zap.Logger) *BatchSender {
	return &BatchSender{
		client: client,
		cfg:    cfg,
		queue:  make(chan *AlertPayload, cfg.Size*2),
		logger: logger,
	}
}

// Enqueue 加入发送队列
func (bs *BatchSender) Enqueue(payload *AlertPayload) error {
	select {
	case bs.queue <- payload:
		return nil
	default:
		return fmt.Errorf("batch queue is full")
	}
}

// Start 启动批量发送
func (bs *BatchSender) Start(ctx context.Context) {
	ticker := time.NewTicker(bs.cfg.Interval)
	defer ticker.Stop()

	batch := make([]*AlertPayload, 0, bs.cfg.Size)

	for {
		select {
		case <-ctx.Done():
			// 处理剩余消息
			if len(batch) > 0 {
				bs.sendBatch(ctx, batch)
			}
			return
		case payload := <-bs.queue:
			batch = append(batch, payload)
			if len(batch) >= bs.cfg.Size {
				bs.sendBatch(ctx, batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				bs.sendBatch(ctx, batch)
				batch = batch[:0]
			}
		}
	}
}

func (bs *BatchSender) sendBatch(ctx context.Context, batch []*AlertPayload) {
	for _, payload := range batch {
		if err := bs.client.SendWithRetry(ctx, payload); err != nil {
			bs.logger.Error("Failed to send alert",
				zap.String("task_id", payload.TaskID),
				zap.Error(err),
			)
		}
	}
}
