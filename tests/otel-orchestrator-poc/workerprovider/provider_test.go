// SPDX-License-Identifier: GPL-3.0-or-later
package workerprovider

import (
	"context"
	"io"
	"testing"
	"time"

	"go.opentelemetry.io/collector/confmap"
)

func TestParentLifetime(t *testing.T) {
	for _, parentCloses := range []bool{true, false} {
		t.Run(map[bool]string{true: "parent EOF", false: "validation cleanup"}[parentCloses], func(t *testing.T) {
			r, w := io.Pipe()
			defer r.Close()
			defer w.Close()
			p := &provider{input: r}
			go func() {
				_, _ = io.WriteString(w, "{\"processors\":{\"resource\":{\"value\":\"${env:PRIVATE} $literal\"}}}\n")
			}()
			changed := make(chan *confmap.ChangeEvent, 1)
			retrieved, err := p.Retrieve(
				context.Background(),
				"worker:stdin",
				func(e *confmap.ChangeEvent) { changed <- e },
			)
			if err != nil {
				t.Fatal(err)
			}
			config, err := retrieved.AsConf()
			if err != nil {
				t.Fatal(err)
			}
			if got := config.Get("processors::resource::value"); got != "$${env:PRIVATE} $$literal" {
				t.Fatalf("literal escaped incorrectly: %v", got)
			}
			if parentCloses {
				w.Close()
				select {
				case event := <-changed:
					if event.Error == nil {
						t.Fatal("EOF must terminate, not reload")
					}
				case <-time.After(time.Second):
					t.Fatal("missing parent EOF notification")
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := retrieved.Close(ctx); err != nil {
				t.Fatal(err)
			}
			if err := p.Shutdown(ctx); err != nil && err != io.ErrClosedPipe {
				t.Fatal(err)
			}
			if !parentCloses {
				select {
				case <-changed:
					t.Fatal("cleanup emitted spurious parent failure")
				default:
				}
			}
		})
	}
}
