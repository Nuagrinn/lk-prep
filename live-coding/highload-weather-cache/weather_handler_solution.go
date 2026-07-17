// Интервью-задача: highload weather RPC/HTTP handler.
//
// Условие:
// Есть функция, которая через нейронную сеть вычисляет прогноз погоды примерно
// за 1 секунду. Есть highload ручка с нагрузкой около 10k RPS. Нужно
// реализовать код ручки.
//
// Наивное решение вызывает aiWeatherForecast() прямо из handler-а. Это плохо:
// каждый запрос занимает около секунды, создаёт огромную конкурентную нагрузку
// на дорогой dependency и быстро ломает latency/RPS.
//
// Идея решения:
//   - тяжелый прогноз считается в фоне по ticker-у;
//   - handler отдаёт последнее успешно посчитанное значение;
//   - чтение из handler-а должно быть очень дешёвым;
//   - если значение ещё не готово, возвращаем 503;
//   - если обновление не успело или упало, обычно оставляем старое значение.
//
// Здесь используется atomic.Value: один writer редко заменяет целый immutable
// snapshot, а много readers быстро читают готовое значение без mutex-а.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"
)

// aiWeatherForecast через нейронную сеть вычисляет прогноз погоды за ~1 секунду.
func aiWeatherForecast() int {
	time.Sleep(1 * time.Second)
	return rand.Intn(70) - 30
}

type WeatherSnapshot struct {
	Temperature int       `json:"temperature"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type WeatherService struct {
	current atomic.Value // stores WeatherSnapshot
}

func NewWeatherService() *WeatherService {
	return &WeatherService{}
}

func (s *WeatherService) Snapshot() (WeatherSnapshot, bool) {
	v := s.current.Load()
	if v == nil {
		return WeatherSnapshot{}, false
	}
	return v.(WeatherSnapshot), true
}

func (s *WeatherService) refreshOnce() {
	temperature := aiWeatherForecast()
	s.current.Store(WeatherSnapshot{
		Temperature: temperature,
		UpdatedAt:   time.Now(),
	})
}

func (s *WeatherService) StartRefreshing(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshOnce()
		}
	}
}

func (s *WeatherService) WeatherHandler(w http.ResponseWriter, r *http.Request) {
	snapshot, ok := s.Snapshot()
	if !ok {
		http.Error(w, "weather forecast is not ready", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snapshot)
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	service := NewWeatherService()

	// Warm up before accepting traffic. Startup becomes ~1s slower, зато первая
	// highload-ручка не отдаёт 503 и не вызывает дорогую функцию на каждом запросе.
	service.refreshOnce()

	go service.StartRefreshing(ctx, 10*time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("/weather", service.WeatherHandler)

	server := &http.Server{
		Addr:              "localhost:8000",
		Handler:           mux,
		ReadHeaderTimeout: 2 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
