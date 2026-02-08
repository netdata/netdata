// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

import (
	"errors"
	"fmt"
	"strings"
)

func (c *Collector) validateConfig() error {
	if c.Address == "" {
		c.Address = defaultAddress
	}
	if !c.Protocols.NetFlowV5 && !c.Protocols.NetFlowV9 && !c.Protocols.IPFIX && !c.Protocols.SFlow {
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
	if c.Flows.SummaryAggregation == "" {
		c.Flows.SummaryAggregation = defaultSummaryAgg
	} else {
		c.Flows.SummaryAggregation = strings.ToLower(c.Flows.SummaryAggregation)
	}
	switch c.Flows.SummaryAggregation {
	case summaryAggIP, summaryAggIPProtocol, summaryAggFiveTuple:
	default:
		return fmt.Errorf("flows.summary_aggregation must be one of %q, %q, %q", summaryAggIP, summaryAggIPProtocol, summaryAggFiveTuple)
	}
	if c.Flows.LiveTopN.Limit <= 0 {
		c.Flows.LiveTopN.Limit = defaultLiveTopNLimit
	}
	if c.Flows.LiveTopN.SortBy == "" {
		c.Flows.LiveTopN.SortBy = defaultLiveTopNSort
	} else {
		c.Flows.LiveTopN.SortBy = strings.ToLower(c.Flows.LiveTopN.SortBy)
	}
	switch c.Flows.LiveTopN.SortBy {
	case "bytes", "packets":
	default:
		return fmt.Errorf("flows.live_top_n.sort_by must be one of %q, %q", "bytes", "packets")
	}
	for _, exp := range c.Exporters {
		if exp.IP == "" {
			return fmt.Errorf("exporters.ip is required")
		}
	}
	return nil
}
