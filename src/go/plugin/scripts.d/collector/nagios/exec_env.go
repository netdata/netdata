// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"fmt"
	"os"
	"os/user"
	"runtime"
	"sort"
)

func buildRunEnv(jobEnv map[string]string, macroEnv map[string]string) []string {
	merged := buildExecutionBaselineEnv()
	for k, v := range jobEnv {
		merged[k] = replaceMacro(v, macroEnv)
	}
	for k, v := range macroEnv {
		merged[k] = v
	}

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

func buildExecutionBaselineEnv() map[string]string {
	if runtime.GOOS == "windows" {
		return buildWindowsExecutionBaselineEnv()
	}
	return buildUnixExecutionBaselineEnv()
}

func buildUnixExecutionBaselineEnv() map[string]string {
	env := make(map[string]string)
	setEnvFromProcess(env, "PATH", "PATH")
	setEnvFromProcess(env, "PWD", "PWD")
	setEnvFromProcess(env, "TZ", "TZ")
	setEnvFromProcess(env, "TZDIR", "TZDIR")

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
