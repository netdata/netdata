// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package rasdaemon

import (
	"context"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// rasMcCtlBinaryNames / rasMcCtlFallbackPaths locate ras-mc-ctl. It is a Perl script installed
// into sbin by every distro that packages rasdaemon.
//
// ndexec.FindBinary searches PATH by name first, then stats each fallback entry as a FULL FILE
// PATH (not as a directory to scan), so these are complete paths. sbin is frequently absent from
// the PATH of the unprivileged user the agent runs as, which is exactly why the fallbacks matter.
var (
	rasMcCtlBinaryNames   = []string{"ras-mc-ctl"}
	rasMcCtlFallbackPaths = []string{
		"/usr/sbin/ras-mc-ctl",
		"/sbin/ras-mc-ctl",
		"/usr/local/sbin/ras-mc-ctl",
		"/usr/bin/ras-mc-ctl",
		"/bin/ras-mc-ctl",
	}
)

// rasMcCtlCli is the seam the collector collects through, so tests can supply fixture bytes
// instead of executing anything.
type rasMcCtlCli interface {
	summary(ctx context.Context) ([]byte, error)
}

func newRasMcCtlExec(binPath string, timeout time.Duration, log *logger.Logger) *rasMcCtlExec {
	return &rasMcCtlExec{Logger: log, binPath: binPath, timeout: timeout}
}

type rasMcCtlExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

// summary runs `ras-mc-ctl --summary`.
//
// The collection context is passed through so that stopping or reloading the job interrupts an
// in-flight command instead of waiting out the timeout.
//
// It runs UNPRIVILEGED and needs no ndsudo entry: rasdaemon's SQLite database is installed
// world-readable (0644, with 0755 parent directories), so an unprivileged reader can produce a
// complete summary. That is verified on the development host; if a distro ever tightens those
// permissions the command fails loudly rather than silently reporting zero errors, because
// parseSummary rejects empty output.
func (e *rasMcCtlExec) summary(ctx context.Context) ([]byte, error) {
	out, _, _, err := ndexec.RunUnprivilegedWithOptionsUsageContext(
		ctx, e.Logger, e.timeout, ndexec.RunOptions{}, e.binPath, "--summary")
	return out, err
}
