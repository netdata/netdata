//go:build windows

package raw

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

func TestPrepareAcceptConfigUsesLearnedCapacities(t *testing.T) {
	cfg := windows.ServerConfig{
		SupportedProfiles:       protocol.ProfileBaseline,
		PreferredProfiles:       protocol.ProfileBaseline,
		MaxRequestPayloadBytes:  101,
		MaxResponsePayloadBytes: 202,
	}
	server := NewServer("run", "svc", cfg, protocol.MethodIncrement, nil)

	sessionID, acceptCfg := server.prepareAcceptConfig()
	if sessionID != 1 {
		t.Fatalf("session id = %d, want 1", sessionID)
	}
	if acceptCfg.MaxRequestPayloadBytes != cfg.MaxRequestPayloadBytes ||
		acceptCfg.MaxResponsePayloadBytes != cfg.MaxResponsePayloadBytes {
		t.Fatalf("accept config capacities = %d/%d, want %d/%d",
			acceptCfg.MaxRequestPayloadBytes,
			acceptCfg.MaxResponsePayloadBytes,
			cfg.MaxRequestPayloadBytes,
			cfg.MaxResponsePayloadBytes)
	}
}

func TestCreateShmFailsForInvalidWinShmService(t *testing.T) {
	cfg := windows.ServerConfig{
		SupportedProfiles:       windows.WinShmProfileHybrid | windows.WinShmProfileBusywait,
		PreferredProfiles:       windows.WinShmProfileHybrid | windows.WinShmProfileBusywait,
		MaxRequestPayloadBytes:  101,
		MaxResponsePayloadBytes: 202,
	}
	server := NewServer("run", "bad/name", cfg, protocol.MethodIncrement, nil)

	for _, profile := range []uint32{windows.WinShmProfileHybrid, windows.WinShmProfileBusywait} {
		if shm := server.createShm(1, profile, 101, 202); shm != nil {
			shm.WinShmDestroy()
			t.Fatalf("createShm(profile 0x%x) with an invalid service = %v, want nil", profile, shm)
		}
	}
}
