// Package rotatefile provides simple file rotation, compression and cleanup.
package rotatefile

import (
	"io"
	"sync"
	"time"

	"github.com/gookit/goutil/timex"
	"github.com/gookit/goutil/x/basefn"
)

// RotateWriter interface
type RotateWriter interface {
	io.WriteCloser
	Clean() error
	Flush() error
	Rotate() error
	Sync() error
}

const (
	// OneMByte size
	OneMByte uint64 = 1024 * 1024

	// DefaultMaxSize of a log file. default is 20M.
	DefaultMaxSize = 20 * OneMByte
	// DefaultBackNum default backup numbers for old files.
	DefaultBackNum uint = 20
	// DefaultBackTime default backup time for old files. default keeps a week.
	DefaultBackTime uint = 24 * 7
)

// MockClocker mock clock for test.
//
// NOTE: it is concurrency-safe, since the mocked time may be advanced by the
// test goroutine while a background goroutine (eg. async cleaner) reads it.
type MockClocker struct {
	mu sync.RWMutex
	tt time.Time
}

// NewMockClock create a mock time instance from datetime string.
func NewMockClock(datetime string) *MockClocker {
	nt := basefn.Must(timex.FromString(datetime))
	return &MockClocker{tt: nt.Time}
}

// Now get current time.
func (mt *MockClocker) Now() time.Time {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.tt
}

// Add progresses time by the given duration.
func (mt *MockClocker) Add(d time.Duration) {
	mt.mu.Lock()
	mt.tt = mt.tt.Add(d)
	mt.mu.Unlock()
}

// Datetime returns the current time in the format "2006-01-02 15:04:05".
func (mt *MockClocker) Datetime() string {
	mt.mu.RLock()
	defer mt.mu.RUnlock()
	return mt.tt.Format("2006-01-02 15:04:05")
}
