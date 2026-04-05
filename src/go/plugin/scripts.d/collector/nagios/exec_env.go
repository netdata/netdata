// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"fmt"
	"maps"
	"os"
	"os/user"
	"runtime"
	"sort"
)

func buildRunEnv(workingDir string, jobEnv map[string]string, macroEnv map[string]string) []string {
	merged := buildExecutionBaselineEnv(workingDir)
	for k, v := range jobEnv {
		merged[k] = replaceMacro(v, macroEnv)
	}
	maps.Copy(merged, macroEnv)

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s=%s", k, merged[k]))
	}
	return out
}

func buildExecutionBaselineEnv(workingDir string) map[string]string {
	if runtime.GOOS == "windows" {
		return buildWindowsExecutionBaselineEnv()
	}
	return buildUnixExecutionBaselineEnv(workingDir)
}

func buildUnixExecutionBaselineEnv(workingDir string) map[string]string {
	env := make(map[string]string)
	setEnvFromProcess(env, "PATH", "PATH")
	setEnvFromProcess(env, "TZ", "TZ")
	setEnvFromProcess(env, "TZDIR", "TZDIR")
	setWorkingDirEnv(env, workingDir)

	tmpdir := os.Getenv("TMPDIR")
	if tmpdir == "" {
		tmpdir = os.TempDir()
	}
	if tmpdir != "" {
		env["TMPDIR"] = tmpdir
	}

	if u, err := user.Current(); err == nil {
		if u.Username != "" {
			env["USER"] = u.Username
			env["LOGNAME"] = u.Username
		}
		if u.HomeDir != "" {
			env["HOME"] = u.HomeDir
		}
	} else {
		setEnvFromProcess(env, "USER", "USER")
		setEnvFromProcess(env, "LOGNAME", "LOGNAME")
		setEnvFromProcess(env, "HOME", "HOME")
	}

	env["SHELL"] = "/bin/sh"
	env["LC_ALL"] = "C"
	return env
}

func buildWindowsExecutionBaselineEnv() map[string]string {
	env := make(map[string]string)
	for _, key := range []string{
		"PATH",
		"TMP",
		"TEMP",
		"USERPROFILE",
		"HOMEDRIVE",
		"HOMEPATH",
		"SystemRoot",
		"WINDIR",
		"ComSpec",
		"PATHEXT",
		"TZ",
	} {
		setEnvFromProcess(env, key, key)
	}
	return env
}

func setEnvFromProcess(dst map[string]string, dstKey, srcKey string) {
	if value := os.Getenv(srcKey); value != "" {
		dst[dstKey] = value
	}
}

func setWorkingDirEnv(dst map[string]string, workingDir string) {
	if workingDir != "" {
		dst["PWD"] = workingDir
		return
	}
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		dst["PWD"] = cwd
	}
}
