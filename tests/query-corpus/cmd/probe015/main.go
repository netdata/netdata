// SPDX-License-Identifier: GPL-3.0-or-later

// probe015 is the CASE-015 forensics driver: it boots a stock daemon (as a
// child process, so strace -f from the caller reaches it), then runs N
// replication pushes that write the reply burst in fixed-size chunks and
// close the socket immediately. Per iteration it prints a ledger line:
// bytes written vs rows stored vs the daemon's own bytes_in accounting —
// the raw material for reconciling read-vs-parse-vs-store.
//
// Temporary diagnostic tool for the corpus SOW CASE-015 entry.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func main() {
	binary := flag.String("binary", "../../build/netdata", "stock netdata binary")
	points := flag.Int("points", 60, "points per iteration")
	chunk := flag.Int("chunk", 4096, "write() chunk size for the reply burst")
	gap := flag.Duration("gap", 0, "delay between chunk writes (biases the race toward tail loss)")
	iters := flag.Int("iters", 10, "iterations")
	keep := flag.Bool("keep", false, "keep the daemon run dir")
	flag.Parse()

	runDir, err := os.MkdirTemp("", "probe015-")
	die(err)
	fmt.Printf("run dir: %s\n", runDir)

	td, err := daemon.Start(daemon.Options{Binary: mustAbs(*binary), RunDir: runDir})
	die(err)
	fmt.Printf("daemon: %s (streaming %s)\n", td.BaseURL, td.Addr)

	for i := 0; i < *iters; i++ {
		iteration(td, i, *points, *chunk, *gap)
		time.Sleep(500 * time.Millisecond) // separate iterations for strace carving
	}

	_ = td.Stop()
	report(runDir)

	if !*keep {
		_ = os.RemoveAll(runDir)
	} else {
		fmt.Printf("kept: %s\n", runDir)
	}
}

func iteration(td *daemon.Daemon, i, points, chunk int, gap time.Duration) {
	host := fmt.Sprintf("probe-%02d", i)
	guid := fmt.Sprintf("22222222-2222-4222-8222-%012d", i)
	chartID := fmt.Sprintf("fixture.p%02d", i)
	t0 := int64(1700000000)
	firstT, lastT := t0, t0+int64(points)

	conn, err := stream.Connect(td.Addr, daemon.StreamKey,
		stream.HostInfo{Hostname: host, MachineGUID: guid}, stream.CapsReplication)
	die(err)

	conn.DefineChart(stream.Chart{ID: chartID, Title: "probe", Units: "u", Family: "f", Context: chartID})
	conn.Dimension("load", "absolute", 1, 1)
	conn.ChartDefinitionEnd(firstT, lastT, lastT)
	die(conn.Flush())

	// wait for the REPLAY_CHART covering our declared retention
	var after, before int64
	deadline := time.Now().Add(30 * time.Second)
	for {
		line, err := conn.ReadLine(deadline)
		die(err)
		w := strings.Fields(strings.ReplaceAll(line, `"`, " "))
		if len(w) >= 5 && w[0] == "REPLAY_CHART" && w[1] == chartID {
			after, _ = strconv.ParseInt(w[3], 10, 64)
			before, _ = strconv.ParseInt(w[4], 10, 64)
			break
		}
	}

	// build the whole reply as one byte slice
	var sb strings.Builder
	rows := 0
	for t := max64(after+1, firstT+1); t <= min64(before, lastT); t++ {
		fmt.Fprintf(&sb, "RBEGIN '%s' %d %d %d\n", chartID, t-1, t, lastT)
		fmt.Fprintf(&sb, "RSET 'load' %d A\n", (t-t0)%10)
		rows++
	}
	fmt.Fprintf(&sb, "REND 1 %d %d true %d %d %d\n", firstT, lastT, after, before, lastT)
	payload := []byte(sb.String())

	// write in fixed-size chunks, then close IMMEDIATELY after the last one
	for off := 0; off < len(payload); off += chunk {
		end := min(off+chunk, len(payload))
		if off > 0 && gap > 0 {
			time.Sleep(gap)
		}
		die(conn.WriteRaw(payload[off:end]))
	}
	die(conn.Close())

	// let the dust settle, then measure what was stored
	time.Sleep(3 * time.Second)
	stored, retention := storedRows(td, host, chartID, firstT, lastT)
	fmt.Printf("LEDGER iter=%02d host=%s rows_served=%d payload_bytes=%d chunks=%d stored_rows=%d retention=%s\n",
		i, host, rows, len(payload), (len(payload)+chunk-1)/chunk, stored, retention)
}

func storedRows(td *daemon.Daemon, host, context string, first, last int64) (int, string) {
	doc, err := td.DataV3(host, daemon.DataParams(context, first, last, last-first))
	if err != nil {
		return 0, fmt.Sprintf("query-error: %v", err)
	}
	ret, _ := daemon.QueryRetention(doc)
	cols, err := canon.Columns(doc)
	if err != nil {
		return 0, fmt.Sprintf("[%d,%d] decode-error: %v", ret.FirstEntry, ret.LastEntry, err)
	}
	n := 0
	for _, pt := range cols["load"] {
		if pt.Value != nil {
			n++
		}
	}
	return n, fmt.Sprintf("[%d,%d]", ret.FirstEntry, ret.LastEntry)
}

// report extracts the daemon's own accounting per probe host.
func report(runDir string) {
	data, err := os.ReadFile(runDir + "/log/daemon.log")
	if err != nil {
		fmt.Printf("no daemon.log: %v\n", err)
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "receiver disconnected") || !strings.Contains(line, "node=probe-") {
			continue
		}
		var node, msgs, in, out string
		for _, f := range strings.Fields(line) {
			switch {
			case strings.HasPrefix(f, "node="):
				node = f
			case strings.HasPrefix(f, "msgs="):
				msgs = f
			case strings.HasPrefix(f, "bytes_in="):
				in = f
			case strings.HasPrefix(f, "bytes_out="):
				out = f
			}
		}
		fmt.Printf("DAEMON %s %s %s %s\n", node, msgs, in, out)
	}
}

func mustAbs(p string) string {
	abs, err := os.Getwd()
	die(err)
	if strings.HasPrefix(p, "/") {
		return p
	}
	return abs + "/" + p
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func die(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "FATAL:", err)
		os.Exit(1)
	}
}
