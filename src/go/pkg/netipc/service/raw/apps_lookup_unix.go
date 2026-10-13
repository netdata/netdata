//go:build unix

package raw

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

// NewAppsLookupClient creates a raw client bound to the apps-lookup service kind.
func NewAppsLookupClient(runDir, serviceName string, config posix.ClientConfig) *Client {
	return newClient(runDir, serviceName, config, protocol.MethodAppsLookup)
}
