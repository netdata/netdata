package spec

import "testing"

func TestJobConfigDefaults(t *testing.T) {
	cfg := JobConfig{Name: "sample", Plugin: "/usr/lib/nagios/plugins/check_ping"}
	cfg.SetDefaults()

	if cfg.Timeout == 0 {
		t.Fatalf("expected timeout to be set")
	}
	if cfg.CheckInterval == 0 || cfg.RetryInterval == 0 {
		t.Fatalf("expected intervals to be set")
	}
	if cfg.MaxCheckAttempts == 0 {
		t.Fatalf("expected max attempts to be non-zero")
	}
	if cfg.CheckPeriod == "" {
		t.Fatalf("expected check period to default")
	}
}

func TestJobConfigValidate(t *testing.T) {
	cfg := JobConfig{Name: "sample", Plugin: "/bin/true"}
	cfg.SetDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestJobConfigInvalidTimeoutState(t *testing.T) {
	cfg := JobConfig{Name: "sample", Plugin: "/bin/true", TimeoutState: "bogus"}
	cfg.SetDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestJobConfigArgValuesLimit(t *testing.T) {
	cfg := JobConfig{Name: "sample", Plugin: "/bin/true"}
	for i := 0; i < MaxArgMacros+1; i++ {
		cfg.ArgValues = append(cfg.ArgValues, "value")
	}
	cfg.SetDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error when arg_values exceed limit")
	}
}
