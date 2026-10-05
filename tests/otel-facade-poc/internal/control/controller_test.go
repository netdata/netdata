// SPDX-License-Identifier: GPL-3.0-or-later

package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

func newTestController(t *testing.T) (*Controller, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	c, err := New(io.NopCloser(strings.NewReader("")), out, "127.0.0.1:4317", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return c, out
}

func command(t *testing.T, c *Controller, out *bytes.Buffer, args, payload string) int {
	t.Helper()
	out.Reset()
	err := c.HandleCall(context.Background(), functions.Call{
		UID: "test", Method: "config", Args: strings.Split(args, " "),
		HasPayload: payload != "", ContentType: "application/json", Payload: []byte(payload),
	})
	if err != nil {
		t.Fatal(err)
	}
	var response struct{ Status int }
	lines := strings.Split(out.String(), "\n")
	if len(lines) < 3 || !strings.HasPrefix(lines[0], "FUNCTION_RESULT_BEGIN test ") {
		t.Fatalf("invalid result: %s", out)
	}
	if err := json.Unmarshal([]byte(lines[1]), &response); err != nil {
		t.Fatal(err)
	}
	return response.Status
}

func TestPassiveAddAndStaleReadiness(t *testing.T) {
	c, out := newTestController(t)
	_, changed, _, _ := c.Snapshot()
	if code := command(t, c, out, Prefix+"hostmetrics add first", `{}`); code != 202 {
		t.Fatalf("add: %d", code)
	}
	select {
	case <-changed:
		t.Fatal("passive add requested reload")
	default:
	}
	if err := c.Ready(1, true); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), " status running") {
		t.Fatal("passive job marked running")
	}
	if code := command(t, c, out, Prefix+"hostmetrics:first enable", ""); code != 202 {
		t.Fatalf("enable: %d", code)
	}
	select {
	case <-changed:
	default:
		t.Fatal("enable did not invalidate snapshot")
	}
	if err := c.Ready(1, true); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), " status running") {
		t.Fatal("stale service marked replacement running")
	}
	if err := c.Ready(2, true); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "CONFIG otel-poc:hostmetrics:first status running") || strings.Index(text, "FUNCTION_RESULT_END") > strings.Index(text, " status running") {
		t.Fatalf("acceptance/readiness ordering: %s", text)
	}
}

func TestRejectedUpdatePreservesSnapshot(t *testing.T) {
	c, out := newTestController(t)
	command(t, c, out, Prefix+"hostmetrics add first", `{"interval":"1s"}`)
	command(t, c, out, Prefix+"hostmetrics:first enable", "")
	before, changed, _, _ := c.Snapshot()
	for _, invalid := range []string{`{"interval":"0s"}`, `{"interval":null}`, `{"paths":[]}`, `{"unknown":true}`, `null`, `{} {}`} {
		if code := command(t, c, out, Prefix+"hostmetrics:first update", invalid); code != 400 {
			t.Fatalf("%s returned %d", invalid, code)
		}
	}
	after, _, _, _ := c.Snapshot()
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if !bytes.Equal(a, b) {
		t.Fatal("rejected update changed effective Collector config")
	}
	select {
	case <-changed:
		t.Fatal("rejected update requested reload")
	default:
	}
}

func TestDisabledUpdateAndTemplateTest(t *testing.T) {
	c, out := newTestController(t)
	command(t, c, out, Prefix+"hostmetrics add first", `{}`)
	command(t, c, out, Prefix+"hostmetrics:first disable", "")
	_, changed, _, _ := c.Snapshot()
	if code := command(t, c, out, Prefix+"hostmetrics:first update", `{"interval":"3s"}`); code != 200 {
		t.Fatalf("disabled update: %d", code)
	}
	if code := command(t, c, out, Prefix+"hostmetrics test candidate", `{"interval":"1s"}`); code != 200 {
		t.Fatalf("template test: %d", code)
	}
	select {
	case <-changed:
		t.Fatal("disabled update or test requested reload")
	default:
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestFailedReplyDoesNotReleaseActivation(t *testing.T) {
	c, out := newTestController(t)
	command(t, c, out, Prefix+"hostmetrics add first", `{}`)
	_, changed, terminal, _ := c.Snapshot()
	c.output = failingWriter{}
	err := c.HandleCall(context.Background(), functions.Call{UID: "failure", Method: "config", Args: []string{Prefix + "hostmetrics:first", "enable"}})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected write failure, got %v", err)
	}
	select {
	case <-changed:
		t.Fatal("failed acceptance reply released activation")
	default:
	}
	select {
	case <-terminal:
	default:
		t.Fatal("output failure did not fail closed")
	}
	if _, _, _, err := c.Snapshot(); err == nil {
		t.Fatal("failed controller still served configuration")
	}
}
