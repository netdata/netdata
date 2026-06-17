//go:build unix

package raw

import "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"

func testLookupClientConfig() posix.ClientConfig {
	return testClientConfig()
}
