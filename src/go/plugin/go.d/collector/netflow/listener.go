// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

import (
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

func (c *Collector) startListener() error {
	addr, err := net.ResolveUDPAddr("udp", c.Address)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to start UDP listener: %w", err)
	}

	if c.Aggregation.ReceiveBuffer > 0 {
		_ = conn.SetReadBuffer(c.Aggregation.ReceiveBuffer)
	}

	c.conn = conn
	c.wg.Add(1)
	go c.readLoop()
	return nil
}

func (c *Collector) readLoop() {
	defer c.wg.Done()

	buffer := make([]byte, c.Aggregation.MaxPacketSize)
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		if err := c.conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			continue
		}

		n, addr, err := c.conn.ReadFromUDP(buffer)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			c.Warningf("netflow read error: %v", err)
			continue
		}
		if n == 0 {
			continue
		}

		payload := make([]byte, n)
		copy(payload, buffer[:n])

		records, err := c.decoder.Decode(payload, addr.IP)
		if err != nil {
			c.aggregator.RecordDecodeError()
			c.Warningf("netflow decode error from %s: %v", addr.IP.String(), err)
			continue
		}
		if len(records) == 0 {
			continue
		}
		c.aggregator.AddRecords(records)
	}
}
