package timeperiod

import (
	"testing"
	"time"
)

func TestCompileAndAllows(t *testing.T) {
	cfgs := EnsureDefault([]Config{})
	set, err := Compile(cfgs)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	per, err := set.Resolve(DefaultPeriodName)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !per.Allows(time.Now()) {
		t.Fatalf("default period should allow now")
	}
}

func TestWeeklyRule(t *testing.T) {
	cfgs := []Config{
		{
			Name:  "work",
			Rules: []RuleConfig{{Type: "weekly", Days: []string{"monday"}, Ranges: []string{"09:00-17:00"}}},
		},
	}
	set, err := Compile(cfgs)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	per, _ := set.Resolve("work")
	mon := time.Date(2025, time.January, 6, 10, 0, 0, 0, time.UTC) // Monday
	if !per.Allows(mon) {
		t.Fatalf("expected monday 10:00 to be allowed")
	}
	sun := time.Date(2025, time.January, 5, 10, 0, 0, 0, time.UTC)
	if per.Allows(sun) {
		t.Fatalf("expected sunday to be disallowed")
	}
}

func TestExcludeCycles(t *testing.T) {
	cfgs := []Config{
		{
			Name:    "self",
			Rules:   []RuleConfig{{Type: "weekly", Ranges: []string{"00:00-24:00"}}},
			Exclude: []string{"self"},
		},
		{
			Name:    "a",
			Rules:   []RuleConfig{{Type: "weekly", Ranges: []string{"00:00-24:00"}}},
			Exclude: []string{"b"},
		},
		{
			Name:    "b",
			Rules:   []RuleConfig{{Type: "weekly", Ranges: []string{"00:00-24:00"}}},
			Exclude: []string{"a"},
		},
	}
	set, err := Compile(cfgs)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	self, _ := set.Resolve("self")
	if !self.Allows(time.Now()) {
		t.Fatalf("self-excluding period should still evaluate without errors")
	}
	a, _ := set.Resolve("a")
	if a.Allows(time.Now()) {
		t.Fatalf("mutually excluding periods should not allow any time")
	}
}
