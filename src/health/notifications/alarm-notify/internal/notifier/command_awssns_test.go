// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Select the helper via fixed SNS argv, leaving the production credential environment unchanged.
func snsCommandHelper() int {
	dir := filepath.Dir(os.Args[0])
	home := os.Getenv("HOME")
	info, err := os.Stat(home)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return 80
	}
	if err := os.MkdirAll(filepath.Join(home, ".aws", "cli", "cache"), 0700); err != nil {
		return 81
	}
	if err := os.WriteFile(filepath.Join(home, ".aws", "cli", "cache", "credential.json"), []byte("synthetic-private-value"), 0600); err != nil {
		return 82
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return 83
	}
	env := os.Environ()
	slices.Sort(env)
	file, err := os.OpenFile(filepath.Join(dir, "capture.json"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return 84
	}
	err = json.NewEncoder(file).Encode(commandCapture{Args: os.Args[1:], Env: env, Input: string(input)})
	_ = file.Close()
	if err != nil {
		return 85
	}
	fmt.Fprintln(os.Stdout, "synthetic-private-value")
	fmt.Fprintln(os.Stderr, "synthetic-private-value")
	switch filepath.Base(os.Args[0]) {
	case "aws-fail":
		return 7
	case "aws-wait":
		address, err := os.ReadFile(filepath.Join(dir, "control-address"))
		if err != nil {
			return 86
		}
		conn, err := net.DialTimeout("tcp", string(address), 5*time.Second)
		if err != nil {
			return 87
		}
		defer conn.Close()
		if err := json.NewEncoder(conn).Encode(os.Getpid()); err != nil {
			return 88
		}
		_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
		_, _ = io.Copy(io.Discard, conn)
	}
	return 0
}

func snsHelperDestination(t *testing.T, mode string) (Destination, string) {
	t.Helper()
	dst := snsDestination(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "aws"+mode)
	require.NoError(t, os.Symlink(dst.Executable, path))
	dst.Executable = path
	return dst, filepath.Join(dir, "capture.json")
}

func assertSNSHomeRemoved(t *testing.T, capture commandCapture) {
	t.Helper()
	var home string
	for _, entry := range capture.Env {
		if value, ok := strings.CutPrefix(entry, "HOME="); ok {
			home = value
		}
	}
	require.NotEmpty(t, home)
	_, err := os.Stat(home)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunSNS(t *testing.T) {
	for name, test := range map[string]struct {
		source, region, arn, status, mode string
		env                               map[string]string
		code                              int
	}{
		"static warning":              {source: "static", region: "us-east-1", arn: testSNSARN, status: "WARNING", env: map[string]string{"AWS_ACCESS_KEY_ID": "key", "AWS_SECRET_ACCESS_KEY": "secret", "AWS_SESSION_TOKEN": "token"}},
		"web identity critical":       {source: "web_identity", region: "eu-west-1", arn: "arn:aws:sns:eu-west-1:123456789012:alerts", status: "CRITICAL", env: map[string]string{"AWS_ROLE_ARN": "arn:aws:iam::123456789012:role/notifier", "AWS_WEB_IDENTITY_TOKEN_FILE": "/unread/token", "AWS_ROLE_SESSION_NAME": "notifier-test"}},
		"ecs clear":                   {source: "ecs", region: "us-east-1", arn: testSNSARN, status: "CLEAR", env: map[string]string{"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "/credentials"}},
		"imds":                        {source: "imds", region: "cn-north-1", arn: "arn:aws-cn:sns:cn-north-1:123456789012:alerts", status: "WARNING"},
		"dotted platform application": {source: "imds", region: "us-east-1", arn: "arn:aws:sns:us-east-1:123456789012:endpoint/APNS/app.v1/12345678-1234-1234-1234-123456789012", status: "WARNING"},
		"failure":                     {source: "imds", region: "us-east-1", arn: testSNSARN, status: "WARNING", mode: "-fail", code: 1},
	} {
		t.Run(name, func(t *testing.T) {
			for _, key := range []string{"HOME", "AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE", "BOTO_CONFIG", "AWS_CREDENTIAL_FILE", "AWS_PROFILE", "AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_SNS", "AWS_DATA_PATH", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_CONTAINER_CREDENTIALS_FULL_URI", "AWS_EC2_METADATA_SERVICE_ENDPOINT", "HTTPS_PROXY", "PYTHONPATH"} {
				t.Setenv(key, "synthetic-private-value")
			}
			dst, capture := snsHelperDestination(t, test.mode)
			dst.CredentialSource, dst.Env, dst.TargetARN = test.source, test.env, test.arn
			dst.MessageTemplate = "file://{{info}}"
			event := expectedEvent()
			event.Status, event.Info = test.status, "{{node}} $(literal) θερμοκρασία\ntext"
			input, err := json.Marshal(event)
			require.NoError(t, err)
			cfg := Config{Version: 1, Destinations: map[string]Destination{"target": dst}}
			var stdout, stderr bytes.Buffer
			assert.Equal(t, test.code, Run(context.Background(), []string{"send", "--config", writeCommandConfig(t, cfg), "--destination", "target"}, bytes.NewReader(input), &stdout, &stderr), stderr.String())
			assert.Empty(t, stdout.String())
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			captures := readCommandCaptures(t, capture)
			require.Len(t, captures, 1)
			got := captures[0]
			wantEnv := []string{"AWS_CLI_AUTO_PROMPT=off", "AWS_CLI_FILE_ENCODING=UTF-8", "AWS_CONFIG_FILE=/dev/null", "AWS_DEFAULT_REGION=" + test.region,
				"AWS_EC2_METADATA_DISABLED=true", "AWS_IGNORE_CONFIGURED_ENDPOINT_URLS=true", "AWS_MAX_ATTEMPTS=1", "AWS_PAGER=", "AWS_REGION=" + test.region,
				"AWS_RETRY_MODE=standard", "AWS_SHARED_CREDENTIALS_FILE=/dev/null", "BOTO_CONFIG=/dev/null", "NO_PROXY=*", "PATH=/usr/local/bin:/usr/bin:/bin"}
			if test.source == "imds" {
				wantEnv[4] = "AWS_EC2_METADATA_DISABLED=false"
			}
			for key, value := range test.env {
				wantEnv = append(wantEnv, key+"="+value)
			}
			for _, entry := range got.Env {
				if strings.HasPrefix(entry, "HOME=") {
					wantEnv = append(wantEnv, entry)
				}
			}
			slices.Sort(wantEnv)
			status := map[string]string{"WARNING": "needs attention", "CRITICAL": "is critical", "CLEAR": "recovered"}[test.status]
			wantJSON, err := json.Marshal(snsPublish{TargetARN: test.arn, Subject: "test-node " + status + " - test alert - test.chart", Message: "file://{{node}} $(literal) θερμοκρασία\ntext"})
			require.NoError(t, err)
			assert.Equal(t, commandCapture{Args: []string{"sns", "publish", "--region", test.region, "--cli-input-json", "file:///dev/stdin", "--no-cli-pager", "--no-cli-auto-prompt", "--output", "json"}, Env: wantEnv, Input: string(wantJSON) + "\n"}, got)
			assertSNSHomeRemoved(t, got)
		})
	}
}

func TestRunSNSSelectionAndFailures(t *testing.T) {
	for name, test := range map[string]struct {
		mode        string
		code, calls int
		message     string
	}{
		"validate":             {mode: "validate", message: "configuration is valid"},
		"unselected":           {mode: "unselected", message: "0 succeeded, 0 failed"},
		"missing secret":       {mode: "secret", code: 1, message: "secret resolved to an empty value"},
		"invalid resolved URI": {mode: "uri", code: 1, message: "relative path"},
		"invalid subject":      {mode: "subject", code: 1, message: "subject must be"},
		"empty message":        {mode: "message", code: 1, message: "message must be nonempty"},
		"start failure":        {mode: "missing", code: 1, message: "could not start command"},
		"mixed deduplicated":   {mode: "mixed", calls: 1, message: "1 succeeded, 1 failed"},
	} {
		t.Run(name, func(t *testing.T) {
			privateHomes := t.TempDir()
			t.Setenv("TMPDIR", privateHomes)
			dst, capture := snsHelperDestination(t, "")
			bad := dst
			bad.Executable = filepath.Join(t.TempDir(), "synthetic-private-value")
			event := expectedEvent()
			switch test.mode {
			case "validate", "unselected", "secret":
				dst.CredentialSource = "static"
				dst.Env = map[string]string{"AWS_ACCESS_KEY_ID": "key", "AWS_SECRET_ACCESS_KEY": "${env:UNSET_SNS_TEST_SECRET}"}
				t.Setenv("UNSET_SNS_TEST_SECRET", "")
			case "uri":
				dst.CredentialSource = "ecs"
				dst.Env = map[string]string{"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI": "${env:SNS_TEST_URI}"}
				t.Setenv("SNS_TEST_URI", "@synthetic-private-value/path")
			case "subject":
				event.Node += "\n"
			case "message":
				dst.MessageTemplate = "{{info}}"
				event.Info = ""
			case "missing":
				dst.Executable = bad.Executable
			}
			cfg := Config{Version: 1, Destinations: map[string]Destination{"target": dst, "bad": bad}, Routing: Routing{Roles: map[string][]string{"nobody": {}, "mixed": {"bad", "target", "target"}}}}
			args := []string{"send", "--config", writeCommandConfig(t, cfg)}
			switch test.mode {
			case "validate":
				args[0] = "validate"
			case "unselected":
				args = append(args, "--role", "nobody")
			case "mixed":
				args = append(args, "--role", "mixed")
			default:
				args = append(args, "--destination", "target")
			}
			input, err := json.Marshal(event)
			require.NoError(t, err)
			var stdout, stderr bytes.Buffer
			assert.Equal(t, test.code, Run(context.Background(), args, bytes.NewReader(input), &stdout, &stderr), stderr.String())
			assert.Contains(t, stdout.String()+stderr.String(), test.message)
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
			if test.calls > 0 {
				captures := readCommandCaptures(t, capture)
				require.Len(t, captures, test.calls)
				assertSNSHomeRemoved(t, captures[0])
			} else {
				_, err := os.Stat(capture)
				assert.ErrorIs(t, err, os.ErrNotExist)
			}
			remaining, err := filepath.Glob(filepath.Join(privateHomes, "alarm-notify-home-*"))
			require.NoError(t, err)
			assert.Empty(t, remaining)
		})
	}
}

func TestRunSNSCancellation(t *testing.T) {
	for name, test := range map[string]struct{ deadline bool }{"cancel": {}, "deadline": {true}} {
		t.Run(name, func(t *testing.T) {
			dst, capture := snsHelperDestination(t, "-wait")
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			require.NoError(t, listener.(*net.TCPListener).SetDeadline(time.Now().Add(5*time.Second)))
			require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(dst.Executable), "control-address"), []byte(listener.Addr().String()), 0600))
			cfg := Config{Version: 1, Destinations: map[string]Destination{"target": dst}}
			timeout := "10s"
			if test.deadline {
				timeout = "2s"
			}
			args := []string{"send", "--config", writeCommandConfig(t, cfg), "--destination", "target", "--timeout", timeout}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var stdout, stderr bytes.Buffer
			done := make(chan int, 1)
			go func() { done <- Run(ctx, args, strings.NewReader(validEvent), &stdout, &stderr) }()
			conn, err := listener.Accept()
			require.NoError(t, err)
			defer conn.Close()
			require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
			var pid int
			require.NoError(t, json.NewDecoder(conn).Decode(&pid))
			if !test.deadline {
				cancel()
			}
			select {
			case code := <-done:
				assert.Equal(t, 1, code)
			case <-time.After(5 * time.Second):
				t.Fatal("Run did not finish cleanup")
			}
			assert.ErrorIs(t, syscall.Kill(pid, 0), syscall.ESRCH)
			captures := readCommandCaptures(t, capture)
			require.Len(t, captures, 1)
			assertSNSHomeRemoved(t, captures[0])
			assert.NotContains(t, stderr.String(), "synthetic-private-value")
		})
	}
}

func TestSNSClosedProcessAdmission(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	processes := &commandexec.Runner{}
	processes.CloseAndWait()
	dst, capture := snsHelperDestination(t, "")
	require.ErrorIs(t, sendAWSSNS(context.Background(), processes, dst, expectedEvent()), context.Canceled)
	_, err := os.Stat(capture)
	assert.ErrorIs(t, err, os.ErrNotExist)
	remaining, err := filepath.Glob(filepath.Join(dir, "alarm-notify-home-*"))
	require.NoError(t, err)
	assert.Empty(t, remaining)
}
