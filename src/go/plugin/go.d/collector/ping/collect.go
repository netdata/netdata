// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"fmt"
	"sync"
)

func (c *Collector) collect() (map[string]float64, error) {
	mu := &sync.Mutex{}
	mx := make(map[string]float64)
	var wg sync.WaitGroup

	for _, v := range c.Hosts {
		wg.Add(1)
		go func(v string) { defer wg.Done(); c.pingHost(v, mx, mu) }(v)
	}
	wg.Wait()

	return mx, nil
}

func (c *Collector) pingHost(host string, mx map[string]float64, mu *sync.Mutex) {
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
		mx[px+"min_rtt"] = stats.MinRtt.Seconds()
		mx[px+"max_rtt"] = stats.MaxRtt.Seconds()
		mx[px+"avg_rtt"] = stats.AvgRtt.Seconds()
		mx[px+"std_dev_rtt"] = stats.StdDevRtt.Seconds()
	}
	mx[px+"packets_recv"] = float64(stats.PacketsRecv)
	mx[px+"packets_sent"] = float64(stats.PacketsSent)
	mx[px+"packet_loss"] = stats.PacketLoss
}
