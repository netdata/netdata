// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type option struct{ name, value string }

const (
	optInclude         = "include"
	optIncludeToplevel = "include-toplevel"
	optCumulative      = "statistics-cumulative"
	optEnable          = "control-enable"
	optInterface       = "control-interface"
	optPort            = "control-port"
	optUseCert         = "control-use-cert"
	optKeyFile         = "control-key-file"
	optCertFile        = "control-cert-file"
)

func isOptionUsed(opt option) bool {
	switch opt.name {
	case optInclude,
		optIncludeToplevel,
		optCumulative,
		optEnable,
		optInterface,
		optPort,
		optUseCert,
		optKeyFile,
		optCertFile:
		return true
	}
	return false
}

// TODO:
// If also using chroot, using full path names for the included files works, relative pathnames for the included names
// work if the directory where the daemon is started equals its chroot/working directory or is specified before
// the include statement with  directory:  dir.

// Parse parses Unbound configuration files into UnboundConfig.
// It follows logic described in the 'man unbound.conf':
//   - Files can be included using the 'include:' directive. It can appear anywhere, it accepts a single file name as argument.
//   - Processing continues as if the text  from  the included file was copied into the config file at that point.
//   - Wildcards can be used to include multiple files.
//
// It stops processing on any error: syntax error, recursive include, glob matches directory etc.
func Parse(entryPath string) (*UnboundConfig, error) {
	options, err := parse(entryPath, nil)
	if err != nil {
		return nil, err
	}
	return fromOptions(options), nil
}

func parse(filename string, visited map[string]bool) ([]option, error) {
	if visited == nil {
		visited = make(map[string]bool)
	}
	if visited[filename] {
		return nil, fmt.Errorf("'%s' already visited", filename)
	}
	visited[filename] = true

	f, err := open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var options []option
	sc := bufio.NewScanner(f)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		opt, err := parseLine(line)
		if err != nil {
			return nil, fmt.Errorf("file '%s', error on parsing line '%s': %v", filename, line, err)
		}

		if !isOptionUsed(opt) {
			continue
		}

		if opt.name != optInclude && opt.name != optIncludeToplevel {
			options = append(options, opt)
			continue
		}

		filenames, err := globInclude(opt.value)
		if err != nil {
			return nil, err
		}

		for _, name := range filenames {
			opts, err := parse(name, visited)
			if err != nil {
				return nil, err
			}
			options = append(options, opts...)
		}
	}
	return options, nil
}

func globInclude(include string) ([]string, error) {
	if isGlobPattern(include) {
		return filepath.Glob(include)
	}
	return []string{include}, nil
}

func parseLine(line string) (option, error) {
	parts := strings.Split(line, ":")
	if len(parts) < 2 {
		return option{}, errors.New("bad syntax")
	}
	key, value := cleanKeyValue(parts[0], parts[1])
	return option{name: key, value: value}, nil
}

func cleanKeyValue(key, value string) (string, string) {
	if i := strings.IndexByte(value, '#'); i > 0 {
		value = value[:i-1]
	}
	key = strings.TrimSpace(key)
	value = strings.Trim(strings.TrimSpace(value), "\"'")
	return key, value
}

func isGlobPattern(value string) bool {
	magicChars := `*?[`
	if runtime.GOOS != "windows" {
		magicChars = `*?[\`
	}
	return strings.ContainsAny(value, magicChars)
}

func open(filename string) (*os.File, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("'%s' is not a regular file", filename)
	}
	return f, nil
}
