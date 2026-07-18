package jobmgrtest

import (
	"context"
	"testing"
	"time"
)

func TestProcessDriverUsesPublicComposition(t *testing.T) {
	tests := map[string]struct {
		scenario string
	}{
		"restart": {
			scenario: "restart",
		},
		"input fence": {
			scenario: "input-fence",
		},
		"noncooperative shutdown": {
			scenario: "noncooperative-shutdown",
		},
	}
	driver := &ProcessDriver{}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
			defer cancel()
			if err := driver.Run(ctx, test.scenario); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCollectorBoundaryDriverTreatsCleanupAsOpaque(t *testing.T) {
	tests := map[string]struct {
		scenario string
	}{
		"cleanup once": {
			scenario: "cleanup-once",
		},
		"retained cleanup": {
			scenario: "retained-cleanup",
		},
	}
	driver := &CollectorBoundaryDriver{}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				5*time.Second,
			)
			defer cancel()
			if err := driver.Run(ctx, test.scenario); err != nil {
				t.Fatal(err)
			}
		})
	}
}
