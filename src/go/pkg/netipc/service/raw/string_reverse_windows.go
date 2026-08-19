//go:build windows

package raw

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

// NewStringReverseClient creates a raw client bound to the string-reverse service kind.
func NewStringReverseClient(runDir, serviceName string, config windows.ClientConfig) *Client {
	return newClient(runDir, serviceName, config, protocol.MethodStringReverse)
}
