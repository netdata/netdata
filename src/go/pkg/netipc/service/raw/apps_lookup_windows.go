//go:build windows

package raw

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

// NewAppsLookupClient creates a raw client bound to the apps-lookup service kind.
func NewAppsLookupClient(runDir, serviceName string, config windows.ClientConfig) *Client {
	return newClient(runDir, serviceName, config, protocol.MethodAppsLookup)
}
