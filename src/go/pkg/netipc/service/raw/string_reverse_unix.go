//go:build unix

package raw

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

// NewStringReverseClient creates a raw client bound to the string-reverse service kind.
func NewStringReverseClient(runDir, serviceName string, config posix.ClientConfig) *Client {
	return newClient(runDir, serviceName, config, protocol.MethodStringReverse)
}
