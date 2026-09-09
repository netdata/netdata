//go:build windows

package raw

import windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"

func testLookupClientConfig() windows.ClientConfig {
	return testWinClientConfig()
}
