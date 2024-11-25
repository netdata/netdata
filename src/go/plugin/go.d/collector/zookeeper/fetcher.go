// SPDX-License-Identifier: GPL-3.0-or-later

package zookeeper

import (
	"bytes"
	"fmt"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

const limitReadLines = 2000

type zookeeperFetcher struct {
	socket.Client
}

func (c *zookeeperFetcher) fetch(command string) (rows []string, err error) {
	if err = c.Connect(); err != nil {
		return nil, err
	}
	defer func() { _ = c.Disconnect() }()

	var num int
	clientErr := c.Command(command, func(b []byte) bool {
		if !isZKLine(b) || isMntrLineOK(b) {
			rows = append(rows, string(b))
		}
		if num += 1; num >= limitReadLines {
			err = fmt.Errorf("read line limit exceeded (%d)", limitReadLines)
			return false
		}
		return true
	})
	if clientErr != nil {
		return nil, clientErr
	}
	if err != nil {
		return nil, err
	}

	return rows, nil
}

func isZKLine(line []byte) bool {
	return bytes.HasPrefix(line, []byte("zk_"))
}

func isMntrLineOK(line []byte) bool {
	idx := bytes.LastIndexByte(line, '\t')
	return idx > 0 && collectedZKKeys[unsafeString(line)[:idx]]
}

func unsafeString(b []byte) string {
	return *((*string)(unsafe.Pointer(&b)))
}

var collectedZKKeys = map[string]bool{
	"zk_num_alive_connections":      true,
	"zk_outstanding_requests":       true,
	"zk_min_latency":                true,
	"zk_avg_latency":                true,
	"zk_max_latency":                true,
	"zk_packets_received":           true,
	"zk_packets_sent":               true,
	"zk_open_file_descriptor_count": true,
	"zk_max_file_descriptor_count":  true,
	"zk_znode_count":                true,
	"zk_ephemerals_count":           true,
	"zk_watch_count":                true,
	"zk_approximate_data_size":      true,
	"zk_server_state":               true,
}
