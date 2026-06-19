package main

import (
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
)

type SocketLegacyConfig struct {
	PluginsDir    string
	Kernels       uint32
	IsRHF         int
	KernelVersion uint32
	IsDebian      bool
	HasBTF        bool
	ConfigFound   bool
	Enabled       bool
	UpdateEvery   int
	MapsPerCore   bool
	ObjectFlavor  string
	BTFPath       string
}

type SocketLegacyHandle struct {
	Plan        LoadPlan
	Runtime     *libbpfloader.SocketRuntime
	UpdateEvery int
	ConfigFound bool
	MapsPerCore bool
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
		PluginsDir:   defaultPluginsDir(),
		Kernels:      socketKernelMask,
		IsRHF:        -1,
		IsDebian:     IsDebianFlavor(),
		BTFPath:      socketDefaultBTFPath,
		UpdateEvery:  socketDefaultUpdateEvery,
		HasBTF:       kernelBTFSupported(socketDefaultBTFPath),
		MapsPerCore:  true,
		ObjectFlavor: socketDefaultObjectFlavor,
		Enabled:      false, // stock ebpf.d.conf: socket = no
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

	kver, isRHF, err := resolveKernelAndRH()
	if err != nil {
		return SocketLegacyConfig{}, err
	}
	cfg.KernelVersion = kver
	cfg.IsRHF = isRHF

	return cfg, nil
}

func BuildSocketLegacyPlan(cfg SocketLegacyConfig) LoadPlan {
	return buildKprobeLegacyPlan(kprobePlanRequest{
		PluginsDir:    cfg.PluginsDir,
		Kernels:       cfg.Kernels,
		IsRHF:         cfg.IsRHF,
		KernelVersion: cfg.KernelVersion,
		IsDebian:      cfg.IsDebian,
		HasBTF:        cfg.HasBTF,
		ObjectFlavor:  cfg.ObjectFlavor,
		Name:          "socket",
	})
}
