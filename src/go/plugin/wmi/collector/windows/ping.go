// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"context"
	"errors"
	"net"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pinger"
)

// Every job pings its host with the SNMP collector's probe: three raw ICMP echo
// requests 100 ms apart.
const (
	pingPackets    = 3
	pingInterval   = 100 * time.Millisecond
	minPingTimeout = time.Second
	maxPingTimeout = 3 * time.Second

	pingDisabledMessage = "ICMP ping disabled: the plugin may not open raw ICMP sockets, " +
		"which on Linux requires the CAP_NET_RAW capability"
)

func (c *Collector) initPinger() error {
	client, err := c.newPinger(pinger.Config{
		Probe: pinger.ProbeConfig{
			Network:    "ip",
			Privileged: true,
			Packets:    pingPackets,
			Interval:   confopt.Duration(pingInterval),
			Timeout:    c.pingTimeout(),
		},
	}, c.Logger)
	if err != nil {
		// The pinger rejects only its own fixed settings, in fixed text.
		return dyncfg.NewPublicError("failed to initialize ICMP ping: "+err.Error(), err)
	}
	c.pingClient = client
	return nil
}

// pingTimeout bounds the echo requests and replies of one probe by the
// collection interval, within 1-3 seconds as the SNMP collector does, and never
// beyond the collection timeout. Resolving a host name is not covered.
func (c *Collector) pingTimeout() time.Duration {
	timeout := time.Duration(c.UpdateEvery) * time.Second
	timeout = max(min(timeout, maxPingTimeout), minPingTimeout)
	return min(timeout, c.Timeout.Duration())
}

// startPing probes the host while the WMI poll runs. The returned function
// waits for the probe and reports whether it produced a sample; abandon
// cancels a probe whose result is no longer needed.
func (c *Collector) startPing(ctx context.Context) func(abandon bool) (pinger.Sample, bool) {
	client := c.pingClient
	if client == nil {
		return func(bool) (pinger.Sample, bool) { return pinger.Sample{}, false }
	}
	ctx, cancel := context.WithCancel(ctx)
	type outcome struct {
		sample pinger.Sample
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		sample, err := client.Probe(ctx, c.Host)
		done <- outcome{sample, err}
	}()
	return func(abandon bool) (pinger.Sample, bool) {
		defer cancel()
		if abandon {
			cancel()
		}
		result := <-done
		return result.sample, c.pingSucceeded(ctx, result.err)
	}
}

// pingSucceeded handles a probe error. A missing privilege disables ping for
// the job with one message, even when the probe was abandoned; other failures
// are logged at most once a minute. A host that does not answer is not an
// error: its sample reports the loss.
func (c *Collector) pingSucceeded(ctx context.Context, err error) bool {
	switch {
	case err == nil:
		return true
	case pingNotPermitted(err):
		c.pingClient = nil
		c.Error(dyncfg.NewPublicError(pingDisabledMessage, err))
	case ctx.Err() != nil:
	default:
		c.Limit("wmi:ping", 1, time.Minute).Warning(pingFailure(err))
	}
	return false
}

// pingNotPermitted reports a raw ICMP socket the system refused to create. A
// refused send, such as one a local firewall rejects, is an ordinary failure.
func pingNotPermitted(err error) bool {
	var opErr *net.OpError
	var errno syscall.Errno
	return errors.As(err, &opErr) && opErr.Op == "listen" &&
		errors.As(err, &errno) && (errno == syscall.EPERM || errno == syscall.EACCES)
}

// pingFailure keeps the operating system's error text, which is fixed; the
// probe's own message names the host and its address.
func pingFailure(err error) error {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dyncfg.NewPublicError("ICMP ping failed: could not resolve the host", err)
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return dyncfg.NewPublicError("ICMP ping failed: "+errno.Error(), err)
	}
	return dyncfg.NewPublicError("ICMP ping failed", err)
}
