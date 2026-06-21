package rotatefile

import (
	"os"
	"sort"
	"sync"
	"time"

	"github.com/gookit/goutil/errorx"
	"github.com/gookit/goutil/fsutil"
	"github.com/gookit/rotatefile/internal"
)

const defaultCheckInterval = 60 * time.Second

// CConfig struct for clean files
type CConfig struct {
	// BackupNum max number for keep old files.
	//
	// 0 is not limit, default is 20.
	BackupNum uint `json:"backup_num" yaml:"backup_num"`

	// BackupTime max time for keep old files, unit is TimeUnit.
	//
	// 0 is not limit, default is a week.
	BackupTime uint `json:"backup_time" yaml:"backup_time"`

	// Compress determines if the rotated log files should be compressed using gzip.
	// The default is not to perform compression.
	Compress bool `json:"compress" yaml:"compress"`

	// Patterns dir path with filename match patterns.
	//
	// eg: ["/tmp/error.log.*", "/path/to/info.log.*", "/path/to/dir/*"]
	Patterns []string `json:"patterns" yaml:"patterns"`

	// TimeClock for clean files
	TimeClock Clocker

	// TimeUnit for BackupTime. default is hours: time.Hour
	TimeUnit time.Duration `json:"time_unit" yaml:"time_unit"`

	// CheckInterval for clean files on daemon run. default is 60s.
	CheckInterval time.Duration `json:"check_interval" yaml:"check_interval"`

	// IgnoreError ignore remove file error, continue to clean other files.
	IgnoreError bool `json:"ignore_error" yaml:"ignore_error"`

	// RotateMode for rotate split files TODO
	//  - copy+cut: copy contents then truncate file
	//	- rename : rename file(use for like PHP-FPM app)
	// RotateMode RotateMode `json:"rotate_mode" yaml:"rotate_mode"`
}

// CConfigFunc for clean config
type CConfigFunc func(c *CConfig)

// AddDirPath for clean, will auto append * for match all files
func (c *CConfig) AddDirPath(dirPaths ...string) *CConfig {
	for _, dirPath := range dirPaths {
		if !fsutil.IsDir(dirPath) {
			continue
		}
		c.Patterns = append(c.Patterns, dirPath+"/*")
	}
	return c
}

// AddPattern for clean. eg: "/tmp/error.log.*"
func (c *CConfig) AddPattern(patterns ...string) *CConfig {
	c.Patterns = append(c.Patterns, patterns...)
	return c
}

// WithConfigFn for custom settings
func (c *CConfig) WithConfigFn(fns ...CConfigFunc) *CConfig {
	for _, fn := range fns {
		if fn != nil {
			fn(c)
		}
	}
	return c
}

// NewCConfig instance
func NewCConfig() *CConfig {
	return &CConfig{
		BackupNum:  DefaultBackNum,
		BackupTime: DefaultBackTime,
		TimeClock:  DefaultTimeClockFn,
		TimeUnit:   time.Hour,
		// check interval time
		CheckInterval: defaultCheckInterval,
	}
}

// FilesClear multi files by time.
//
// use for rotate and clear other program produce log files
type FilesClear struct {
	// mu guards quitDaemon (created in DaemonClean, read/closed in StopDaemon)
	mu  sync.Mutex
	cfg *CConfig

	quitDaemon chan struct{}
}

// NewFilesClear instance
func NewFilesClear(fns ...CConfigFunc) *FilesClear {
	cfg := NewCConfig().WithConfigFn(fns...)
	return &FilesClear{cfg: cfg}
}

// Config get
func (r *FilesClear) Config() *CConfig {
	return r.cfg
}

// WithConfig for custom set config
func (r *FilesClear) WithConfig(cfg *CConfig) *FilesClear {
	r.cfg = cfg
	return r
}

// WithConfigFn for custom settings
func (r *FilesClear) WithConfigFn(fns ...CConfigFunc) *FilesClear {
	r.cfg.WithConfigFn(fns...)
	return r
}

//
// ---------------------------------------------------------------------------
// clean backup files
// ---------------------------------------------------------------------------
//

// StopDaemon for stop daemon clean
func (r *FilesClear) StopDaemon() {
	r.mu.Lock()
	q := r.quitDaemon
	r.mu.Unlock()

	if q == nil {
		panic("cannot quit daemon, please call DaemonClean() first")
	}
	close(q)
}

// DaemonClean daemon clean old files by config
//
// NOTE: this method will block current goroutine
//
// Usage:
//
//	fc := rotatefile.NewFilesClear(nil)
//	fc.WithConfigFn(func(c *rotatefile.CConfig) {
//		c.AddDirPath("./testdata")
//	})
//
//	wg := sync.WaitGroup{}
//	wg.Add(1)
//
//	// start daemon
//	go fc.DaemonClean(func() {
//		wg.Done()
//	})
//
//	// wait for stop
//	wg.Wait()
func (r *FilesClear) DaemonClean(onStop func()) {
	if r.cfg.BackupNum == 0 && r.cfg.BackupTime == 0 {
		panic("clean: backupNum and backupTime are both 0")
	}

	r.mu.Lock()
	r.quitDaemon = make(chan struct{})
	quit := r.quitDaemon
	r.mu.Unlock()

	// fallback to default to avoid time.NewTicker panic on interval <= 0
	interval := r.cfg.CheckInterval
	if interval <= 0 {
		interval = defaultCheckInterval
	}

	tk := time.NewTicker(interval)
	defer tk.Stop()

	// do an initial clean immediately on start
	internal.PrintErrln("files-clear: cleanup old files error:", r.Clean())

	for {
		select {
		case <-quit:
			if onStop != nil {
				onStop()
			}
			return
		case <-tk.C: // do cleaning
			internal.PrintErrln("files-clear: cleanup old files error:", r.Clean())
		}
	}
}

// Clean old files by config
func (r *FilesClear) Clean() error {
	if r.cfg.BackupNum == 0 && r.cfg.BackupTime == 0 {
		return errorx.Err("clean: backupNum and backupTime are both 0")
	}

	// clear by time, can also clean by number
	for _, filePattern := range r.cfg.Patterns {
		if err := r.cleanByPattern(filePattern); err != nil {
			return err
		}
	}
	return nil
}

// CleanByPattern clean files by pattern
func (r *FilesClear) cleanByPattern(filePattern string) (err error) {
	// backupDur > 0 means clean by mod-time is enabled. compute on each call so
	// config changes (BackupTime/TimeUnit) take effect and avoid shared state.
	var backupDur time.Duration
	if r.cfg.BackupTime > 0 {
		backupDur = time.Duration(r.cfg.BackupTime) * r.cfg.TimeUnit
	}

	oldFiles := make([]fsutil.FileInfo, 0, 8)
	cutTime := r.cfg.TimeClock.Now().Add(-backupDur)

	// find and clean expired files
	err = fsutil.GlobWithFunc(filePattern, func(filePath string) error {
		stat, err := os.Stat(filePath)
		if err != nil {
			return err
		}

		// not handle subdir TODO: support subdir
		if stat.IsDir() {
			return nil
		}

		// no time limit, or not expired: collect for clean by number.
		// NOTE: when backupDur<=0 we must NOT treat files as expired, otherwise
		// all files would be wrongly removed (cutTime would be "now").
		if backupDur <= 0 || stat.ModTime().After(cutTime) {
			oldFiles = append(oldFiles, fsutil.NewFileInfo(filePath, stat))
			return nil
		}

		// remove expired file
		return r.remove(filePath)
	})

	// scan error: do not continue to clean by number, and do not swallow it.
	if err != nil {
		return errorx.Wrap(err, "files-clear: scan files error")
	}

	// clear by backup number.
	backNum := int(r.cfg.BackupNum)
	remNum := len(oldFiles) - backNum

	if backNum > 0 && remNum > 0 {
		// sort by mod-time, oldest at first.
		sort.Sort(fsutil.FileInfos(oldFiles))

		for idx := 0; idx < len(oldFiles); idx++ {
			if err = r.remove(oldFiles[idx].Path()); err != nil {
				break
			}

			remNum--
			if remNum == 0 {
				break
			}
		}
	}
	return
}

func (r *FilesClear) remove(filePath string) error {
	err := os.Remove(filePath)
	// ignore remove error, continue to clean other files
	if err != nil && r.cfg.IgnoreError {
		internal.PrintErrln("files-clear: remove file error (ignored):", err)
		return nil
	}
	return err
}
