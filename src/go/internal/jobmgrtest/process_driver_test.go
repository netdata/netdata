package jobmgrtest

import (
	"context"
	"testing"
	"time"
)

func TestProcessDriverUsesPublicComposition(t *testing.T) {
	tests := map[string]struct {
		predicate string
	}{
		"restart": {
			predicate: "F05.2",
		},
		"input fence": {
			predicate: "F06.2",
		},
		"noncooperative shutdown": {
			predicate: "F05.3",
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
			if err := driver.Run(ctx, test.predicate); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCollectorBoundaryDriverTreatsCleanupAsOpaque(t *testing.T) {
	tests := map[string]struct {
		predicate string
	}{
		"cleanup once": {
			predicate: "F18.4",
		},
		"retained cleanup": {
			predicate: "F18.5",
		},
		"repeated held stop": {
			predicate: "F18.1",
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
			if err := driver.Run(ctx, test.predicate); err != nil {
				t.Fatal(err)
			}
		})
	}
}
