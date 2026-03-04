package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"timewheel/internal/config"
	"timewheel/internal/model/dto"
)

// Auth 认证中间件
func Auth(cfg *config.AuthConfig, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.Enabled {
			c.Next()
			return
		}

		switch cfg.Type {
		case "jwt":
			jwtAuth(cfg.JWT, logger)(c)
		case "api_key":
			apiKeyAuth(cfg.APIKey, logger)(c)
		default:
			c.Next()
		}
	}
}

// jwtAuth JWT 认证中间件
func jwtAuth(cfg config.JWTConfig, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse(401, "Missing authorization header"))
			return
		}

		// 解析 Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse(401, "Invalid authorization format"))
			return
		}

		tokenString := parts[1]

		// 解析 token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(cfg.Secret), nil
		})

		if err != nil {
			logger.Debug("JWT validation failed", zap.Error(err))
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse(401, "Invalid token"))
			return
		}

		if !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse(401, "Invalid token"))
			return
		}

		// 提取 claims
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			c.Set("user_id", claims["sub"])
			c.Set("issuer", claims["iss"])
		}

		c.Next()
	}
}

// apiKeyAuth API Key 认证中间件
func apiKeyAuth(cfg config.APIKeyConfig, logger *zap.Logger) gin.HandlerFunc {
	validKeys := make(map[string]string)
	for _, entry := range cfg.Keys {
		validKeys[entry.Key] = entry.Name
	}

	return func(c *gin.Context) {
		apiKey := c.GetHeader(cfg.Header)
		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse(401, "Missing API key"))
			return
		}

		if name, ok := validKeys[apiKey]; ok {
			c.Set("api_key_name", name)
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse(401, "Invalid API key"))
	}
}

// RateLimit 限流中间件
func RateLimit(cfg *config.RateLimitConfig, logger *zap.Logger) gin.HandlerFunc {
	if !cfg.Enabled {
		return func(c *gin.Context) { c.Next() }
	}

	// 简单的令牌桶实现
	// 生产环境应使用更健壮的实现，如 golang.org/x/time/rate
	type client struct {
		tokens    float64
		lastCheck time.Time
	}

	clients := make(map[string]*client)
	limit := cfg.RequestsPerSecond
	burst := float64(cfg.Burst)

	return func(c *gin.Context) {
		var key string
		if cfg.ByIP {
			key = c.ClientIP()
		} else {
			key = "global"
		}

		now := time.Now()
		var cl *client
		var exists bool

		if cl, exists = clients[key]; !exists {
			cl = &client{
				tokens:    burst,
				lastCheck: now,
			}
			clients[key] = cl
		}

		// 补充令牌
		elapsed := now.Sub(cl.lastCheck).Seconds()
		cl.tokens += elapsed * limit
		if cl.tokens > burst {
			cl.tokens = burst
		}
		cl.lastCheck = now

		// 检查令牌
		if cl.tokens < 1 {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, dto.ErrorResponse(429, "Rate limit exceeded"))
			return
		}

		cl.tokens--
		c.Next()
	}
}

// Logger 日志中间件
func Logger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		if status >= 400 {
			logger.Warn("HTTP request",
				zap.Int("status", status),
				zap.String("method", c.Request.Method),
				zap.String("path", path),
				zap.String("query", query),
				zap.Duration("latency", latency),
				zap.String("client_ip", c.ClientIP()),
				zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()),
			)
		} else {
			logger.Info("HTTP request",
				zap.Int("status", status),
				zap.String("method", c.Request.Method),
				zap.String("path", path),
				zap.String("query", query),
				zap.Duration("latency", latency),
				zap.String("client_ip", c.ClientIP()),
			)
		}
	}
}

// Recovery 恢复中间件
func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("Panic recovered",
					zap.Any("error", err),
					zap.String("path", c.Request.URL.Path),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, dto.ErrorResponse(500, "Internal server error"))
			}
		}()
		c.Next()
	}
}

// CORS 跨域中间件
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-API-Key")
		c.Header("Access-Control-Expose-Headers", "Content-Length, Content-Type")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// RequestID 请求 ID 中间件
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

func generateRequestID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}
