package main

import (
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// socketIntervalDelta returns the per-field difference between consecutive raw
// BPF counter readings.  Counter resets (current < prev) are clamped to zero.
func socketIntervalDelta(curr, prev ebpfSocketPublishApps) ebpfSocketPublishApps {
	clamp := func(c, p uint64) uint64 {
		if c >= p {
			return c - p
		}
		return 0
	}
	return ebpfSocketPublishApps{
		BytesSent:           clamp(curr.BytesSent, prev.BytesSent),
		BytesReceived:       clamp(curr.BytesReceived, prev.BytesReceived),
		CallTCPSent:         clamp(curr.CallTCPSent, prev.CallTCPSent),
		CallTCPReceived:     clamp(curr.CallTCPReceived, prev.CallTCPReceived),
		Retransmit:          clamp(curr.Retransmit, prev.Retransmit),
		CallUDPSent:         clamp(curr.CallUDPSent, prev.CallUDPSent),
		CallUDPReceived:     clamp(curr.CallUDPReceived, prev.CallUDPReceived),
		CallClose:           clamp(curr.CallClose, prev.CallClose),
		CallTCPV4Connection: clamp(curr.CallTCPV4Connection, prev.CallTCPV4Connection),
		CallTCPV6Connection: clamp(curr.CallTCPV6Connection, prev.CallTCPV6Connection),
		UpdateEverySec:      curr.UpdateEverySec,
	}
}

// UpdateSocketApps stores the latest per-PID socket snapshot and rebuilds the
// merged entry set.  Called by the socket collector each cycle.
//
// On the success path (entries != nil) this function computes per-interval
// deltas (raw BPF counter - previous raw BPF counter) before writing to
// socketData.  New PIDs emit zero on their first cycle to suppress the initial
// spike that would otherwise appear when they first enter the BPF map.  Exited
// PIDs are absent from entries and therefore absent from socketData, so the
// rebuild drops their socket contribution automatically.
//
// entries == nil signals a failed collection cycle: the socket contribution is
// cleared but the SOCKET flag is left untouched (the caller clears it via
// MarkSocketInactive).
func (s *ebpfSharedMemoryStore) UpdateSocketApps(entries []libbpfloader.SocketPIDEntry, updateEverySec uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	clear(s.socketData)

	// Set the SOCKET flag only on the success path (entries != nil).
	// clearSocketApps() passes nil to signal a failed collection cycle; the
	// caller clears the flag via MarkSocketInactive() before calling here.
	if entries != nil {
		s.activeModules |= ebpfgoSHMFlagSocket
		clear(s.nextPrevSocketData)
		for _, e := range entries {
			raw := ebpfSocketPublishApps{
				BytesSent:           e.BytesSent,
				BytesReceived:       e.BytesReceived,
				CallTCPSent:         e.CallTCPSent,
				CallTCPReceived:     e.CallTCPReceived,
				Retransmit:          e.Retransmit,
				CallUDPSent:         e.CallUDPSent,
				CallUDPReceived:     e.CallUDPReceived,
				CallClose:           e.CallClose,
				CallTCPV4Connection: e.CallTCPV4Connection,
				CallTCPV6Connection: e.CallTCPV6Connection,
				UpdateEverySec:      updateEverySec,
			}
			s.nextPrevSocketData[e.PID] = raw
			if prev, ok := s.prevSocketData[e.PID]; ok {
				s.socketData[e.PID] = socketIntervalDelta(raw, prev)
			} else {
				// New PID: emit zero this cycle; first-cycle spike suppressed.
				s.socketData[e.PID] = ebpfSocketPublishApps{}
			}
		}
		s.prevSocketData, s.nextPrevSocketData = s.nextPrevSocketData, s.prevSocketData
	}

	s.rebuildEntriesLocked()
}
