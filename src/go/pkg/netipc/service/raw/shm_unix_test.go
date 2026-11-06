//go:build unix

package raw

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

func testUnixShmServerConfig() posix.ServerConfig {
	cfg := testServerConfig()
	cfg.SupportedProfiles = protocol.ProfileBaseline | protocol.ProfileSHMHybrid
	cfg.PreferredProfiles = protocol.ProfileSHMHybrid
	return cfg
}

func testUnixShmClientConfig() posix.ClientConfig {
	cfg := testClientConfig()
	cfg.SupportedProfiles = protocol.ProfileBaseline | protocol.ProfileSHMHybrid
	cfg.PreferredProfiles = protocol.ProfileSHMHybrid
	return cfg
}

func startTestServerUnixWithConfig(
	service string,
	cfg posix.ServerConfig,
	expectedMethodCode uint16,
	handler DispatchHandler,
) *testServer {
	ensureRunDir()
	cleanupAll(service)

	s := NewServer(testRunDir, service, cfg, expectedMethodCode, handler)
	doneCh := runServerForTest(s)
	return &testServer{server: s, doneCh: doneCh}
}
