package main

import "testing"

func TestBuildProcessLegacyPlanUsesBaseForPerPIDIntegration(t *testing.T) {
	cases := map[string]struct {
		apps       bool
		cgroups    bool
		wantFlavor ObjectFlavor
		wantObject string
	}{
		"apps":    {apps: true, wantFlavor: ObjectFlavorBase, wantObject: "ebpf.d/rnetdata_ebpf_process.6.12.o"},
		"cgroups": {cgroups: true, wantFlavor: ObjectFlavorBase, wantObject: "ebpf.d/rnetdata_ebpf_process.6.12.o"},
		// Global-only collection keeps the configured flavor; only per-PID
		// consumers need the base object's tbl_pid_stats.
		"global only keeps the configured flavor": {
			wantFlavor: ObjectFlavorBuffer, wantObject: "ebpf.d/rnetdata_ebpf_process_buffer.6.12.o",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := defaultProcessLegacyConfig()
			cfg.PluginsDir = ""
			cfg.KernelVersion = 397850 // 6.18
			cfg.AppsEnabled = tc.apps
			cfg.CgroupsEnabled = tc.cgroups
			cfg.HasBTF = true
			cfg.IsDebian = false
			cfg.ObjectFlavor = "buffer"

			plan := BuildProcessLegacyPlan(cfg)
			if plan.Flavor != tc.wantFlavor || plan.ObjectPath != tc.wantObject {
				t.Fatalf("plan = (%v, %q), want (%v, %q)", plan.Flavor, plan.ObjectPath, tc.wantFlavor, tc.wantObject)
			}
		})
	}
}

// TestBuildProcessLegacyPlanSelectsShippedBaseObject pins the base object per
// kernel to the C module's list: no 5.11, 5.15, 5.16, 6.8 or 6.16.2 base object
// is shipped, the 5.14 objects are RHF-only for selection (SelectIndex), and 7.x
// kernels use the 7.0 object.
func TestBuildProcessLegacyPlanSelectsShippedBaseObject(t *testing.T) {
	cases := map[string]struct {
		kver uint32
		want string
	}{
		"5.12 uses 5.10":   {kver: 5<<16 | 12<<8, want: "ebpf.d/rnetdata_ebpf_process.5.10.o"},
		"5.15 uses 5.10":   {kver: 5<<16 | 15<<8, want: "ebpf.d/rnetdata_ebpf_process.5.10.o"},
		"6.8 uses 5.10":    {kver: 6<<16 | 8<<8, want: "ebpf.d/rnetdata_ebpf_process.5.10.o"},
		"6.16.2 uses 6.12": {kver: 6<<16 | 16<<8 | 2, want: "ebpf.d/rnetdata_ebpf_process.6.12.o"},
		"7.2 uses 7.0":     {kver: 7<<16 | 2<<8, want: "ebpf.d/rnetdata_ebpf_process.7.0.o"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := defaultProcessLegacyConfig()
			cfg.PluginsDir = ""
			cfg.KernelVersion = tc.kver
			cfg.AppsEnabled = true
			cfg.HasBTF = true

			if got := BuildProcessLegacyPlan(cfg).ObjectPath; got != tc.want {
				t.Fatalf("object = %q, want %q", got, tc.want)
			}
		})
	}
}
