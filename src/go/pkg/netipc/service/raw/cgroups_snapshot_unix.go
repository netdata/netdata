//go:build unix

package raw

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

// NewSnapshotClient creates a raw client bound to the cgroups-snapshot service kind.
func NewSnapshotClient(runDir, serviceName string, config posix.ClientConfig) *Client {
	return newClient(runDir, serviceName, config, protocol.MethodCgroupsSnapshot)
}
