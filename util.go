package rotatefile

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/gookit/goutil"
	"github.com/gookit/goutil/fsutil"
	"github.com/gookit/goutil/timex"
)

const compressSuffix = ".gz"

func printErrln(pfx string, err error) {
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, pfx, err)
	}
}

func compressFile(srcPath, dstPath string) error {
	srcFile, err := os.OpenFile(srcPath, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// create and open a gz file
	gzFile, err := fsutil.OpenTruncFile(dstPath)
	if err != nil {
		return err
	}
	defer gzFile.Close()

	srcSt, err := srcFile.Stat()
	if err != nil {
		return err
	}

	zw := gzip.NewWriter(gzFile)
	zw.Name = srcSt.Name()
	zw.ModTime = srcSt.ModTime()

	// do copy
	if _, err = io.Copy(zw, srcFile); err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}

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
	nt := goutil.Must(timex.FromString(datetime))
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
