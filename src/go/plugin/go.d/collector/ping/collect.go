// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type v2Metrics struct {
	minRTT      metrix.SnapshotGaugeVec
	maxRTT      metrix.SnapshotGaugeVec
	avgRTT      metrix.SnapshotGaugeVec
	stdDevRTT   metrix.SnapshotGaugeVec
	rttVariance metrix.SnapshotGaugeVec
	meanJitter  metrix.SnapshotGaugeVec
	ewmaJitter  metrix.SnapshotGaugeVec
	smaJitter   metrix.SnapshotGaugeVec
	packetLoss  metrix.SnapshotGaugeVec
	packetsRecv metrix.SnapshotGaugeVec
	packetsSent metrix.SnapshotGaugeVec
}

type hostSample struct {
	host string

	packetsRecv       int64
	packetsSent       int64
	packetLossPercent float64

	hasRTT         bool
	minRTTMS       float64
	maxRTTMS       float64
	avgRTTMS       float64
	stdDevRTTMS    float64
	rttVarianceMS2 float64

	hasJitter    bool
	meanJitterMS float64
	ewmaJitterMS float64
	smaJitterMS  float64
}

func (c *Collector) collectSamples(updateJitterState bool) []hostSample {
	var (
		mu      sync.Mutex
		samples = make([]hostSample, 0, len(c.Hosts))
		wg      sync.WaitGroup
	)

	for _, host := range c.Hosts {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()

			stats, err := c.prober.Ping(host)
			if err != nil {
				c.Error(err)
				return
			}

			sample := hostSample{
				host:              host,
				packetsRecv:       int64(stats.PacketsRecv),
				packetsSent:       int64(stats.PacketsSent),
				packetLossPercent: stats.PacketLoss,
			}

			if stats.PacketsRecv != 0 {
				sample.hasRTT = true
				sample.minRTTMS = durationToMS(stats.MinRtt)
				sample.maxRTTMS = durationToMS(stats.MaxRtt)
				sample.avgRTTMS = durationToMS(stats.AvgRtt)
				sample.stdDevRTTMS = durationToMS(stats.StdDevRtt)
				stdDevMS := sample.stdDevRTTMS
				sample.rttVarianceMS2 = stdDevMS * stdDevMS
			}

			if len(stats.Rtts) >= 2 {
				meanJitter := calcMeanJitter(stats.Rtts)
				sample.hasJitter = true
				sample.meanJitterMS = durationToMS(meanJitter)

				if updateJitterState {
					mu.Lock()
					sample.ewmaJitterMS = durationToMS(c.updateEWMAJitter(host, meanJitter))
					sample.smaJitterMS = durationToMS(c.updateSMAJitter(host, meanJitter))
					mu.Unlock()
				}
			}

			mu.Lock()
			samples = append(samples, sample)
			mu.Unlock()
		}(host)
	}
	wg.Wait()

	return samples
}

func durationToMS(v time.Duration) float64 {
	return float64(v) / float64(time.Millisecond)
}

// calcMeanJitter calculates mean of absolute consecutive RTT differences
func calcMeanJitter(rtts []time.Duration) time.Duration {
	if len(rtts) < 2 {
		return 0
	}
	var sum int64
	for i := 1; i < len(rtts); i++ {
		diff := rtts[i] - rtts[i-1]
		if diff < 0 {
			diff = -diff
		}
		sum += int64(diff)
	}
	return time.Duration(sum / int64(len(rtts)-1))
}

// updateEWMAJitter updates exponentially weighted moving average jitter
// Formula: J(i) = α * current + (1-α) * J(i-1), where α = 1/N
func (c *Collector) updateEWMAJitter(host string, current time.Duration) time.Duration {
	prev := c.jitterEWMA[host]
	curr := float64(current)
	alpha := 1.0 / float64(c.JitterEWMASamples)
	ewma := alpha*curr + (1-alpha)*prev
	c.jitterEWMA[host] = ewma
	return time.Duration(ewma)
}

// updateSMAJitter updates simple moving average jitter over a sliding window
func (c *Collector) updateSMAJitter(host string, current time.Duration) time.Duration {
	window := c.jitterSMA[host]
	window = append(window, float64(current))
	if len(window) > c.JitterSMAWindow {
		window = window[1:]
	}
	c.jitterSMA[host] = window

	var sum float64
	for _, v := range window {
		sum += v
	}
	return time.Duration(sum / float64(len(window)))
}
