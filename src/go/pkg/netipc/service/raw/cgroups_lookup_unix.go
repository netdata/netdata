//go:build unix

package raw

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

// NewCgroupsLookupClient creates a raw client bound to the cgroups-lookup service kind.
func NewCgroupsLookupClient(runDir, serviceName string, config posix.ClientConfig) *Client {
	return newClient(runDir, serviceName, config, protocol.MethodCgroupsLookup)
}
