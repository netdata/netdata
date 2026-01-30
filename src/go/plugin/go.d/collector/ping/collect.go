// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"fmt"
	"sync"
	"time"
)

func (c *Collector) collect() (map[string]int64, error) {
	mu := &sync.Mutex{}
	mx := make(map[string]int64)
	var wg sync.WaitGroup

	for _, v := range c.Hosts {
		wg.Add(1)
		go func(v string) { defer wg.Done(); c.pingHost(v, mx, mu) }(v)
	}
	wg.Wait()

	return mx, nil
}

func (c *Collector) pingHost(host string, mx map[string]int64, mu *sync.Mutex) {
	stats, err := c.prober.Ping(host)
	if err != nil {
		c.Error(err)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if !c.hosts[host] {
		c.hosts[host] = true
		c.addHostCharts(host)
	}

	px := fmt.Sprintf("host_%s_", host)
	if stats.PacketsRecv != 0 {
		mx[px+"min_rtt"] = stats.MinRtt.Microseconds()
		mx[px+"max_rtt"] = stats.MaxRtt.Microseconds()
		mx[px+"avg_rtt"] = stats.AvgRtt.Microseconds()
		mx[px+"std_dev_rtt"] = stats.StdDevRtt.Microseconds()

		// variance = stddev² stored as μs² (chart Div: 1e6 converts to ms²)
		stdDevUs := stats.StdDevRtt.Microseconds()
		mx[px+"rtt_variance"] = stdDevUs * stdDevUs
	}

	// jitter requires at least 2 RTT samples
	if len(stats.Rtts) >= 2 {
		meanJitter := calcMeanJitter(stats.Rtts)
		mx[px+"mean_jitter"] = meanJitter.Microseconds()
		mx[px+"ewma_jitter"] = c.updateEWMAJitter(host, meanJitter).Microseconds()
		mx[px+"sma_jitter"] = c.updateSMAJitter(host, meanJitter).Microseconds()
	}

	mx[px+"packets_recv"] = int64(stats.PacketsRecv)
	mx[px+"packets_sent"] = int64(stats.PacketsSent)
	mx[px+"packet_loss"] = int64(stats.PacketLoss * 1000)
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
