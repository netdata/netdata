// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

import (
	"errors"
	"fmt"
)

func (c *Collector) validateConfig() error {
	if c.Address == "" {
		c.Address = defaultAddress
	}
	if !c.Protocols.NetFlowV5 && !c.Protocols.NetFlowV9 && !c.Protocols.IPFIX {
		return errors.New("at least one protocol must be enabled")
	}
	if c.Aggregation.MaxPacketSize <= 0 {
		c.Aggregation.MaxPacketSize = defaultMaxPacketSize
	}
	if c.Aggregation.ReceiveBuffer < 0 {
		return fmt.Errorf("receive_buffer must be >= 0")
	}
	if c.Aggregation.MaxBuckets <= 0 {
		c.Aggregation.MaxBuckets = defaultMaxBuckets
	}
	if c.Aggregation.MaxKeys < 0 {
		return fmt.Errorf("max_keys must be >= 0")
	}
	if c.Sampling.DefaultRate <= 0 {
		c.Sampling.DefaultRate = defaultSamplingRate
	}
	for _, exp := range c.Exporters {
		if exp.IP == "" {
			return fmt.Errorf("exporters.ip is required")
		}
	}
	return nil
}
