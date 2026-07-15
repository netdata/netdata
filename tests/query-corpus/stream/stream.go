// SPDX-License-Identifier: GPL-3.0-or-later

// Package stream implements a minimal Netdata streaming child (the fixture
// "pusher"): it connects to a parent, negotiates capabilities, defines
// charts, pushes samples and serves replication requests over the real wire
// protocol. The daemon under test is completely stock; everything
// fixture-specific lives on this side of the socket.
package stream

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Capability bits, mirroring src/streaming/stream-capabilities.h. Only the
// bits the pusher negotiates are defined; the absence of compression and
// IEEE754 caps keeps the wire plaintext with plain decimal numbers, and the
// absence of SLOTS keeps chart/dimension references as string ids.
const (
	CapVCaps        uint32 = 1 << 6
	CapHLabels      uint32 = 1 << 7
	CapCLabels      uint32 = 1 << 9
	CapReplication  uint32 = 1 << 12
	CapInterpolated uint32 = 1 << 14
)

// CapsLive is the minimal capability set for live BEGIN2/SET2 ingestion.
const CapsLive = CapVCaps | CapHLabels | CapCLabels | CapInterpolated

// CapsReplication additionally announces child retention so the parent
// requests history through the replication dialogue.
const CapsReplication = CapsLive | CapReplication

// prompt is the parent's raw handshake acceptance, followed by the
// negotiated capabilities integer.
const prompt = "Hit me baby, push them over with the version="

// SN flags as sent on the wire with SET2/RSET.
const (
	FlagNotAnomalous = "A"  // sample explicitly not anomalous
	FlagAnomalous    = "''" // empty flags: sample is anomalous
	FlagEmpty        = "E"  // empty slot: a gap
	FlagReset        = "R"  // counter reset
)

// HostInfo identifies the child host of one streaming connection.
type HostInfo struct {
	Hostname    string
	MachineGUID string
	UpdateEvery int // defaults to 1
}

// Chart carries the metadata sent with the CHART line.
type Chart struct {
	ID          string // "type.id"
	Title       string
	Units       string
	Family      string
	Context     string
	Type        string // line/area/stacked; defaults to line
	Priority    int    // defaults to 1000
	UpdateEvery int    // defaults to 1
}

// Conn is one streaming child connection. Writes are buffered; callers
// control burst boundaries with Flush. The first write error sticks and is
// reported by Err and Flush.
type Conn struct {
	conn       net.Conn
	r          *bufio.Reader
	w          *bufio.Writer
	err        error
	Negotiated uint32
}

// Connect dials the parent, performs the STREAM handshake and returns the
// connection with the negotiated capabilities.
func Connect(addr, apiKey string, hi HostInfo, caps uint32) (*Conn, error) {
	if hi.UpdateEvery <= 0 {
		hi.UpdateEvery = 1
	}

	nc, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("stream: dial %s: %w", addr, err)
	}

	// the large write buffer lets a caller emit a whole fixture burst as one
	// write() syscall, keeping burst boundaries (Flush) meaningful
	c := &Conn{conn: nc, r: bufio.NewReader(nc), w: bufio.NewWriterSize(nc, 2<<20)}

	req := fmt.Sprintf(
		"STREAM key=%s&hostname=%s&registry_hostname=%s&machine_guid=%s"+
			"&update_every=%d&os=linux&timezone=Etc/UTC&abbrev_timezone=UTC"+
			"&utc_offset=0&hops=1&ver=%d&NETDATA_PROTOCOL_VERSION=1.1 HTTP/1.1\r\n"+
			"User-Agent: query-corpus-pusher/1.0\r\n"+
			"Accept: */*\r\n\r\n",
		apiKey, hi.Hostname, hi.Hostname, hi.MachineGUID, hi.UpdateEvery, caps)

	_ = nc.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := nc.Write([]byte(req)); err != nil {
		nc.Close()
		return nil, fmt.Errorf("stream: handshake write: %w", err)
	}

	// scan for the prompt (the reply is tiny, byte-wise matching is fine)
	matched := 0
	for matched < len(prompt) {
		b, err := c.r.ReadByte()
		if err != nil {
			nc.Close()
			return nil, fmt.Errorf("stream: handshake read after %d matched bytes: %w", matched, err)
		}
		switch b {
		case prompt[matched]:
			matched++
		case prompt[0]:
			matched = 1
		default:
			matched = 0
		}
	}

	// the negotiated capabilities integer follows the prompt
	var digits []byte
	for {
		_ = nc.SetReadDeadline(time.Now().Add(2 * time.Second))
		b, err := c.r.ReadByte()
		if err != nil {
			if len(digits) > 0 && errors.Is(err, os.ErrDeadlineExceeded) {
				break // stream paused after the number: we have it
			}
			nc.Close()
			return nil, fmt.Errorf("stream: handshake caps read: %w", err)
		}
		if b >= '0' && b <= '9' {
			digits = append(digits, b)
			continue
		}
		_ = c.r.UnreadByte()
		break
	}
	_ = nc.SetDeadline(time.Time{})

	n, err := strconv.ParseUint(string(digits), 10, 32)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("stream: handshake caps parse %q: %w", digits, err)
	}
	c.Negotiated = uint32(n)
	return c, nil
}

// Close flushes buffered output and closes the socket.
func (c *Conn) Close() error {
	flushErr := c.Flush()
	closeErr := c.conn.Close()
	if flushErr != nil {
		return flushErr
	}
	return closeErr
}

// Err returns the first write error, if any.
func (c *Conn) Err() error { return c.err }

// Flush writes all buffered protocol lines to the socket.
func (c *Conn) Flush() error {
	if c.err != nil {
		return c.err
	}
	c.err = c.w.Flush()
	return c.err
}

// Linef buffers one protocol line.
func (c *Conn) Linef(format string, a ...any) {
	if c.err != nil {
		return
	}
	if _, err := fmt.Fprintf(c.w, format, a...); err != nil {
		c.err = err
		return
	}
	c.err = c.w.WriteByte('\n')
}

// DefineChart buffers the CHART line and makes the chart the current scope.
func (c *Conn) DefineChart(ch Chart) {
	if ch.Type == "" {
		ch.Type = "line"
	}
	if ch.Priority <= 0 {
		ch.Priority = 1000
	}
	if ch.UpdateEvery <= 0 {
		ch.UpdateEvery = 1
	}
	c.Linef("CHART %s '' '%s' '%s' '%s' '%s' %s %d %d '' fixture-pusher corpus",
		ch.ID, ch.Title, ch.Units, ch.Family, ch.Context, ch.Type, ch.Priority, ch.UpdateEvery)
}

// Dimension buffers a DIMENSION line for the current chart scope.
func (c *Conn) Dimension(id, algorithm string, mul, div int) {
	if algorithm == "" {
		algorithm = "absolute"
	}
	if mul == 0 {
		mul = 1
	}
	if div == 0 {
		div = 1
	}
	c.Linef("DIMENSION '%s' '' %s %d %d ''", id, algorithm, mul, div)
}

// CLabel buffers one chart label (RRDLABEL_SRC_CONFIG) for the current
// chart scope; commit with CLabelCommit.
func (c *Conn) CLabel(name, value string) {
	c.Linef("CLABEL '%s' '%s' 2", name, value)
}

// CLabelCommit applies the buffered CLABEL lines to the current chart.
func (c *Conn) CLabelCommit() {
	c.Linef("CLABEL_COMMIT")
}

// Begin2 opens one interpolated sample for the chart: endTime is the exact
// per-sample timestamp; the wall-clock field is left unset ('#').
func (c *Conn) Begin2(chartID string, updateEvery int, endTime int64) {
	c.Linef("BEGIN2 '%s' %d %d #", chartID, updateEvery, endTime)
}

// Set2 stores one dimension sample. flags is the SN flags text (Flag*
// constants); FlagEmpty sends the canonical empty-slot form. The value is
// sent explicitly (not '#'): the parser's '#' fallback re-derives the value
// from the collected field, which it parses as an INTEGER for non-float
// dimensions (pluginsd_parser.c pluginsd_set_v2) — fractional fixture
// values would be truncated.
func (c *Conn) Set2(dimID, collected, flags string) {
	if flags == FlagEmpty {
		c.Linef("SET2 '%s' 0 NAN E", dimID)
		return
	}
	c.Linef("SET2 '%s' %s %s %s", dimID, collected, collected, flags)
}

// End2 closes the sample opened by Begin2.
func (c *Conn) End2() {
	c.Linef("END2")
}

// ChartDefinitionEnd declares the current chart's retention, prompting the
// parent to request replication of (firstT, lastT]. The request range is
// exclusive on the left: declare firstT one step BEFORE the first point that
// must be replicated.
func (c *Conn) ChartDefinitionEnd(firstT, lastT, childNow int64) {
	c.Linef("CHART_DEFINITION_END %d %d %d", firstT, lastT, childNow)
}

// ReadLine reads one protocol line from the parent. Debug primitive for
// probes that drive the dialogue manually.
func (c *Conn) ReadLine(deadline time.Time) (string, error) {
	_ = c.conn.SetReadDeadline(deadline)
	line, err := c.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// WriteRaw flushes any buffered lines and writes b to the socket as one
// write() syscall. Debug primitive for probes that control syscall
// boundaries precisely.
func (c *Conn) WriteRaw(b []byte) error {
	if err := c.Flush(); err != nil {
		return err
	}
	if _, err := c.conn.Write(b); err != nil {
		c.err = err
		return err
	}
	return nil
}

// ReplayValue is one dimension's sample inside a replicated row.
type ReplayValue struct {
	ID        string
	Collected string // ignored for FlagEmpty
	Flags     string
}

// ReplayRow is one replicated second: the per-dimension samples at time T.
type ReplayRow struct {
	T    int64
	Dims []ReplayValue
}

// ReplayHandler returns the fixture rows for chart in the window
// (after, before] — exclusive after, inclusive before.
type ReplayHandler func(chart string, after, before int64) []ReplayRow

// ServeReplication answers the parent's REPLAY_CHART requests from handler
// until every chart in charts has been granted streaming (start_streaming
// true), the parent closes, or timeout expires. It returns the number of
// rows served per chart.
func (c *Conn) ServeReplication(charts map[string]struct{ FirstT, LastT int64 }, childNow int64, handler ReplayHandler, timeout time.Duration) (map[string]int, error) {
	if err := c.Flush(); err != nil {
		return nil, err
	}

	served := make(map[string]int, len(charts))
	granted := make(map[string]bool, len(charts))
	deadline := time.Now().Add(timeout)

	for len(granted) < len(charts) {
		_ = c.conn.SetReadDeadline(deadline)
		line, err := c.r.ReadString('\n')
		if err != nil {
			return served, fmt.Errorf("stream: replication read (granted %d/%d): %w", len(granted), len(charts), err)
		}

		// parent quotes the chart id and the boolean with double quotes
		words := strings.Fields(strings.ReplaceAll(line, `"`, " "))
		if len(words) < 5 || words[0] != "REPLAY_CHART" {
			continue
		}
		chart := words[1]
		wantStream := words[2] == "true"
		after, err1 := strconv.ParseInt(words[3], 10, 64)
		before, err2 := strconv.ParseInt(words[4], 10, 64)
		if err1 != nil || err2 != nil {
			return served, fmt.Errorf("stream: cannot parse REPLAY_CHART window: %q", strings.TrimSpace(line))
		}

		ret, known := charts[chart]
		if !known {
			return served, fmt.Errorf("stream: parent requested replication of unknown chart %q", chart)
		}

		if after != 0 && before != 0 {
			for _, row := range handler(chart, after, before) {
				c.Linef("RBEGIN '%s' %d %d %d", chart, row.T-1, row.T, childNow)
				for _, dv := range row.Dims {
					if dv.Flags == FlagEmpty {
						c.Linef("RSET '%s' NAN E", dv.ID)
					} else {
						c.Linef("RSET '%s' %s %s", dv.ID, dv.Collected, dv.Flags)
					}
				}
				served[chart]++
			}
		}

		streamWord := "false"
		if wantStream {
			streamWord = "true"
			granted[chart] = true
		}
		c.Linef("REND 1 %d %d %s %d %d %d", ret.FirstT, ret.LastT, streamWord, after, before, childNow)
		if err := c.Flush(); err != nil {
			return served, err
		}
	}

	_ = c.conn.SetReadDeadline(time.Time{})
	return served, nil
}
