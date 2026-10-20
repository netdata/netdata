// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/command"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type commandCapture struct {
	Args  []string
	Env   []string
	Input string
}

// Re-execute this test binary as an owned helper, including sendsms' fixed argv shape.
func TestMain(m *testing.M) {
	if mode := os.Getenv("NOTIFIER_TEST_COMMAND_HELPER"); mode != "" {
		os.Exit(commandHelper(mode))
	}
	if len(os.Args) > 2 && os.Args[1] == "sns" && os.Args[2] == "publish" {
		os.Exit(snsCommandHelper())
	}
	os.Exit(m.Run())
}

func commandHelper(mode string) int {
	if strings.HasPrefix(mode, "irc-") {
		return ircCommandHelper(mode)
	}
	if mode == "parent" || mode == "early-parent" || mode == "child" {
		conn, err := net.DialTimeout("tcp", os.Getenv("NOTIFIER_TEST_ADDRESS"), 5*time.Second)
		if err != nil {
			return 80
		}
		defer conn.Close()
		if err := json.NewEncoder(conn).Encode(os.Getpid()); err != nil {
			return 81
		}
		if mode == "child" {
			_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
			_, _ = io.Copy(io.Discard, conn)
			return 0
		}
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), "NOTIFIER_TEST_COMMAND_HELPER=child")
		child.Stdin = os.Stdin
		child.Stdout = os.Stdout
		if mode == "early-parent" {
			if err := child.Start(); err != nil {
				return 82
			}
			return 0
		}
		if err := child.Run(); err != nil {
			return 82
		}
		return 0
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return 83
	}
	env := os.Environ()
	slices.Sort(env)
	file, err := os.OpenFile(os.Getenv("NOTIFIER_TEST_CAPTURE"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return 84
	}
	err = json.NewEncoder(file).Encode(commandCapture{Args: os.Args[1:], Env: env, Input: string(input)})
	_ = file.Close()
	if err != nil {
		return 85
	}
	// Enough output to fill a pipe: the runner must discard without copying or logging it.
	for range 1024 {
		_, _ = fmt.Fprint(os.Stdout, strings.Repeat("synthetic-private-value", 64))
		_, _ = fmt.Fprint(os.Stderr, strings.Repeat("synthetic-private-value", 64))
	}
	if mode == "fail" {
		return 7
	}
	return 0
}

func testCommandDestination(t *testing.T, mode string) (map[string]any, string) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	capture := filepath.Join(t.TempDir(), "capture.json")
	return map[string]any{"type": "command", "executable": executable, "env": map[string]string{
		"NOTIFIER_TEST_COMMAND_HELPER": mode, "NOTIFIER_TEST_CAPTURE": capture, "GORACE": "atexit_sleep_ms=0",
	}}, capture
}

func writeCommandConfig(t *testing.T, cfg testConfig) string {
	t.Helper()
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	return writeConfig(t, string(data))
}

func readCommandCaptures(t *testing.T, path string) []commandCapture {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	var result []commandCapture
	decoder := json.NewDecoder(file)
	for {
		var captured commandCapture
		err := decoder.Decode(&captured)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		result = append(result, captured)
	}
	return result
}

func TestRunLocalCommand(t *testing.T) {
	for name, test := range map[string]struct {
		provider, mode, status string
		code                   int
	}{
		"command warning":  {"command", "record", "WARNING", 0},
		"command critical": {"command", "record", "CRITICAL", 0},
		"command clear":    {"command", "record", "CLEAR", 0},
		"command failure":  {"command", "fail", "WARNING", 1},
		"SMS warning":      {"smstools3", "record", "WARNING", 0},
		"SMS critical":     {"smstools3", "record", "CRITICAL", 0},
		"SMS recovery":     {"smstools3", "record", "CLEAR", 0},
		"SMS failure":      {"smstools3", "fail", "WARNING", 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("NOTIFIER_TEST_AMBIENT_SECRET", "must-not-be-inherited")
			t.Setenv("NOTIFIER_TEST_EXPLICIT_SECRET", "synthetic-private-value")
			dst, capture := testCommandDestination(t, test.mode)
			dst["type"] = test.provider
			dst["env"].(map[string]string)["TOKEN"] = "${env:NOTIFIER_TEST_EXPLICIT_SECRET}"
			wantArgs := []string{"space argument", "$(literal)", "${env:LITERAL}", "", "line\nbreak"}
			dst["args"] = wantArgs
			event := testutil.ExpectedEvent()
			event.Status = test.status
			event.Duration, event.NonClearDuration = new(uint32(0)), new(uint32(123))
			input, err := json.Marshal(event)
			require.NoError(t, err)
			wantInput := string(input) + "\n"
			if dst["type"] == "smstools3" {
				dst["to"] = "15005550009"
				delete(dst, "args")
				message := map[string]string{"WARNING": "test-node needs attention: test.chart, Temperature is high = 42.5 C", "CRITICAL": "test-node is critical: test.chart, Temperature is high = 42.5 C", "CLEAR": "test-node recovered: test.chart, Temperature is high"}[test.status]
				wantArgs = []string{"15005550009", message}
				wantInput = ""
			}
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{"target": dst}}
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), []string{"send", "--config", writeCommandConfig(t, cfg), "--destination", "target"}, bytes.NewReader(input), &stdout, &stderr)
			assert.Equal(t, test.code, code, stderr.String())
			assert.Empty(t, stdout.String())
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			assert.NotContains(t, stderr.String(), dst["executable"])
			if test.code != 0 {
				assert.Contains(t, stderr.String(), "exited with status 7")
			}
			wantEnv := []string{"GORACE=atexit_sleep_ms=0", "NOTIFIER_TEST_CAPTURE=" + capture,
				"NOTIFIER_TEST_COMMAND_HELPER=" + test.mode, "PATH=" + commandexec.DefaultPath, "TOKEN=synthetic-private-value"}
			assert.Equal(t, []commandCapture{{Args: wantArgs, Env: wantEnv, Input: wantInput}}, readCommandCaptures(t, capture))
		})
	}
}

func TestRunCommandHelperPrecedence(t *testing.T) {
	for name, test := range map[string]struct {
		mode string
		code int
	}{
		"record SNS-looking arguments": {"record", 0},
		"fail SNS-looking arguments":   {"fail", 1},
	} {
		t.Run(name, func(t *testing.T) {
			dst, capture := testCommandDestination(t, test.mode)
			dst["args"] = []string{"sns", "publish", "literal argument"}
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{"target": dst}}
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), []string{"send", "--config", writeCommandConfig(t, cfg), "--destination", "target"}, strings.NewReader(testutil.ValidEvent), &stdout, &stderr)
			require.Equal(t, test.code, code, stderr.String())
			if test.mode == "fail" {
				assert.Contains(t, stderr.String(), "exited with status 7")
			}
			input, err := json.Marshal(testutil.ExpectedEvent())
			require.NoError(t, err)
			assert.Equal(t, []commandCapture{{
				Args:  []string{"sns", "publish", "literal argument"},
				Env:   []string{"GORACE=atexit_sleep_ms=0", "NOTIFIER_TEST_CAPTURE=" + capture, "NOTIFIER_TEST_COMMAND_HELPER=" + test.mode, "PATH=" + commandexec.DefaultPath},
				Input: string(input) + "\n",
			}}, readCommandCaptures(t, capture))
		})
	}
}

func TestRunCommandSelectionAndFailures(t *testing.T) {
	for _, provider := range []string{"command", "email"} {
		t.Run(provider, func(t *testing.T) {
			for name, test := range map[string]struct {
				selection, mode, secret string
				validate, missing       bool
				code, calls             int
				message                 string
			}{
				"validate never executes or resolves": {validate: true, secret: "${env:NOTIFIER_TEST_MISSING_COMMAND_SECRET}", message: "configuration is valid"},
				"unselected never resolves":           {selection: "silent", secret: "${env:NOTIFIER_TEST_MISSING_COMMAND_SECRET}"},
				"any success and deduplication":       {selection: "mixed", calls: 1, message: "1 succeeded, 1 failed"},
				"all failures":                        {selection: "mixed", mode: "fail", calls: 1, code: 1, message: "all attempted destinations failed"},
				"missing executable":                  {missing: true, code: 1, message: "could not start command"},
				"missing secret":                      {secret: "${env:NOTIFIER_TEST_MISSING_COMMAND_SECRET}", code: 1, message: "not set"},
				"resolved NUL":                        {secret: "FILE_NUL", code: 1, message: "NUL"},
			} {
				t.Run(name, func(t *testing.T) {
					dst, capture := testCommandDestination(t, test.mode)
					dst["type"] = provider
					if provider == "email" {
						dst["recipients"] = []string{"root"}
					}
					if test.mode == "" {
						dst["env"].(map[string]string)["NOTIFIER_TEST_COMMAND_HELPER"] = "record"
					}
					if test.secret != "" {
						dst["env"].(map[string]string)["TOKEN"] = test.secret
						if test.secret == "FILE_NUL" {
							secret := filepath.Join(t.TempDir(), "synthetic-secret")
							require.NoError(t, os.WriteFile(secret, []byte("synthetic-private-value\x00"), 0600))
							dst["env"].(map[string]string)["TOKEN"] = "${file:" + secret + "}"
						}
					}
					bad := map[string]any{"type": "command", "executable": filepath.Join(t.TempDir(), "synthetic-private-value")}
					if test.missing {
						dst["executable"] = bad["executable"]
					}
					cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{"target": dst, "bad": bad}, Routing: notifier.Routing{Roles: map[string][]string{"mixed": {"bad", "target", "target"}}}}
					args := []string{"send", "--config", writeCommandConfig(t, cfg)}
					if test.validate {
						args[0] = "validate"
					} else if test.selection != "" {
						args = append(args, "--role", test.selection)
					} else {
						args = append(args, "--destination", "target")
					}
					var stdout, stderr bytes.Buffer
					assert.Equal(t, test.code, Run(context.Background(), args, strings.NewReader(testutil.ValidEvent), &stdout, &stderr), stderr.String())
					assert.Contains(t, stdout.String()+stderr.String(), test.message)
					assert.NotContains(t, stdout.String()+stderr.String(), "synthetic-private-value")
					if test.calls > 0 {
						assert.Len(t, readCommandCaptures(t, capture), test.calls)
					} else {
						_, err := os.Stat(capture)
						assert.ErrorIs(t, err, os.ErrNotExist)
					}
				})
			}
		})
	}
}

func TestRunCommandCancellation(t *testing.T) {
	for name, test := range map[string]struct {
		deadline  bool
		large     bool
		earlyExit bool
		email     bool
		irc       bool
	}{"cancel": {}, "timeout": {deadline: true}, "cancel blocked stdin copy": {large: true}, "early exit bounds inherited stdin": {large: true, earlyExit: true},
		"irc early exit bounds inherited pipes": {irc: true, earlyExit: true},
		"irc cancel":                            {irc: true}, "irc timeout": {irc: true, deadline: true},
		"email cancel": {email: true}, "email timeout": {email: true, deadline: true}, "email blocked stdin": {email: true, large: true}} {
		t.Run(name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			require.NoError(t, listener.(*net.TCPListener).SetDeadline(time.Now().Add(5*time.Second)))
			dst, _ := testCommandDestination(t, "parent")
			if test.irc {
				dst["type"] = "irc"
				dst["host"] = "irc.example.com"
				dst["nickname"] = "notify"
				dst["realname"] = "Netdata alerts"
				dst["channel"] = "#alerts"
			}
			if test.email {
				dst["type"] = "email"
				dst["recipients"] = []string{"root"}
			}
			if test.earlyExit {
				dst["env"].(map[string]string)["NOTIFIER_TEST_COMMAND_HELPER"] = "early-parent"
			}
			dst["env"].(map[string]string)["NOTIFIER_TEST_ADDRESS"] = listener.Addr().String()
			cfg := testConfig{Version: 1, Destinations: map[string]map[string]any{"target": dst}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			event := testutil.ExpectedEvent()
			if test.large {
				event.Info = strings.Repeat("x", 2<<20)
			}
			input, err := json.Marshal(event)
			require.NoError(t, err)
			timeout := "10s"
			if test.deadline {
				timeout = "2s"
			}
			args := []string{"send", "--config", writeCommandConfig(t, cfg), "--destination", "target", "--timeout", timeout}
			var stdout, stderr bytes.Buffer
			done := make(chan int, 1)
			go func() { done <- Run(ctx, args, bytes.NewReader(input), &stdout, &stderr) }()
			var connections []net.Conn
			var pids []int
			for range 2 {
				conn, err := listener.Accept()
				require.NoError(t, err)
				defer conn.Close()
				require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
				var pid int
				require.NoError(t, json.NewDecoder(conn).Decode(&pid))
				connections = append(connections, conn)
				pids = append(pids, pid)
			}
			if !test.deadline && !test.earlyExit {
				cancel()
			}
			select {
			case code := <-done:
				assert.Equal(t, 1, code)
			case <-time.After(5 * time.Second):
				t.Fatal("Run did not complete command cleanup")
			}
			// Parent is reaped before Run returns; the descendant's socket closes even if init has not reaped it yet.
			assert.ErrorIs(t, syscall.Kill(pids[0], 0), syscall.ESRCH)
			if test.earlyExit {
				// This deliberately violates the foreground contract. Closing the owned socket releases the child.
				if test.irc {
					assert.Contains(t, stderr.String(), "command protocol did not complete")
				} else {
					assert.Contains(t, stderr.String(), "command input did not complete")
				}
				return
			}
			for _, conn := range connections {
				buffer := make([]byte, 1)
				_, err := conn.Read(buffer)
				assert.ErrorIs(t, err, io.EOF)
			}
			assert.Empty(t, stdout.String())
			if test.deadline {
				assert.Contains(t, stderr.String(), "timed out")
			} else {
				assert.Contains(t, stderr.String(), "canceled")
			}
		})
	}
}

func TestClosedCommandAdmission(t *testing.T) {
	dst, capture := testCommandDestination(t, "record")
	processes := &commandexec.Runner{}
	processes.CloseAndWait()
	sender, err := command.New(command.Config{Executable: dst["executable"].(string), Env: dst["env"].(map[string]string)}, processes)
	require.NoError(t, err)
	require.ErrorIs(t, sender.Send(context.Background(), testutil.ExpectedEvent()), context.Canceled)
	_, err = os.Stat(capture)
	assert.ErrorIs(t, err, os.ErrNotExist)
}
