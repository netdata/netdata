// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	ndlpEmerg  = 0
	ndlpAlert  = 1
	ndlpCrit   = 2
	ndlpErr    = 3
	ndlpWarn   = 4
	ndlpNotice = 5
	ndlpInfo   = 6
	ndlpDebug  = 7
)

var ndlpNames = map[int]string{
	ndlpEmerg:  "emergency",
	ndlpAlert:  "alert",
	ndlpCrit:   "critical",
	ndlpErr:    "error",
	ndlpWarn:   "warning",
	ndlpNotice: "notice",
	ndlpInfo:   "info",
	ndlpDebug:  "debug",
}

type invocationLogger struct {
	programName string
	cmdLine     string
	level       int
}

func newInvocationLogger(args []string, level int) invocationLogger {
	programName := "cgroup-name"
	if len(args) > 0 && args[0] != "" {
		programName = filepath.Base(args[0])
	}
	return invocationLogger{
		programName: programName,
		cmdLine:     buildCmdLine(args),
		level:       level,
	}
}

func buildCmdLine(args []string) string {
	if len(args) == 0 {
		return "'' "
	}
	var b strings.Builder
	for _, arg := range args {
		b.WriteString("'")
		b.WriteString(arg)
		b.WriteString("' ")
	}
	return b.String()
}

func (l *invocationLogger) logf(level int, format string, args ...any) {
	if level > l.level {
		return
	}
	message := format
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}
	fmt.Fprintf(os.Stderr, "time=%s comm=%s level=%s request=%q msg=%q\n",
		time.Now().Format("2006-01-02T15:04:05.000Z07:00"),
		l.programName, ndlpNames[level], l.cmdLine, message)
}

type callRecord struct {
	label    string
	duration time.Duration
}

type invocationBudget struct {
	timeout   time.Duration
	expiresAt time.Time
	callsMu   sync.Mutex
	calls     []callRecord
}

func (b *invocationBudget) context() (context.Context, context.CancelFunc) {
	if b.timeout <= 0 {
		return context.WithCancel(context.Background())
	}
	b.expiresAt = time.Now().Add(b.timeout)
	return context.WithDeadline(context.Background(), b.expiresAt)
}

func (b *invocationBudget) expired() bool {
	return !b.expiresAt.IsZero() && time.Now().After(b.expiresAt)
}

func (b *invocationBudget) track(label string, started time.Time) {
	b.callsMu.Lock()
	b.calls = append(b.calls, callRecord{label: label, duration: time.Since(started)})
	b.callsMu.Unlock()
}

func (r *resolver) setupDeadline() (context.Context, context.CancelFunc) {
	return r.budget.context()
}

func (r *resolver) budgetExpired() bool {
	return r.budget.expired()
}

func (r *resolver) track(label string, started time.Time) {
	r.budget.track(label, started)
}

func (r *resolver) logCallBreakdown() {
	r.budget.callsMu.Lock()
	var b strings.Builder
	for i := range r.budget.calls {
		fmt.Fprintf(&b, " %s=%s", r.budget.calls[i].label, r.budget.calls[i].duration.Round(time.Millisecond))
	}
	r.budget.callsMu.Unlock()

	breakdown := b.String()
	if breakdown == "" {
		breakdown = " (no external calls were attempted)"
	}
	r.errorf("name resolution budget expired; time spent per external call:%s", breakdown)
}

func (r *resolver) logf(level int, format string, args ...any) {
	r.logger.logf(level, format, args...)
}
func (r *resolver) infof(format string, args ...any)    { r.logf(ndlpInfo, format, args...) }
func (r *resolver) warningf(format string, args ...any) { r.logf(ndlpWarn, format, args...) }
func (r *resolver) errorf(format string, args ...any)   { r.logf(ndlpErr, format, args...) }
func (r *resolver) fatalf(format string, args ...any)   { r.logf(ndlpAlert, format, args...) }
