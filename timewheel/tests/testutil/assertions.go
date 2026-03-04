package testutil

import (
	"fmt"
	"reflect"
	"runtime"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// Assertion 断言助手
type Assertion struct {
	t *testing.T
}

// NewAssertion 创建断言助手
func NewAssertion(t *testing.T) *Assertion {
	return &Assertion{t: t}
}

// Equal 断言相等
func (a *Assertion) Equal(expected, actual interface{}, msgAndArgs ...interface{}) {
	a.t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		a.t.Errorf("Expected %v, got %v. %v", expected, actual, msg(msgAndArgs))
	}
}

// NotEqual 断言不相等
func (a *Assertion) NotEqual(expected, actual interface{}, msgAndArgs ...interface{}) {
	a.t.Helper()
	if reflect.DeepEqual(expected, actual) {
		a.t.Errorf("Expected %v to not equal %v. %v", expected, actual, msg(msgAndArgs))
	}
}

// True 断言为真
func (a *Assertion) True(value bool, msgAndArgs ...interface{}) {
	a.t.Helper()
	if !value {
		a.t.Errorf("Expected true, got false. %v", msg(msgAndArgs))
	}
}

// False 断言为假
func (a *Assertion) False(value bool, msgAndArgs ...interface{}) {
	a.t.Helper()
	if value {
		a.t.Errorf("Expected false, got true. %v", msg(msgAndArgs))
	}
}

// Nil 断言为 nil
func (a *Assertion) Nil(value interface{}, msgAndArgs ...interface{}) {
	a.t.Helper()
	if !isNil(value) {
		a.t.Errorf("Expected nil, got %v. %v", value, msg(msgAndArgs))
	}
}

// NotNil 断言不为 nil
func (a *Assertion) NotNil(value interface{}, msgAndArgs ...interface{}) {
	a.t.Helper()
	if isNil(value) {
		a.t.Errorf("Expected not nil. %v", msg(msgAndArgs))
	}
}

// Error 断言有错误
func (a *Assertion) Error(err error, msgAndArgs ...interface{}) {
	a.t.Helper()
	if err == nil {
		a.t.Errorf("Expected error, got nil. %v", msg(msgAndArgs))
	}
}

// NoError 断言无错误
func (a *Assertion) NoError(err error, msgAndArgs ...interface{}) {
	a.t.Helper()
	if err != nil {
		a.t.Errorf("Expected no error, got %v. %v", err, msg(msgAndArgs))
	}
}

// Contains 断言包含
func (a *Assertion) Contains(s, contains string, msgAndArgs ...interface{}) {
	a.t.Helper()
	if !containsStr(s, contains) {
		a.t.Errorf("Expected %q to contain %q. %v", s, contains, msg(msgAndArgs))
	}
}

// NotContains 断言不包含
func (a *Assertion) NotContains(s, contains string, msgAndArgs ...interface{}) {
	a.t.Helper()
	if containsStr(s, contains) {
		a.t.Errorf("Expected %q to not contain %q. %v", s, contains, msg(msgAndArgs))
	}
}

// GreaterThan 断言大于
func (a *Assertion) GreaterThan(actual, expected interface{}, msgAndArgs ...interface{}) {
	a.t.Helper()
	result := compare(actual, expected)
	if result <= 0 {
		a.t.Errorf("Expected %v > %v. %v", actual, expected, msg(msgAndArgs))
	}
}

// GreaterOrEqual 断言大于等于
func (a *Assertion) GreaterOrEqual(actual, expected interface{}, msgAndArgs ...interface{}) {
	a.t.Helper()
	result := compare(actual, expected)
	if result < 0 {
		a.t.Errorf("Expected %v >= %v. %v", actual, expected, msg(msgAndArgs))
	}
}

// LessThan 断言小于
func (a *Assertion) LessThan(actual, expected interface{}, msgAndArgs ...interface{}) {
	a.t.Helper()
	result := compare(actual, expected)
	if result >= 0 {
		a.t.Errorf("Expected %v < %v. %v", actual, expected, msg(msgAndArgs))
	}
}

// LessOrEqual 断言小于等于
func (a *Assertion) LessOrEqual(actual, expected interface{}, msgAndArgs ...interface{}) {
	a.t.Helper()
	result := compare(actual, expected)
	if result > 0 {
		a.t.Errorf("Expected %v <= %v. %v", actual, expected, msg(msgAndArgs))
	}
}

// Len 断言长度
func (a *Assertion) Len(object interface{}, length int, msgAndArgs ...interface{}) {
	a.t.Helper()
	value := reflect.ValueOf(object)
	if value.Len() != length {
		a.t.Errorf("Expected length %d, got %d. %v", length, value.Len(), msg(msgAndArgs))
	}
}

// Empty 断言为空
func (a *Assertion) Empty(object interface{}, msgAndArgs ...interface{}) {
	a.t.Helper()
	if !isEmpty(object) {
		a.t.Errorf("Expected empty, got %v. %v", object, msg(msgAndArgs))
	}
}

// NotEmpty 断言不为空
func (a *Assertion) NotEmpty(object interface{}, msgAndArgs ...interface{}) {
	a.t.Helper()
	if isEmpty(object) {
		a.t.Errorf("Expected not empty. %v", msg(msgAndArgs))
	}
}

// Panics 断言 panic
func (a *Assertion) Panics(fn func(), msgAndArgs ...interface{}) {
	a.t.Helper()
	defer func() {
		if r := recover(); r == nil {
			a.t.Errorf("Expected panic. %v", msg(msgAndArgs))
		}
	}()
	fn()
}

// NotPanics 断言不 panic
func (a *Assertion) NotPanics(fn func(), msgAndArgs ...interface{}) {
	a.t.Helper()
	defer func() {
		if r := recover(); r != nil {
			a.t.Errorf("Expected no panic, got %v. %v", r, msg(msgAndArgs))
		}
	}()
	fn()
}

// Eventually 断言最终满足条件
func (a *Assertion) Eventually(condition func() bool, waitFor time.Duration, tick time.Duration, msgAndArgs ...interface{}) {
	a.t.Helper()
	timer := time.NewTimer(waitFor)
	defer timer.Stop()

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			a.t.Errorf("Condition not satisfied after %v. %v", waitFor, msg(msgAndArgs))
			return
		case <-ticker.C:
			if condition() {
				return
			}
		}
	}
}

// Never 断言永远不满足条件
func (a *Assertion) Never(condition func() bool, waitFor time.Duration, tick time.Duration, msgAndArgs ...interface{}) {
	a.t.Helper()
	timer := time.NewTimer(waitFor)
	defer timer.Stop()

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			return
		case <-ticker.C:
			if condition() {
				a.t.Errorf("Condition satisfied when it should not. %v", msg(msgAndArgs))
				return
			}
		}
	}
}

// WithinDuration 断言时间差在范围内
func (a *Assertion) WithinDuration(expected, actual, delta time.Duration, msgAndArgs ...interface{}) {
	a.t.Helper()
	diff := expected - actual
	if diff < 0 {
		diff = -diff
	}
	if diff > delta {
		a.t.Errorf("Expected %v within %v of %v, difference is %v. %v",
			actual, delta, expected, diff, msg(msgAndArgs))
	}
}

// 辅助函数

func msg(msgAndArgs []interface{}) string {
	if len(msgAndArgs) == 0 {
		return ""
	}
	return fmt.Sprintf("Message: %v", msgAndArgs)
}

func isNil(value interface{}) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return rv.IsNil()
	}
	return false
}

func containsStr(s, contains string) bool {
	return len(s) >= len(contains) && s[:len(contains)] == contains ||
		len(s) > len(contains) && containsStr(s[1:], contains)
}

func compare(a, b interface{}) int {
	switch a.(type) {
	case int:
		ai := a.(int)
		bi := b.(int)
		if ai < bi {
			return -1
		} else if ai > bi {
			return 1
		}
		return 0
	case int64:
		ai := a.(int64)
		bi := b.(int64)
		if ai < bi {
			return -1
		} else if ai > bi {
			return 1
		}
		return 0
	case float64:
		ai := a.(float64)
		bi := b.(float64)
		if ai < bi {
			return -1
		} else if ai > bi {
			return 1
		}
		return 0
	case time.Duration:
		ai := a.(time.Duration)
		bi := b.(time.Duration)
		if ai < bi {
			return -1
		} else if ai > bi {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func isEmpty(object interface{}) bool {
	if object == nil {
		return true
	}
	rv := reflect.ValueOf(object)
	switch rv.Kind() {
	case reflect.String:
		return rv.String() == ""
	case reflect.Slice, reflect.Array, reflect.Map:
		return rv.Len() == 0
	case reflect.Ptr:
		return rv.IsNil()
	}
	return false
}

// AssertNoGoroutineLeak 断言无 goroutine 泄漏
func AssertNoGoroutineLeak(t *testing.T, options ...goleak.Option) {
	t.Helper()
	if err := goleak.Find(options...); err != nil {
		t.Errorf("Goroutine leak detected: %v", err)
	}
}

// AssertMemoryUsage 断言内存使用
func AssertMemoryUsage(t *testing.T, maxAllocMB uint64) {
	t.Helper()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	if m.Alloc > maxAllocMB*1024*1024 {
		t.Errorf("Memory usage %.2f MB exceeds limit %d MB",
			float64(m.Alloc)/1024/1024, maxAllocMB)
	}
}

// AssertGoroutineCount 断言 goroutine 数量
func AssertGoroutineCount(t *testing.T, max int) {
	t.Helper()
	count := runtime.NumGoroutine()
	if count > max {
		t.Errorf("Goroutine count %d exceeds limit %d", count, max)
	}
}
