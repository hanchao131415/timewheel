package timewheel

import (
	"encoding/json"
	"net/http"
	"time"
)

// Health 返回健康状态
func (tw *TimeWheel) Health() *HealthStatus {
	status := "healthy"
	components := map[string]string{
		"pool":    "healthy",
		"cache":   "healthy",
		"storage": "healthy",
	}

	if !tw.running.Load() {
		status = "unhealthy"
	}

	totalTasks, _ := tw.Stats()

	return &HealthStatus{
		Status:     status,
		Running:    tw.running.Load(),
		TaskCount:  int64(totalTasks),
		Uptime:     time.Since(tw.startTime).String(),
		StartTime:  tw.startTime,
		Components: components,
	}
}

// HTTPHandler 返回 HTTP 处理器
func (tw *TimeWheel) HTTPHandler() http.Handler {
	mux := http.NewServeMux()

	// 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		health := tw.Health()
		w.Header().Set("Content-Type", "application/json")
		if health.Status != "healthy" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(health)
	})

	// 就绪检查
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !tw.running.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]bool{"ready": false})
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"ready": true})
	})

	return mux
}
