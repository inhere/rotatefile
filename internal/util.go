package internal

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gookit/goutil/fsutil"
)

// CompressSuffix the suffix of compressed file.
const CompressSuffix = ".gz"

// AddSuffix2path add suffix to file path.
//
// eg: "/path/to/error.log" => "/path/to/error.{suffix}.log"
func AddSuffix2path(filePath, suffix string) string {
	ext := filepath.Ext(filePath)
	return filePath[:len(filePath)-len(ext)] + "." + suffix + ext
}

// BuildGlobPattern builds a glob pattern for the given logfile. NOTE: use for testing only.
func BuildGlobPattern(logfile string) string {
	return logfile[:len(logfile)-4] + "*"
}

// PrintErrln print error to stderr with a prefix when err is not nil.
func PrintErrln(pfx string, err error) {
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, pfx, err)
	}
}

// CompressFile compress the src file to dst file by gzip.
func CompressFile(srcPath, dstPath string) error {
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
