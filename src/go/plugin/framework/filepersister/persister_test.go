// SPDX-License-Identifier: GPL-3.0-or-later

package filepersister

import (
	"context"
	"errors"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := map[string]struct {
		path    string
		wantErr bool
	}{
		"empty filepath": {
			wantErr: true,
			path:    "",
		},
		"not empty filepath": {
			wantErr: false,
			path:    "testdata/test.json",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			p := New(test.path)
			require.NotNil(t, p)
		})
	}
}

func TestPersister_Run(t *testing.T) {
	tests := map[string]struct {
		wantErr  bool
		wantFile string
	}{
		"no save because data bytes error": {
			wantErr: true,
		},
		"successful save": {
			wantErr: false,
			wantFile: `
{
 "module1": {
  "name1:17896517344060997937": "ok"
 },
 "module2": {
  "name2:14519194242031159283": "ok"
 }
}
`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "netdata-go.d-test-filepersister-run")
			require.NoError(t, err)
			defer func() { assert.NoError(t, os.RemoveAll(dir)) }()

			filename := path.Join(dir, "filestatus")

			p := New(filename)

			data := newMockData(test.wantFile)
			data.wantError = test.wantErr

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				defer close(done)
				p.Run(ctx, data)
			}()

			cancel()

			timeout := time.Second * 5
			tk := time.NewTimer(timeout)
			defer tk.Stop()

			select {
			case <-done:
			case <-tk.C:
				t.Errorf("timed out after %s", timeout)
			}

			bs, err := os.ReadFile(filename)

			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, strings.TrimSpace(test.wantFile), strings.TrimSpace(string(bs)))
			}
		})
	}
}

func newMockData(s string) *mockData {
	m := &mockData{
		data: s,
		ch:   make(chan struct{}, 1),
	}
	m.ch <- struct{}{}

	return m
}

type mockData struct {
	data      string
	ch        chan struct{}
	wantError bool
}

func (m *mockData) Bytes() ([]byte, error) {
	if m.wantError {
		return nil, errors.New("mockData.Bytes() mock error")
	}
	return []byte(m.data), nil
}

func (m *mockData) Updated() <-chan struct{} {
	return m.ch
}
