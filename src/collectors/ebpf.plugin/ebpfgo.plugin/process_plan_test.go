package main

import "testing"

func TestBuildProcessLegacyPlanUsesBaseForPerPIDIntegration(t *testing.T) {
	cases := map[string]struct {
		apps    bool
		cgroups bool
	}{
		"apps":    {apps: true},
		"cgroups": {cgroups: true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := defaultProcessLegacyConfig()
			cfg.PluginsDir = ""
			cfg.KernelVersion = 397850 // 6.18
			cfg.AppsEnabled = tc.apps
			cfg.CgroupsEnabled = tc.cgroups
			cfg.HasBTF = true
			cfg.ObjectFlavor = "buffer"

			plan := BuildProcessLegacyPlan(cfg)
			if plan.Flavor != ObjectFlavorBase {
				t.Fatalf("plan flavor = %v, want base for per-PID integration", plan.Flavor)
			}
			if plan.ObjectPath != "ebpf.d/pnetdata_ebpf_process.6.12.o" {
				t.Fatalf("plan object = %q, want 6.12 base process object", plan.ObjectPath)
			}
		})
	}
}
