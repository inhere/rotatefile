// Command filecleaner cleans old/expired log or backup files by patterns,
// configured via a JSON file. It is built on rotatefile.FilesClear.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/gookit/goutil/cflag"
	"github.com/gookit/goutil/cliutil"
	"github.com/gookit/goutil/jsonutil"
	"github.com/gookit/goutil/timex"
	"github.com/gookit/rotatefile"
)

// jobConfig is one clean job in the config file.
//
// NOTE: durations (time_unit/check_interval) are human-readable strings such
// as "1h", "24h", "60s" — easier to write than the raw nanoseconds that
// time.Duration would require in JSON.
type jobConfig struct {
	Patterns       []string `json:"patterns"`
	BackupNum      uint     `json:"backup_num"`
	BackupTime     uint     `json:"backup_time"`
	TimeUnit       string   `json:"time_unit"`      // eg: "1h", "24h". default "1h"
	CheckInterval  string   `json:"check_interval"` // eg: "60s". default "60s"
	Compress       bool     `json:"compress"`
	Recursive      bool     `json:"recursive"`
	RemoveEmptyDir bool     `json:"remove_empty_dir"`
	IgnoreError    bool     `json:"ignore_error"`
}

// fileConfig is the top-level config file structure.
type fileConfig struct {
	Jobs []jobConfig `json:"jobs"`
}

// build info, can be injected via -ldflags at build time (see Makefile).
var (
	Version   = "0.1.0"
	GitCommit = ""
	GoVersion = ""
	BuildTime = ""
)

// fullVersion builds a version string. extra build info is appended only when
// injected via -ldflags, so a plain `go build` still shows just the version.
func fullVersion() string {
	s := Version
	if GitCommit != "" {
		s += ", " + GitCommit
	}
	if BuildTime != "" {
		s += ", " + BuildTime
	}
	// if GoVersion != "" {
	// 	s += " go" + GoVersion
	// }
	return s
}

var showVer bool
var opts = struct {
	config string
	daemon bool
	dryRun bool
}{}

func main() {
	c := cflag.New(func(c *cflag.CFlags) {
		c.Desc = "Clean old/expired log or backup files by patterns, config via JSON file"
		c.Version = Version
	})

	c.StringVar(&opts.config, "config", "filecleaner.json", "the JSON config file path;;c")
	c.BoolVar(&opts.daemon, "daemon", false, "run as daemon and clean periodically;;d")
	c.BoolVar(&opts.dryRun, "dry-run", false, "only print the files to be removed, do not delete")
	c.BoolVar(&showVer, "version", false, "print the verion;;V")

	c.Example = "{{cmd}} -c filecleaner.json\n  {{cmd}} --dry-run -c filecleaner.json\n  {{cmd}} --daemon -c filecleaner.json"
	c.Func = run
	c.AfterFlagParse = func(c *cflag.CFlags) bool {
		if showVer {
			fmt.Println(fullVersion())
			return false
		}
		return true
	}

	// use Parse (not MustParse) so a failure exits non-zero for scripts/cron.
	if err := c.Parse(nil); err != nil {
		cliutil.Errorln("ERROR:", err)
		os.Exit(1)
	}
}

func run(_ *cflag.CFlags) error {
	var fc fileConfig
	if err := jsonutil.ReadFile(opts.config, &fc); err != nil {
		return fmt.Errorf("read config file %q error: %w", opts.config, err)
	}
	if len(fc.Jobs) == 0 {
		return fmt.Errorf("no clean jobs found in config file: %s", opts.config)
	}

	cleaners := make([]*rotatefile.FilesClear, 0, len(fc.Jobs))
	for i, job := range fc.Jobs {
		cc, err := buildCConfig(job)
		if err != nil {
			return fmt.Errorf("job#%d config error: %w", i+1, err)
		}
		cc.DryRun = opts.dryRun
		cleaners = append(cleaners, rotatefile.NewFilesClear().WithConfig(cc))
	}

	if opts.dryRun {
		cliutil.Warnln("[dry-run] no file will be actually removed")
	}
	if opts.daemon {
		return runDaemon(cleaners)
	}
	return runOnce(cleaners)
}

// buildCConfig maps a job config to rotatefile.CConfig.
//
// NOTE: it starts from NewCConfig() only to get a valid TimeClock and sane
// TimeUnit/CheckInterval defaults; BackupNum/BackupTime are taken verbatim from
// the file (0 means "no limit" for that dimension).
func buildCConfig(job jobConfig) (*rotatefile.CConfig, error) {
	cc := rotatefile.NewCConfig()
	cc.Patterns = job.Patterns
	cc.BackupNum = job.BackupNum
	cc.BackupTime = job.BackupTime
	cc.Compress = job.Compress
	cc.Recursive = job.Recursive
	cc.RemoveEmptyDir = job.RemoveEmptyDir
	cc.IgnoreError = job.IgnoreError

	if job.TimeUnit != "" {
		dur, err := timex.ToDuration(job.TimeUnit)
		if err != nil {
			return nil, fmt.Errorf("invalid time_unit %q: %w", job.TimeUnit, err)
		}
		cc.TimeUnit = dur
	}
	if job.CheckInterval != "" {
		dur, err := timex.ToDuration(job.CheckInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid check_interval %q: %w", job.CheckInterval, err)
		}
		cc.CheckInterval = dur
	}
	return cc, nil
}

// runOnce runs each cleaner a single time.
func runOnce(cleaners []*rotatefile.FilesClear) error {
	for i, fc := range cleaners {
		cliutil.Infoln("run clean job", i+1, "- patterns:", fc.Config().Patterns)
		if err := fc.Clean(); err != nil {
			return fmt.Errorf("job#%d clean error: %w", i+1, err)
		}
	}
	cliutil.Successln("file clean done")
	return nil
}

// runDaemon runs each cleaner as a daemon and blocks until SIGINT/SIGTERM.
func runDaemon(cleaners []*rotatefile.FilesClear) error {
	stopSig := make(chan os.Signal, 1)
	signal.Notify(stopSig, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup
	for i, fc := range cleaners {
		wg.Add(1)
		go fc.DaemonClean(func() { wg.Done() })
		cliutil.Infoln("started daemon clean job", i+1, "- patterns:", fc.Config().Patterns)
	}

	cliutil.Infoln("filecleaner daemon running, press Ctrl+C to stop")
	<-stopSig

	cliutil.Warnln("stopping daemon ...")
	for _, fc := range cleaners {
		fc.StopDaemon()
	}
	wg.Wait()
	cliutil.Successln("daemon stopped")
	return nil
}
