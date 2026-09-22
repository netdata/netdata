// SPDX-License-Identifier: GPL-3.0-or-later

// Package legacycustom runs trusted legacy functions after native configuration and routing.
package legacycustom

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"maps"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/legacyconfig"
)

//go:embed helpers.sh
var helpers string

//go:embed presentation.sh
var presentationScript string

type Sender struct {
	executable string
	script     string
	runner     *commandexec.Runner
}

// New prepares one notification's literal state. Only fixed probes and syntax checks
// run here; custom code executes in Send, after the complete delivery plan is valid.
func New(ctx context.Context, settings legacyconfig.Settings, recipients []string, e event.Event, runner *commandexec.Runner) (*Sender, error) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return nil, errors.New("custom delivery is currently supported only on Linux and macOS")
	}
	if settings.Functions["custom_sender"] == "" {
		return nil, errors.New("custom_sender function is required")
	}
	for name, source := range settings.Functions {
		if strings.HasPrefix(name, "_netdata_custom_") {
			return nil, errors.New("custom function uses a reserved runtime name")
		}
		body, err := functionBody(name, source)
		if err != nil {
			return nil, err
		}
		if name == "custom_sender" && stockPlaceholder(body) {
			return nil, errors.New("custom_sender is the unconfigured stock placeholder; define a function that sends notifications")
		}
	}
	values := maps.Clone(settings.Variables)
	for name, value := range values {
		if reservedVariable(name) {
			return nil, errors.New("custom settings contain a reserved shell variable; use ordinary scalar names and absolute executable paths")
		}
		if !commandexec.ValidEnvironmentName(name) || strings.ContainsRune(value, 0) {
			return nil, errors.New("custom settings contain an invalid scalar name or NUL")
		}
	}
	executable, err := executablePath(values["bash"], "bash", true)
	if err != nil {
		return nil, err
	}
	values["bash"] = executable
	values["curl"], err = executablePath(values["curl"], "curl", false)
	if err != nil {
		return nil, err
	}
	values["goto_url"] = e.URL
	values["date_utc"] = e.Timestamp.UTC().Format(time.RFC3339)
	if values["images_base_url"] == "" {
		values["images_base_url"] = "https://registry.my-netdata.io"
	}
	values["to_custom"] = strings.Join(recipients, " ")
	script, err := sourceScript(values, settings.Recipients, settings.Functions)
	if err != nil {
		return nil, err
	}
	if runner == nil {
		return nil, errors.New("custom delivery requires a process runner")
	}
	// Check the interpreter without feeding it any configuration or function source.
	err = runner.RunSession(ctx, executable, []string{"--noprofile", "--norc", "-s"}, environment(), func(output io.Reader, input io.WriteCloser) error {
		if _, err := io.WriteString(input, "if (( BASH_VERSINFO[0] >= 4 )); then printf 'ready\\n'; fi\n"); err != nil {
			return errors.New("could not check Bash version")
		}
		if err := input.Close(); err != nil {
			return errors.New("could not check Bash version")
		}
		answer, err := io.ReadAll(io.LimitReader(output, 16))
		if err != nil || string(answer) != "ready\n" {
			return errors.New("Bash version 4 or later is required")
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("Bash preflight: %w", err)
	}
	if err := runner.Run(ctx, executable, []string{"--noprofile", "--norc", "-n", "-s"}, environment(), strings.NewReader(script)); err != nil {
		return nil, fmt.Errorf("custom function syntax is not supported by the configured Bash: %w", err)
	}
	return &Sender{executable: executable, script: script, runner: runner}, nil
}

// Send executes the notification captured by New; the public event is not serialized.
func (s *Sender) Send(ctx context.Context, _ event.Event) error {
	return s.runner.RunWithPrivateHome(ctx, s.executable, []string{"--noprofile", "--norc", "-s"}, environment(), strings.NewReader(s.script))
}

func environment() []string { return []string{"PATH=" + commandexec.DefaultPath, "LC_ALL=C"} }

func executablePath(configured, name string, required bool) (string, error) {
	path := configured
	if path == "" {
		var err error
		path, err = exec.LookPath(name)
		if err != nil {
			if !required {
				return "", nil
			}
			return "", fmt.Errorf("%s executable is required; configure an absolute path", name)
		}
	}
	if err := commandexec.ValidateOptions(path, nil, nil); err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return path, nil
}

func reservedVariable(name string) bool {
	for _, prefix := range []string{"BASH", "LC_", "HIST", "COMP_", "READLINE_", "_netdata_custom_"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	switch name {
	case "_", "SHELLOPTS", "ENV", "IFS", "PATH", "HOME", "SHELL", "CDPATH", "GLOBIGNORE", "FIGNORE", "LANG", "LANGUAGE", "UID", "EUID", "PPID", "PIPESTATUS", "DIRSTACK", "FUNCNAME", "GROUPS", "LINENO", "OPTARG", "OPTIND", "OPTERR", "POSIXLY_CORRECT", "PS0", "PS1", "PS2", "PS3", "PS4", "PROMPT_COMMAND", "RANDOM", "SRANDOM", "SECONDS", "EPOCHSECONDS", "EPOCHREALTIME", "PWD", "OLDPWD", "SHLVL", "COLUMNS", "LINES", "TMOUT", "MAILCHECK", "IGNOREEOF", "PROMPT_DIRTRIM", "COMPREPLY", "COPROC", "MAPFILE":
		return true
	}
	return false
}

func quote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func sourceScript(values map[string]string, recipients map[string]map[string]string, functions map[string]string) (string, error) {
	var script strings.Builder
	// All data precedes function declarations, so helper overrides cannot affect handoff.
	for _, name := range slices.Sorted(maps.Keys(values)) {
		if strings.ContainsRune(values[name], 0) {
			return "", errors.New("custom context contains NUL")
		}
		fmt.Fprintf(&script, "%s=%s || exit 1\n", name, quote(values[name]))
	}
	for _, name := range slices.Sorted(maps.Keys(recipients)) {
		if !commandexec.ValidEnvironmentName(name) || !strings.HasPrefix(name, "role_recipients_") {
			return "", errors.New("invalid custom recipient map name")
		}
		fmt.Fprintf(&script, "declare -A %s=(", name)
		for _, key := range slices.Sorted(maps.Keys(recipients[name])) {
			value := recipients[name][key]
			if key == "" || strings.ContainsRune(key+value, 0) {
				return "", errors.New("custom recipient map contains an empty key or NUL")
			}
			fmt.Fprintf(&script, "[%s]=%s ", quote(key), quote(value))
		}
		script.WriteString(") || exit 1\n")
	}
	script.WriteString("readonly -a _netdata_custom_curl_options=(")
	for _, option := range strings.FieldsFunc(values["curl_options"], func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' }) {
		script.WriteString(quote(option) + " ")
	}
	script.WriteString(") || exit 1\n")
	script.WriteString(helpers)
	script.WriteByte('\n')
	script.WriteString(presentationScript)
	script.WriteByte('\n')
	for _, name := range slices.Sorted(maps.Keys(functions)) {
		script.WriteString(functions[name])
		script.WriteByte('\n')
	}
	// Last command preserves the function's status; children see EOF, not more shell source.
	script.WriteString("custom_sender " + quote(values["to_custom"]) + "\n")
	return script.String(), nil
}
