// SPDX-License-Identifier: GPL-3.0-or-later
package grouping

import (
	"path"
	"strings"
	"unicode"

	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/model"
)

func isManager(p model.Process, name string) bool {
	switch name {
	case "init", "systemd", "containerd-shim-runc-v2", "docker-init", "tini", "dumb-init", "openrc-run", "crond", "gnome-shell", "plasmashell", "xfwm4", "netdata", "kthread", "kthreadd":
		return true
	}
	return strings.Contains(p.Cmdline, "python3") && strings.Contains(p.Cmdline, "bin/yugabyted")
}

// Adapted from apps_pid.c interpreter naming, without filesystem probes. Raw
// comm stays untouched for matching/Function display. Inline scripts (-c/-e)
// never become names; only an executable or script path is used.
func processName(p model.Process) string {
	name := strings.Trim(p.Comm, "()")
	args := commandWords(p.Cmdline)
	if len(args) > 0 {
		executable := path.Base(args[0])
		if strings.HasPrefix(executable, name) && name != "" {
			name = executable
		}
	}
	if len(args) > 1 {
		switch name {
		case "python", "python2", "python3", "sh", "bash", "zsh", "dash", "csh", "tcsh", "ksh", "node", "perl":
			for _, arg := range args[1:] {
				if arg == "-c" || arg == "-e" || arg == "-m" {
					break
				}
				if strings.HasPrefix(arg, "-") {
					continue
				}
				if strings.Contains(arg, "/") || hasScriptExtension(arg) {
					name = path.Base(arg)
				}
				break
			}
		}
	}
	for _, ext := range []string{".sh", ".py", ".pl", ".js"} {
		name = strings.TrimSuffix(name, ext)
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '_'
		}
		return r
	}, name)
	if name == "" {
		return "unknown"
	}
	return name
}
func hasScriptExtension(s string) bool {
	for _, ext := range []string{".sh", ".py", ".pl", ".js"} {
		if strings.HasSuffix(s, ext) {
			return true
		}
	}
	return false
}

// Native snapshots accept either NUL arguments or whitespace-rendered cmdline.
func commandWords(s string) []string {
	if strings.ContainsRune(s, 0) {
		return strings.Split(strings.TrimRight(s, "\x00"), "\x00")
	}
	var out []string
	var word strings.Builder
	var quote rune
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			word.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case unicode.IsSpace(r):
			if word.Len() > 0 {
				out = append(out, word.String())
				word.Reset()
			}
		default:
			word.WriteRune(r)
		}
	}
	if escaped {
		word.WriteRune('\\')
	}
	if word.Len() > 0 {
		out = append(out, word.String())
	}
	return out
}
