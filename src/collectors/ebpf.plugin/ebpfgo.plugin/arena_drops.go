package main

import "github.com/netdata/netdata/go/plugins/pkg/netdataapi"

// Arena drops are cumulative in BPF; the incremental dimension reports events/s.
func createArenaDropsChart(api *netdataapi.API, module string, updateEvery int) {
	if api == nil {
		return
	}
	pluginOutputMu.Lock()
	defer pluginOutputMu.Unlock()

	api.HOST("")
	api.CHART(netdataapi.ChartOpts{
		TypeID:      "ebpf",
		ID:          module + "_arena_drops",
		Title:       "Dropped eBPF arena events",
		Units:       "events/s",
		Family:      "arena",
		Context:     "ebpf." + module + "_arena_drops",
		ChartType:   "line",
		Priority:    29000,
		UpdateEvery: updateEvery,
		Plugin:      "ebpf-go.plugin",
		Module:      module,
	})
	api.DIMENSION(netdataapi.DimensionOpts{
		ID:         "dropped",
		Name:       "dropped",
		Algorithm:  "incremental",
		Multiplier: 1,
		Divisor:    1,
	})
}

func publishArenaDrops(api *netdataapi.API, module string, dropped uint64, usecSince int) {
	if api == nil {
		return
	}
	pluginOutputMu.Lock()
	defer pluginOutputMu.Unlock()

	api.BEGIN("ebpf", module+"_arena_drops", usecSince)
	api.SET("dropped", int64(dropped))
	api.END()
}
