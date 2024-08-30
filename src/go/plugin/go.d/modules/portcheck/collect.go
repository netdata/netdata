// SPDX-License-Identifier: GPL-3.0-or-later

package portcheck

import (
	"fmt"
	"sync"
	"time"
)

type checkState string

const (
	checkStateSuccess checkState = "success"
	checkStateTimeout checkState = "timeout"
	checkStateFailed  checkState = "failed"
)

func (pc *PortCheck) collect() (map[string]int64, error) {
	wg := &sync.WaitGroup{}

	for _, p := range pc.ports {
		wg.Add(1)
		go func(p *port) { pc.checkPort(p); wg.Done() }(p)
	}
	wg.Wait()

	mx := make(map[string]int64)

	for _, p := range pc.ports {
		mx[fmt.Sprintf("port_%d_current_state_duration", p.number)] = int64(p.inState)
		mx[fmt.Sprintf("port_%d_latency", p.number)] = int64(p.latency)
		mx[fmt.Sprintf("port_%d_%s", p.number, checkStateSuccess)] = 0
		mx[fmt.Sprintf("port_%d_%s", p.number, checkStateTimeout)] = 0
		mx[fmt.Sprintf("port_%d_%s", p.number, checkStateFailed)] = 0
		mx[fmt.Sprintf("port_%d_%s", p.number, p.state)] = 1
	}

	return mx, nil
}

func (pc *PortCheck) checkPort(p *port) {
	start := time.Now()
	conn, err := pc.dial("tcp", fmt.Sprintf("%s:%d", pc.Host, p.number), pc.Timeout.Duration())
	dur := time.Since(start)

	defer func() {
		if conn != nil {
			_ = conn.Close()
		}
	}()

	if err != nil {
		v, ok := err.(interface{ Timeout() bool })
		if ok && v.Timeout() {
			pc.setPortState(p, checkStateTimeout)
		} else {
			pc.setPortState(p, checkStateFailed)
		}
		return
	}
	pc.setPortState(p, checkStateSuccess)
	p.latency = durationToMs(dur)
}

func (pc *PortCheck) setPortState(p *port, s checkState) {
	if p.state != s {
		p.inState = pc.UpdateEvery
		p.state = s
	} else {
		p.inState += pc.UpdateEvery
	}
}

func durationToMs(duration time.Duration) int {
	return int(duration) / (int(time.Millisecond) / int(time.Nanosecond))
}
