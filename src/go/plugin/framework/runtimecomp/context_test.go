// SPDX-License-Identifier: GPL-3.0-or-later

package runtimecomp

import (
	"context"
	"testing"
)

type mockService struct{}

func (mockService) RegisterComponent(ComponentConfig) error { return nil }
func (mockService) UnregisterComponent(string)              {}
func (mockService) RegisterProducer(string, func() error) error {
	return nil
}
func (mockService) UnregisterProducer(string) {}

func TestContextHelpers(t *testing.T) {
	tests := map[string]struct {
		ctx     context.Context
		service Service
		wantOK  bool
	}{
		"nil context and nil service": {
			ctx:     nil,
			service: nil,
			wantOK:  false,
		},
		"background context without service": {
			ctx:     context.Background(),
			service: nil,
			wantOK:  false,
		},
		"context with service": {
			ctx:     context.Background(),
			service: mockService{},
			wantOK:  true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := ContextWithService(test.ctx, test.service)
			got, ok := ServiceFromContext(ctx)
			if ok != test.wantOK {
				t.Fatalf("ServiceFromContext() ok = %v, want %v", ok, test.wantOK)
			}
			if !test.wantOK {
				if got != nil {
					t.Fatalf("ServiceFromContext() service = %#v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("ServiceFromContext() service = nil, want non-nil")
			}
		})
	}
}
