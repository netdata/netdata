// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"context"
	"sync"
	"time"
)

type hostSample struct {
	host string

	packetsRecv       int64
	packetsSent       int64
	packetLossPercent float64

	hasRTT         bool
	minRTTUs       float64
	maxRTTUs       float64
	avgRTTUs       float64
	stdDevRTTUs    float64
	rttVarianceMS2 float64

	hasJitter    bool
	meanJitterUs float64
	ewmaJitterUs float64
	smaJitterUs  float64
}

func (c *Collector) collect(context.Context) error {
	samples := c.collectSamples(true)
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
		packetsRecv.WithLabelValues(sample.host).Observe(float64(sample.packetsRecv))
		packetsSent.WithLabelValues(sample.host).Observe(float64(sample.packetsSent))
		packetLoss.WithLabelValues(sample.host).Observe(sample.packetLossPercent)

		if sample.hasRTT {
			minRTT.WithLabelValues(sample.host).Observe(sample.minRTTUs)
			maxRTT.WithLabelValues(sample.host).Observe(sample.maxRTTUs)
			avgRTT.WithLabelValues(sample.host).Observe(sample.avgRTTUs)
			stdDevRTT.WithLabelValues(sample.host).Observe(sample.stdDevRTTUs)
			rttVariance.WithLabelValues(sample.host).Observe(sample.rttVarianceMS2)
		}

		if sample.hasJitter {
			meanJitter.WithLabelValues(sample.host).Observe(sample.meanJitterUs)
			ewmaJitter.WithLabelValues(sample.host).Observe(sample.ewmaJitterUs)
			smaJitter.WithLabelValues(sample.host).Observe(sample.smaJitterUs)
		}
	}

	return nil
}

func (c *Collector) collectSamples(updateJitterState bool) []hostSample {
	var (
		mu      sync.Mutex
		samples = make([]hostSample, 0, len(c.Hosts))
		wg      sync.WaitGroup
	)

	for _, host := range c.Hosts {
		host := host
		wg.Go(func() {
			stats, err := c.prober.Ping(host)
			if err != nil {
				c.Error(err)
				return
			}

			sample := hostSample{
				host:              host,
				packetsRecv:       int64(stats.PacketsRecv),
				packetsSent:       int64(stats.PacketsSent),
				packetLossPercent: stats.PacketLoss * 1000,
			}

			if stats.PacketsRecv != 0 {
				sample.hasRTT = true
				sample.minRTTUs = durToMicros(stats.MinRtt)
				sample.maxRTTUs = durToMicros(stats.MaxRtt)
				sample.avgRTTUs = durToMicros(stats.AvgRtt)
				sample.stdDevRTTUs = durToMicros(stats.StdDevRtt)
				stdDevUs := sample.stdDevRTTUs
				sample.rttVarianceMS2 = stdDevUs * stdDevUs
			}

			if len(stats.Rtts) >= 2 {
				meanJitter := calcMeanJitter(stats.Rtts)
				sample.hasJitter = true
				sample.meanJitterUs = durToMicros(meanJitter)

				if updateJitterState {
					mu.Lock()
					sample.ewmaJitterUs = durToMicros(c.updateEWMAJitter(host, meanJitter))
					sample.smaJitterUs = durToMicros(c.updateSMAJitter(host, meanJitter))
					mu.Unlock()
				}
			}

			mu.Lock()
			samples = append(samples, sample)
			mu.Unlock()
		})
	}
	wg.Wait()

	return samples
}

func durToMicros(v time.Duration) float64 {
	return float64(v.Microseconds())
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
