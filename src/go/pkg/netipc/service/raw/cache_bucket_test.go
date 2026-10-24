package raw

import (
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func TestCacheBucketCountForItemCount(t *testing.T) {
	tests := []struct {
		name      string
		itemCount uint32
		want      uint32
		wantErr   bool
	}{
		{name: "empty", itemCount: 0, want: 0},
		{name: "one", itemCount: 1, want: 16},
		{name: "eight", itemCount: 8, want: 16},
		{name: "nine", itemCount: 9, want: 32},
		{name: "overflow", itemCount: (1 << 30) + 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cacheBucketCountForItemCount(tt.itemCount)
			if tt.wantErr {
				if !errors.Is(err, protocol.ErrOverflow) {
					t.Fatalf("expected ErrOverflow, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("bucket count = %d, want %d", got, tt.want)
			}
		})
	}
}
