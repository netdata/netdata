// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package bgp

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_OpenBGPDPortableLiveEVPNLocalOriginationCLIUnsupported(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OpenBGPD portable live integration test requires Linux")
	}

	cfg := getOpenBGPDPortableIntegrationConfig(t)
	requireDocker(t)

	image := buildOpenBGPDPortableIntegrationImage(t, cfg)
	harness := newOpenBGPDPortableIntegrationHarnessWithConfig(t, image, openbgpdIntegrationConfigPortableExtendedFamilies)
	waitOpenBGPDPortableReady(t, harness)

	output, err := runDockerCommand(15*time.Second,
		"exec", harness.containerName,
		"sh", "-lc",
		"/build/src/bgpctl/bgpctl -j -s /run/bgpd.rsock network show EVPN",
	)
	require.Error(t, err)
	assert.Contains(t, output, "unknown argument: EVPN")
	assert.Contains(t, output, "[ inet | inet6 | IPv4 | IPv6 | VPNv4 | VPNv6 ]")
}
