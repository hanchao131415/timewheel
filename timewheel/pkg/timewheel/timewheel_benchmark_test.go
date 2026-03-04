package timewheel

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func BenchmarkAddTask(b *testing.B) {
	tw, _ := New(WithSlotNum(1000), WithInterval(10*time.Millisecond))
	tw.Start()
	defer tw.Stop()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			task := &Task{
				ID:          fmt.Sprintf("task-%d", i),
				Mode:        TaskModeRepeated,
				Interval:    100 * time.Millisecond,
				Description: fmt.Sprintf("Task %d", i),
				Run: func(ctx context.Context) AlarmResult {
					return AlarmResult{Value: 100, Threshold: 50, IsFiring: true}
				},
			}
			tw.AddTask(task)
			i++
		}
	})
}

func BenchmarkGetTask(b *testing.B) {
	tw, _ := New(WithSlotNum(1000), WithInterval(10*time.Millisecond), WithCache(true))
	tw.Start()
	defer tw.Stop()

	for i := 0; i < 10000; i++ {
		task := &Task{
			ID:          fmt.Sprintf("task-%d", i),
			Mode:        TaskModeRepeated,
			Interval:    100 * time.Millisecond,
			Description: fmt.Sprintf("Task %d", i),
			Run: func(ctx context.Context) AlarmResult {
				return AlarmResult{}
			},
		}
		tw.AddTask(task)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tw.GetTask(fmt.Sprintf("task-%d", i%10000))
			i++
		}
	})
}

func BenchmarkConcurrentAddRemove(b *testing.B) {
	tw, _ := New(WithSlotNum(1000), WithInterval(1*time.Millisecond), WithCache(true))
	tw.Start()
	defer tw.Stop()

	var wg sync.WaitGroup

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			task := &Task{
				ID:          fmt.Sprintf("task-%d", idx),
				Mode:        TaskModeRepeated,
				Interval:    100 * time.Millisecond,
				Description: fmt.Sprintf("Task %d", idx),
				Run: func(ctx context.Context) AlarmResult {
					return AlarmResult{}
				},
			}
			tw.AddTask(task)
			tw.RemoveTask(task.ID)
		}(i)
	}

	wg.Wait()
}

func BenchmarkCacheHitRate(b *testing.B) {
	cache := NewTaskCache(100000)

	for i := 0; i < 50000; i++ {
		cache.Set(fmt.Sprintf("task-%d", i), &taskSlot{})
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			cache.Get(fmt.Sprintf("task-%d", i%100000))
			i++
		}
	})

	stats := cache.GetStats()
	b.Logf("Cache stats: %+v", stats)
}

func BenchmarkStringPool(b *testing.B) {
	pool := NewStringPool()
	strings := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		strings[i] = fmt.Sprintf("task-%d-description", i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			pool.Get(strings[i%1000])
			i++
		}
	})
}
