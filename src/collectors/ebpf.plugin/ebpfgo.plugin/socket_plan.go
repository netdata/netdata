package main

import (
	"fmt"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

const (
	socketKernelMask          uint32 = (1 << 12) - 1
	socketDefaultUpdateEvery         = 10
	socketDefaultBTFPath             = "/sys/kernel/btf"
	socketDefaultObjectFlavor        = "buffer"
	// socketDefaultPIDTableSize matches cachestatDefaultPIDTableSize so both
	// modules produce an identically-sized SHM segment.
	socketDefaultPIDTableSize uint32 = 32768
	// socketDefaultMonitoringTableSize is the default max_entries for tbl_nd_socket.
	// Matches the legacy C plugin default (NETDATA_MAXIMUM_CONNECTIONS_ALLOWED).
	socketDefaultMonitoringTableSize uint32 = 16384
	// socketMaxMonitoringTableSize bounds userspace pre-allocation for
	// tbl_nd_socket snapshots. Larger values can allocate hundreds of GB before
	// the BPF load path gets a chance to reject the map size.
	socketMaxMonitoringTableSize uint32 = 65536
	// socketDefaultUDPConnectionTableSize is the default max_entries for tbl_nv_udp.
	// Matches the legacy C plugin default (NETDATA_MAXIMUM_UDP_CONNECTIONS_ALLOWED).
	socketDefaultUDPConnectionTableSize uint32 = 4096
	socketMaxUDPConnectionTableSize     uint32 = 65536
	// socketMaxBaseSelector is the highest SelectKernelName index for which a
	// base-flavor (no suffix) socket object file is shipped.  Objects for newer
	// kernels use the buffer or arena flavor only.
	socketMaxBaseSelector = 7 // 5.14
)

type SocketLegacyConfig struct {
	PluginsDir                string
	Kernels                   uint32
	IsRHF                     int
	KernelVersion             uint32
	IsDebian                  bool
	HasBTF                    bool
	ConfigFound               bool
	Enabled                   bool
	UpdateEvery               int
	MapsPerCore               bool
	ObjectFlavor              string
	BTFPath                   string
	SocketMonitoringTableSize uint32 // max_entries for tbl_nd_socket
	UDPConnectionTableSize    uint32 // max_entries for tbl_nv_udp
	PidTableSize              uint32 // max rows in the per-PID SHM segment
}

type SocketLegacyHandle struct {
	Plan         LoadPlan
	Runtime      *libbpfloader.SocketRuntime
	UpdateEvery  int
	ConfigFound  bool
	MapsPerCore  bool
	PidTableSize uint32
}

func (h *SocketLegacyHandle) Close() {
	if h == nil || h.Runtime == nil {
		return
	}
	h.Runtime.Close()
	h.Runtime = nil
}

func defaultSocketLegacyConfig() SocketLegacyConfig {
	return SocketLegacyConfig{
		PluginsDir:                defaultPluginsDir(),
		Kernels:                   socketKernelMask,
		IsRHF:                     -1,
		IsDebian:                  IsDebianFlavor(),
		BTFPath:                   socketDefaultBTFPath,
		UpdateEvery:               socketDefaultUpdateEvery,
		HasBTF:                    kernelBTFSupported(socketDefaultBTFPath),
		MapsPerCore:               true,
		ObjectFlavor:              socketDefaultObjectFlavor,
		Enabled:                   false, // stock ebpf.d.conf: socket = no
		SocketMonitoringTableSize: socketDefaultMonitoringTableSize,
		UDPConnectionTableSize:    socketDefaultUDPConnectionTableSize,
		PidTableSize:              socketDefaultPIDTableSize,
	}
}

func resolveSocketLegacyConfig() (SocketLegacyConfig, error) {
	cfg := defaultSocketLegacyConfig()

	fileCfg, found, err := loadSocketConfigFiles()
	if err != nil {
		return SocketLegacyConfig{}, err
	}
	cfg.ConfigFound = found
	if fileCfg.Socket != nil {
		cfg.Enabled = *fileCfg.Socket
	}
	if fileCfg.UpdateEvery != nil && *fileCfg.UpdateEvery > 0 {
		cfg.UpdateEvery = *fileCfg.UpdateEvery
	}
	if fileCfg.MapsPerCore != nil {
		cfg.MapsPerCore = *fileCfg.MapsPerCore
	}
	if fileCfg.BTFPath != nil && *fileCfg.BTFPath != "" {
		cfg.BTFPath = *fileCfg.BTFPath
		cfg.HasBTF = kernelBTFSupported(cfg.BTFPath)
	}
	if fileCfg.ObjectFlavor != nil && *fileCfg.ObjectFlavor != "" {
		cfg.ObjectFlavor = *fileCfg.ObjectFlavor
	}
	if fileCfg.SocketMonitoringTableSize != nil && *fileCfg.SocketMonitoringTableSize > 0 {
		cfg.SocketMonitoringTableSize = applySocketTableSizeClamp(
			*fileCfg.SocketMonitoringTableSize,
			socketMaxMonitoringTableSize,
			"socket monitoring table size")
	}
	if fileCfg.UDPConnectionTableSize != nil && *fileCfg.UDPConnectionTableSize > 0 {
		cfg.UDPConnectionTableSize = applySocketTableSizeClamp(
			*fileCfg.UDPConnectionTableSize,
			socketMaxUDPConnectionTableSize,
			"udp connection table size")
	}
	if fileCfg.PidTable != nil && *fileCfg.PidTable > 0 {
		cfg.PidTableSize = applyPidTableSizeClamp(*fileCfg.PidTable)
	}

	kver, isRHF, err := resolveKernelAndRH()
	if err != nil {
		return SocketLegacyConfig{}, err
	}
	cfg.KernelVersion = kver
	cfg.IsRHF = isRHF

	return cfg, nil
}

func applySocketTableSizeClamp(value, max uint32, name string) uint32 {
	if value <= max {
		return value
	}
	logPluginErr("socket.config", "socket", name, fmt.Errorf("configured value %d exceeds maximum %d; clamping", value, max))
	return max
}

func BuildSocketLegacyPlan(cfg SocketLegacyConfig) LoadPlan {
	return buildKprobeLegacyPlan(kprobePlanRequest{
		PluginsDir:      cfg.PluginsDir,
		Kernels:         cfg.Kernels,
		IsRHF:           cfg.IsRHF,
		KernelVersion:   cfg.KernelVersion,
		IsDebian:        cfg.IsDebian,
		HasBTF:          cfg.HasBTF,
		ObjectFlavor:    cfg.ObjectFlavor,
		Name:            "socket",
		MaxBaseSelector: socketMaxBaseSelector,
	})
}
