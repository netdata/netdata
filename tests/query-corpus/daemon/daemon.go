// SPDX-License-Identifier: GPL-3.0-or-later

// Package daemon boots a completely stock netdata binary with a generated
// test configuration (dbengine under a scratch run dir, all collectors and
// subsystems off) and provides the corpus driver primitives: HTTP queries,
// the retention settle barrier, restart.
package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// StreamKey is the stream.conf api key the fixture children connect
// with — a fixed random UUID identifying the corpus child on a local
// throwaway daemon, not a credential.
const StreamKey = "6c41aa90-b303-49a6-83c8-4859b042e4a0"

// Options configures the daemon under test.
type Options struct {
	Binary       string // path to the stock netdata binary
	RunDir       string // scratch directory for etc/cache/lib/log
	Port         int    // 0 picks a free port
	StorageTiers int    // defaults to 3
	// Tier0DiskSpaceMB, when non-zero, caps tier0's dbengine disk quota
	// so old tier0 datafiles rotate out while higher tiers keep the full
	// history — the layer-4 plan-switching scenario. Retention TIME knobs
	// are unusable at the fixed 2023 fixture epoch (wall-clock enforced).
	Tier0DiskSpaceMB int
	// ReplicationStepSeconds, when non-zero, bounds the parent's per-request
	// replication window, so a streaming fixture can generate rows per
	// request instead of materializing millions of points.
	ReplicationStepSeconds int
}

// Daemon is one running netdata under test.
type Daemon struct {
	Opts    Options
	BaseURL string
	Addr    string // host:port for streaming connections
	cmd     *exec.Cmd
	waitCh  chan error
}

const netdataConfTemplate = `[global]
    hostname = corpus-parent

[directories]
    config = %[1]s/etc
    cache = %[1]s/cache
    lib = %[1]s/lib
    log = %[1]s/log
    home = %[1]s/lib

[web]
    bind to = 127.0.0.1:%[2]d

[db]
    db = dbengine
    update every = 1
    storage tiers = %[3]d
    replication period = 3650d
    replication step = %[4]s
    dbengine tier 0 retention time = 0
    dbengine tier 1 retention time = 0
    dbengine tier 2 retention time = 0
%[5]s
[ml]
    enabled = no

[health]
    enabled = no

[registry]
    enabled = no

[plugins]
    proc = no
    diskspace = no
    cgroups = no
    tc = no
    idlejitter = no
    statsd = no
    apps = no
    go.d = no
    charts.d = no
    python.d = no
    debugfs = no
    perf = no
    slabinfo = no
    ioping = no
    ebpf = no
    systemd-journal = no
    network-viewer = no
    timex = no
    profile = no
`

const streamConfTemplate = `[stream]
    enabled = no

[%s]
    enabled = yes
    type = api
    default memory mode = dbengine
    health enabled by default = no
    replication period = 3650d
`

// freePort asks the kernel for an unused localhost TCP port.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	return port, l.Close()
}

// Start writes the test configuration under RunDir, boots the daemon in the
// foreground and waits until the HTTP API answers.
func Start(o Options) (*Daemon, error) {
	if o.StorageTiers <= 0 {
		o.StorageTiers = 3
	}
	if o.Port == 0 {
		port, err := freePort()
		if err != nil {
			return nil, fmt.Errorf("daemon: free port: %w", err)
		}
		o.Port = port
	}

	for _, sub := range []string{"etc", "cache", "lib", "log"} {
		if err := os.MkdirAll(filepath.Join(o.RunDir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("daemon: run dir: %w", err)
		}
	}

	step := "3650d"
	if o.ReplicationStepSeconds > 0 {
		step = fmt.Sprintf("%ds", o.ReplicationStepSeconds)
	}
	extraDB := ""
	if o.Tier0DiskSpaceMB > 0 {
		extraDB = fmt.Sprintf("    dbengine tier 0 retention size = %dMiB\n", o.Tier0DiskSpaceMB)
	}
	conf := fmt.Sprintf(netdataConfTemplate, o.RunDir, o.Port, o.StorageTiers, step, extraDB)
	confPath := filepath.Join(o.RunDir, "etc", "netdata.conf")
	if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil {
		return nil, fmt.Errorf("daemon: write netdata.conf: %w", err)
	}
	streamConf := fmt.Sprintf(streamConfTemplate, StreamKey)
	if err := os.WriteFile(filepath.Join(o.RunDir, "etc", "stream.conf"), []byte(streamConf), 0o644); err != nil {
		return nil, fmt.Errorf("daemon: write stream.conf: %w", err)
	}
	// opt out of anonymous statistics before first boot
	if err := os.WriteFile(filepath.Join(o.RunDir, "etc", ".opt-out-from-anonymous-statistics"), nil, 0o644); err != nil {
		return nil, fmt.Errorf("daemon: write opt-out: %w", err)
	}

	d := &Daemon{
		Opts:    o,
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d", o.Port),
		Addr:    fmt.Sprintf("127.0.0.1:%d", o.Port),
	}
	if err := d.launch(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Daemon) launch() error {
	confPath := filepath.Join(d.Opts.RunDir, "etc", "netdata.conf")
	cmd := exec.Command(d.Opts.Binary, "-D", "-c", confPath)
	stdout, err := os.Create(filepath.Join(d.Opts.RunDir, "log", "stdout.log"))
	if err != nil {
		return fmt.Errorf("daemon: stdout log: %w", err)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	if err := cmd.Start(); err != nil {
		stdout.Close()
		return fmt.Errorf("daemon: start %s: %w", d.Opts.Binary, err)
	}
	d.cmd = cmd
	d.waitCh = make(chan error, 1)
	go func() {
		d.waitCh <- cmd.Wait()
		stdout.Close()
	}()

	// wait for the HTTP API; the probe needs its own timeout so a daemon
	// that accepts TCP but never answers cannot hang the readiness loop
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(60 * time.Second)
	for {
		resp, err := client.Get(d.BaseURL + "/api/v1/info")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case werr := <-d.waitCh:
			// the process is already reaped; make a later Stop() a no-op
			// instead of blocking forever on the drained wait channel
			d.cmd = nil
			return fmt.Errorf("daemon: exited during startup: %v (see %s/log/stdout.log)", werr, d.Opts.RunDir)
		default:
		}
		if time.Now().After(deadline) {
			_ = d.Stop()
			return fmt.Errorf("daemon: HTTP API not ready after 60s (see %s/log/stdout.log)", d.Opts.RunDir)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Stop terminates the daemon gracefully, escalating to SIGKILL.
func (d *Daemon) Stop() error {
	if d.cmd == nil || d.cmd.Process == nil {
		return nil
	}
	_ = d.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-d.waitCh:
	case <-time.After(30 * time.Second):
		_ = d.cmd.Process.Kill()
		<-d.waitCh
	}
	d.cmd = nil
	return nil
}

// Restart stops the daemon and boots it again on the same run dir and port,
// exercising the journal-replay read path.
func (d *Daemon) Restart() error {
	if err := d.Stop(); err != nil {
		return err
	}
	return d.launch()
}

// queryClient bounds every corpus query: no legitimate corpus query takes
// more than a few seconds, so a stalled daemon fails the test crisply
// instead of hanging it until the go test framework panics.
var queryClient = &http.Client{Timeout: 30 * time.Second}

// getRawBody performs a bounded GET and returns the raw response body.
func getRawBody(u string) ([]byte, error) {
	resp, err := queryClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("daemon: GET %s: %w", u, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("daemon: read %s: %w", u, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon: GET %s: HTTP %d: %s", u, resp.StatusCode, body)
	}
	return body, nil
}

// getJSON performs a bounded GET and parses the JSON response.
func getJSON(u string) (map[string]any, error) {
	body, err := getRawBody(u)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("daemon: parse %s: %w (body %q)", u, err, truncate(body, 300))
	}
	return doc, nil
}

// DataV1Raw queries /host/<host>/api/v1/data and returns the raw response
// body — the classic formatter surface (csv, tsv, ssv, html, arrays…)
// asserted byte-level by the formatter layer.
func (d *Daemon) DataV1Raw(host string, params url.Values) (string, error) {
	body, err := getRawBody(fmt.Sprintf("%s/host/%s/api/v1/data?%s", d.BaseURL, url.PathEscape(host), params.Encode()))
	return string(body), err
}

// DataV3All queries /api/v3/data (all nodes of the agent) with the given
// parameters — the multi-node query surface used by group-by layers.
func (d *Daemon) DataV3All(params url.Values) (map[string]any, error) {
	return getJSON(fmt.Sprintf("%s/api/v3/data?%s", d.BaseURL, params.Encode()))
}

// DataV3 queries /host/<host>/api/v3/data with the given parameters and
// returns the parsed JSON document.
func (d *Daemon) DataV3(host string, params url.Values) (map[string]any, error) {
	return getJSON(fmt.Sprintf("%s/host/%s/api/v3/data?%s", d.BaseURL, url.PathEscape(host), params.Encode()))
}

// HostJSON queries an arbitrary /host/<host>/<endpoint> API path (e.g.
// "api/v2/weights") and returns the parsed JSON document.
func (d *Daemon) HostJSON(host, endpoint string, params url.Values) (map[string]any, error) {
	return getJSON(fmt.Sprintf("%s/host/%s/%s?%s", d.BaseURL, url.PathEscape(host), endpoint, params.Encode()))
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

// DataParams returns the corpus defaults for a tier0 read-back query on one
// context: absolute window (after, before] with the given bucket count
// (points = window / update_every for identity read-back).
func DataParams(context string, after, before, points int64) url.Values {
	return url.Values{
		"scope_contexts": {context},
		"after":          {strconv.FormatInt(after, 10)},
		"before":         {strconv.FormatInt(before, 10)},
		"points":         {strconv.FormatInt(points, 10)},
		"time_group":     {"average"},
		"group_by":       {"dimension"},
		"aggregation":    {"avg"},
		"format":         {"json2"},
		"options":        {"jsonwrap"},
	}
}

// DataParamsTier returns a forced-tier read-back query: tier=N pins the
// query plan to that tier (RRDR_OPTION_SELECTED_TIER — no tier switching, no
// cross-tier gap filling) and natural points snap the view update_every to
// the tier granularity, so an aligned window reads back one bucket per tier
// point. timeGroup selects which STORAGE_POINT field the value carries
// (query-execute.c tier fetch): sum, min, max, or average (= sum/count).
func DataParamsTier(context string, tier int, after, before, points int64, timeGroup string) url.Values {
	p := DataParams(context, after, before, points)
	p.Set("tier", strconv.Itoa(tier))
	p.Set("time_group", timeGroup)
	return p
}

// Retention is the db window the daemon reports for one query.
type Retention struct {
	FirstEntry int64
	LastEntry  int64
}

// QueryRetention extracts db.first_entry/db.last_entry from a json2 reply.
func QueryRetention(doc map[string]any) (Retention, bool) {
	db, ok := doc["db"].(map[string]any)
	if !ok {
		return Retention{}, false
	}
	first, ok1 := db["first_entry"].(float64)
	last, ok2 := db["last_entry"].(float64)
	if !ok1 || !ok2 {
		return Retention{}, false
	}
	return Retention{FirstEntry: int64(first), LastEntry: int64(last)}, true
}

// WaitRetention polls the context on host until the daemon reports exactly
// the expected retention window — the corpus settle barrier. It returns the
// last observed retention on timeout.
func (d *Daemon) WaitRetention(host, context string, first, last int64, timeout time.Duration) (Retention, error) {
	deadline := time.Now().Add(timeout)
	var seen Retention
	for {
		doc, err := d.DataV3(host, DataParams(context, first-1, last, last-first+1))
		if err == nil {
			if ret, ok := QueryRetention(doc); ok {
				seen = ret
				if ret.FirstEntry == first && ret.LastEntry == last {
					return ret, nil
				}
			}
		}
		if time.Now().After(deadline) {
			return seen, fmt.Errorf("daemon: retention not settled on %s/%s after %s: have [%d,%d] want [%d,%d]",
				host, context, timeout, seen.FirstEntry, seen.LastEntry, first, last)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
