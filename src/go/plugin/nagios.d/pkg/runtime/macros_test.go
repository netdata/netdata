package runtime

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/nagios.d/pkg/spec"
)

func TestBuildMacroSet(t *testing.T) {
	ctx := MacroContext{
		Job: spec.JobSpec{
			Name:      "http_check",
			Plugin:    "/usr/lib/nagios/plugins/check_http",
			Args:      []string{"-H", "$HOSTADDRESS$", "-p", "$ARG1$", "-w", "$ARG2$"},
			ArgValues: []string{"8080", "5"},
			CustomVars: map[string]string{
				"ENDPOINT": "/health",
			},
		},
		UserMacros: map[string]string{"USER1": "/usr/lib/nagios/plugins"},
		Vnode:      VnodeInfo{Hostname: "web1", Address: "192.0.2.10", Alias: "web-node"},
		State:      StateInfo{ServiceState: "OK"},
	}

	s := BuildMacroSet(ctx)

	if got := s.Env["NAGIOS_HOSTADDRESS"]; got != "192.0.2.10" {
		t.Fatalf("host address macro mismatch: %s", got)
	}
	if got := s.Env["NAGIOS__SERVICEENDPOINT"]; got != "/health" {
		t.Fatalf("service custom var missing: %s", got)
	}

	if len(s.CommandArgs) != len(ctx.Job.Args) {
		t.Fatalf("arg length mismatch")
	}
	if s.CommandArgs[1] != "192.0.2.10" || s.CommandArgs[3] != "8080" {
		t.Fatalf("macro substitution failed: %v", s.CommandArgs)
	}
	if got := s.Env["NAGIOS_ARG1"]; got != "8080" {
		t.Fatalf("arg macro missing: %s", got)
	}
}
