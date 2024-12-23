// SPDX-License-Identifier: GPL-3.0-or-later

package rabbitmq

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Collector) collectNodes(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathAPINodes)
	if err != nil {
		return fmt.Errorf("failed to create node stats request: %w", err)
	}

	var resp []apiNodeResp

	if err := c.webClient().RequestJSON(req, &resp); err != nil {
		return err
	}

	for _, node := range resp {
		c.cache.getNode(node).seen = true

		px := fmt.Sprintf("node_%s_", node.Name)

		mx[px+"avail_status_running"] = metrix.Bool(node.Running)
		mx[px+"avail_status_down"] = metrix.Bool(!node.Running)

		if node.OsPid == "" {
			continue
		}

		for _, v := range []string{"clear", "detected"} {
			mx[px+"network_partition_status_"+v] = 0
		}
		mx[px+"network_partition_status_clear"] = metrix.Bool(len(node.Partitions) == 0)
		mx[px+"network_partition_status_detected"] = metrix.Bool(len(node.Partitions) > 0)

		mx[px+"mem_alarm_status_clear"] = metrix.Bool(!node.MemAlarm)
		mx[px+"mem_alarm_status_triggered"] = metrix.Bool(node.MemAlarm)
		mx[px+"disk_free_alarm_status_clear"] = metrix.Bool(!node.DiskFreeAlarm)
		mx[px+"disk_free_alarm_status_triggered"] = metrix.Bool(node.DiskFreeAlarm)

		mx[px+"fds_available"] = node.FDTotal - node.FDUsed
		mx[px+"fds_used"] = node.FDUsed
		mx[px+"mem_available"] = node.MemLimit - node.MemUsed
		mx[px+"mem_used"] = node.MemUsed
		mx[px+"sockets_available"] = node.SocketsTotal - node.SocketsUsed
		mx[px+"sockets_used"] = node.SocketsUsed
		mx[px+"procs_available"] = node.ProcTotal - node.ProcUsed
		mx[px+"procs_used"] = node.ProcUsed
		mx[px+"disk_free_bytes"] = node.DiskFree
		mx[px+"run_queue"] = node.RunQueue
		mx[px+"uptime"] = node.Uptime / 1000 // ms to seconds

		for _, peer := range node.ClusterLinks {
			c.cache.getNodeClusterPeer(node, peer).seen = true

			mx[px+"peer_"+peer.Name+"_cluster_link_recv_bytes"] = peer.RecvBytes
			mx[px+"peer_"+peer.Name+"_cluster_link_send_bytes"] = peer.SendBytes
		}
	}

	return nil
}
