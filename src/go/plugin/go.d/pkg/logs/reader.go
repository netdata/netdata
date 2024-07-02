// SPDX-License-Identifier: GPL-3.0-or-later

package logs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/netdata/netdata/go/plugins/logger"
)

const (
	maxEOF = 60
)

var (
	ErrNoMatchedFile = errors.New("no matched files")
)

// Reader is a log rotate aware Reader
// TODO: better reopen algorithm
// TODO: handle truncate
type Reader struct {
	file          *os.File
	path          string
	excludePath   string
	eofCounter    int
	continuousEOF int
	log           *logger.Logger
}

// Open a file and seek to end of the file.
// path: the shell file name pattern
// excludePath: the shell file name pattern
func Open(path string, excludePath string, log *logger.Logger) (*Reader, error) {
	var err error
	if path, err = filepath.Abs(path); err != nil {
		return nil, err
	}
	if _, err = filepath.Match(path, "/"); err != nil {
		return nil, fmt.Errorf("bad path syntax: %q", path)
	}
	if _, err = filepath.Match(excludePath, "/"); err != nil {
		return nil, fmt.Errorf("bad exclude_path syntax: %q", path)
	}
	r := &Reader{
		path:        path,
		excludePath: excludePath,
		log:         log,
	}

	if err = r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

// CurrentFilename get current opened file name
func (r *Reader) CurrentFilename() string {
	return r.file.Name()
}

func (r *Reader) open() error {
	path := r.findFile()
	if path == "" {
		r.log.Debugf("couldn't find log file, used path: '%s', exclude_path: '%s'", r.path, r.excludePath)
		return ErrNoMatchedFile
	}
	r.log.Debug("open log file: ", path)
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	if _, err = file.Seek(stat.Size(), io.SeekStart); err != nil {
		return err
	}
	r.file = file
	return nil
}

func (r *Reader) Read(p []byte) (n int, err error) {
	n, err = r.file.Read(p)
	if err != nil {
		switch {
		case err == io.EOF:
			err = r.handleEOFErr()
		case errors.Is(err, os.ErrInvalid): // r.file is nil after Close
			err = r.handleInvalidArgErr()
		}
		return
	}
	r.continuousEOF = 0
	return
}

func (r *Reader) handleEOFErr() (err error) {
	err = io.EOF
	r.eofCounter++
	r.continuousEOF++
	if r.eofCounter < maxEOF || r.continuousEOF < 2 {
		return err
	}
	if err2 := r.reopen(); err2 != nil {
		err = err2
	}
	return err
}

func (r *Reader) handleInvalidArgErr() (err error) {
	err = io.EOF
	if err2 := r.reopen(); err2 != nil {
		err = err2
	}
	return err
}

func (r *Reader) Close() (err error) {
	if r == nil || r.file == nil {
		return
	}
	r.log.Debug("close log file: ", r.file.Name())
	err = r.file.Close()
	r.file = nil
	r.eofCounter = 0
	return
}

func (r *Reader) reopen() error {
	r.log.Debugf("reopen, look for: %s", r.path)
	_ = r.Close()
	return r.open()
}

func (r *Reader) findFile() string {
	return find(r.path, r.excludePath)
}

func find(path, exclude string) string {
	return finder{}.find(path, exclude)
}

// TODO: tests
type finder struct{}

func (f finder) find(path, exclude string) string {
	files, _ := filepath.Glob(path)
	if len(files) == 0 {
		return ""
	}

	files = f.filter(files, exclude)
	if len(files) == 0 {
		return ""
	}

	return f.findLastFile(files)
}

func (f finder) filter(files []string, exclude string) []string {
	if exclude == "" {
		return files
	}

	fs := make([]string, 0, len(files))
	for _, file := range files {
		if ok, _ := filepath.Match(exclude, file); ok {
			continue
		}
		fs = append(fs, file)
	}
	return fs
}

// TODO: the logic is probably wrong
func (f finder) findLastFile(files []string) string {
	sort.Strings(files)
	for i := len(files) - 1; i >= 0; i-- {
		stat, err := os.Stat(files[i])
		if err != nil || !stat.Mode().IsRegular() {
			continue
		}
		return files[i]
	}
	return ""
}
