//go:build windows

package raw

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

func TestPreparedWinShmNilAndUnknownProfile(t *testing.T) {
	var nilPrepared *preparedWinShm
	if got := nilPrepared.take(windows.WinShmProfileHybrid); got != nil {
		t.Fatalf("nil prepared take = %v, want nil", got)
	}
	nilPrepared.destroyAll()

	prepared := &preparedWinShm{}
	if got := prepared.take(protocol.ProfileBaseline); got != nil {
		t.Fatalf("unknown prepared profile take = %v, want nil", got)
	}
	prepared.destroyAll()
}

func TestPreparedWinShmTakeProfiles(t *testing.T) {
	hybrid := &windows.WinShmContext{}
	busywait := &windows.WinShmContext{}
	prepared := &preparedWinShm{hybrid: hybrid, busywait: busywait}

	if got := prepared.take(windows.WinShmProfileHybrid); got != hybrid {
		t.Fatalf("hybrid take = %v, want %v", got, hybrid)
	}
	if prepared.hybrid != nil {
		t.Fatalf("hybrid context should be consumed")
	}

	if got := prepared.take(windows.WinShmProfileBusywait); got != busywait {
		t.Fatalf("busywait take = %v, want %v", got, busywait)
	}
	if prepared.busywait != nil {
		t.Fatalf("busywait context should be consumed")
	}
}

func TestPrepareAcceptConfigWithoutWinShm(t *testing.T) {
	cfg := windows.ServerConfig{
		SupportedProfiles:       protocol.ProfileBaseline,
		PreferredProfiles:       protocol.ProfileBaseline,
		MaxRequestPayloadBytes:  101,
		MaxResponsePayloadBytes: 202,
	}
	server := NewServer("run", "svc", cfg, protocol.MethodIncrement, nil)

	sessionID, acceptCfg, prepared, ok := server.prepareAcceptConfig()
	if !ok || prepared != nil {
		t.Fatalf("prepareAcceptConfig = ok %v prepared %v, want true/nil", ok, prepared)
	}
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

func TestPrepareAcceptConfigRejectsInvalidWinShmService(t *testing.T) {
	cfg := windows.ServerConfig{
		SupportedProfiles:       windows.WinShmProfileHybrid | windows.WinShmProfileBusywait,
		PreferredProfiles:       windows.WinShmProfileHybrid | windows.WinShmProfileBusywait,
		MaxRequestPayloadBytes:  101,
		MaxResponsePayloadBytes: 202,
	}
	server := NewServer("run", "bad/name", cfg, protocol.MethodIncrement, nil)

	sessionID, acceptCfg, prepared, ok := server.prepareAcceptConfig()
	if ok || prepared != nil {
		t.Fatalf("prepareAcceptConfig = ok %v prepared %v, want false/nil", ok, prepared)
	}
	if sessionID != 1 {
		t.Fatalf("session id = %d, want 1", sessionID)
	}
	if acceptCfg.SupportedProfiles != 0 || acceptCfg.PreferredProfiles != 0 {
		t.Fatalf("profiles = supported 0x%x preferred 0x%x, want both zero",
			acceptCfg.SupportedProfiles,
			acceptCfg.PreferredProfiles)
	}
}
