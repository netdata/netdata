// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"context"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pinger"
)

func (c *Collector) collect(ctx context.Context) error {
	samples := c.collectSamples(ctx, true)
	if len(samples) == 0 {
		return nil
	}

	vecMeter := c.store.Write().SnapshotMeter("").Vec("host")
	minRTT := vecMeter.Gauge("min_rtt")
	maxRTT := vecMeter.Gauge("max_rtt")
	avgRTT := vecMeter.Gauge("avg_rtt")
	stdDevRTT := vecMeter.Gauge("std_dev_rtt")
	rttVariance := vecMeter.Gauge("rtt_variance")
	meanJitter := vecMeter.Gauge("mean_jitter")
	ewmaJitter := vecMeter.Gauge("ewma_jitter")
	smaJitter := vecMeter.Gauge("sma_jitter")
	packetLoss := vecMeter.Gauge("packet_loss")
	packetsRecv := vecMeter.Gauge("packets_recv")
	packetsSent := vecMeter.Gauge("packets_sent")

	for _, sample := range samples {
		packetsRecv.WithLabelValues(sample.Host).Observe(float64(sample.PacketsRecv))
		packetsSent.WithLabelValues(sample.Host).Observe(float64(sample.PacketsSent))
		packetLoss.WithLabelValues(sample.Host).Observe(sample.PacketLossPct * 1000)

		if sample.RTT.Valid {
			minRTT.WithLabelValues(sample.Host).Observe(float64(sample.RTT.Min.Microseconds()))
			maxRTT.WithLabelValues(sample.Host).Observe(float64(sample.RTT.Max.Microseconds()))
			avgRTT.WithLabelValues(sample.Host).Observe(float64(sample.RTT.Avg.Microseconds()))
			stdDevRTT.WithLabelValues(sample.Host).Observe(float64(sample.RTT.StdDev.Microseconds()))
			rttVariance.WithLabelValues(sample.Host).Observe(float64(sample.RTT.VarianceMicrosecondsSquared()))
		}

		if sample.Jitter.InstantValid {
			meanJitter.WithLabelValues(sample.Host).Observe(float64(sample.Jitter.Mean.Microseconds()))
		}
		if sample.Jitter.SmoothedValid {
			ewmaJitter.WithLabelValues(sample.Host).Observe(float64(sample.Jitter.EWMA.Microseconds()))
			smaJitter.WithLabelValues(sample.Host).Observe(float64(sample.Jitter.SMA.Microseconds()))
		}
	}

	return nil
}

func (c *Collector) collectSamples(ctx context.Context, track bool) []pinger.Sample {
	if c.client == nil {
		return nil
	}

	var (
		mu      sync.Mutex
		samples = make([]pinger.Sample, 0, len(c.Hosts))
		wg      sync.WaitGroup
	)

	for _, host := range c.Hosts {
		wg.Go(func() {
			sample, err := c.probeHost(ctx, host, track)
			if err != nil {
				c.Error(err)
				return
			}

			mu.Lock()
			samples = append(samples, sample)
			mu.Unlock()
		})
	}

	wg.Wait()

	return samples
}

func (c *Collector) probeHost(ctx context.Context, host string, track bool) (pinger.Sample, error) {
	if track {
		return c.client.ProbeAndTrack(ctx, host)
	}
	return c.client.Probe(ctx, host)
}
