// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"fmt"
	"sync"
)

func (p *Ping) collect() (map[string]int64, error) {
	mu := &sync.Mutex{}
	mx := make(map[string]int64)
	var wg sync.WaitGroup

	for _, v := range p.Hosts {
		wg.Add(1)
		go func(v string) { defer wg.Done(); p.pingHost(v, mx, mu) }(v)
	}
	wg.Wait()

	return mx, nil
}

func (p *Ping) pingHost(host string, mx map[string]int64, mu *sync.Mutex) {
	stats, err := p.prober.ping(host)
	if err != nil {
		p.Error(err)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if !p.hosts[host] {
		p.hosts[host] = true
		p.addHostCharts(host)
	}

	px := fmt.Sprintf("host_%s_", host)
	if stats.PacketsRecv != 0 {
		mx[px+"min_rtt"] = stats.MinRtt.Microseconds()
		mx[px+"max_rtt"] = stats.MaxRtt.Microseconds()
		mx[px+"avg_rtt"] = stats.AvgRtt.Microseconds()
		mx[px+"std_dev_rtt"] = stats.StdDevRtt.Microseconds()
	}
	mx[px+"packets_recv"] = int64(stats.PacketsRecv)
	mx[px+"packets_sent"] = int64(stats.PacketsSent)
	mx[px+"packet_loss"] = int64(stats.PacketLoss * 1000)
}
