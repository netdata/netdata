// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type normalTestCut struct {
	attempt uint64
	capture func()
}

func (c *normalTestCut) CaptureNormal() (*NormalDevice, error) {
	if c.capture != nil {
		c.capture()
	}
	now := time.Now().UTC()
	return &NormalDevice{Hostname: "switch.example", CapturedAt: now,
		Latest: &NormalAttempt{ID: c.attempt, Phase: "collect", StartedAt: now, CompletedAt: now},
	}, nil
}

func TestNormalPublicationOwnershipAndProgress(t *testing.T) {
	for name, tc := range map[string]struct {
		coalesce, replace, fail, concurrentUpdate bool
	}{
		"pending polls coalesce":                   {coalesce: true},
		"replacement fences in flight rename":      {replace: true},
		"failed device does not block another":     {fail: true},
		"new updates cannot starve the finite cut": {concurrentUpdate: true},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := NewPublisher(ddsnmp.NewDeviceStore(), t.TempDir())
				first := p.ReplaceNormal("", "first", 1)
				second := p.ReplaceNormal("", "second", 2)
				cut := &normalTestCut{attempt: 1}
				first.Update(cut)
				if tc.coalesce {
					first.Update(&normalTestCut{attempt: 2})
					first.Update(&normalTestCut{attempt: 3})
				}
				second.Update(&normalTestCut{attempt: 1})
				if tc.concurrentUpdate {
					cut.capture = func() { first.Update(&normalTestCut{attempt: 4}) }
				}
				write := p.writeFile
				var published []uint64
				failed := false
				p.writeFile = func(ctx context.Context, path string, document Document, replace func(string, string) error) error {
					if document.Normal.RegistrationID == 1 {
						if tc.replace && document.Normal.RuntimeID == first.runtime {
							successor := p.ReplaceNormal("first", "first", 1)
							successor.Update(&normalTestCut{attempt: 5})
						}
						if tc.fail && !failed {
							failed = true
							return errors.New("disk unavailable for first device")
						}
					}
					if err := write(ctx, path, document, replace); err != nil {
						return err
					}
					published = append(published, document.Normal.RegistrationID)
					return nil
				}
				err := p.publishNormal(t.Context())
				if tc.fail {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				if tc.replace || tc.fail {
					assert.Equal(t, []uint64{2}, published)
				} else {
					assert.Equal(t, []uint64{1, 2}, published)
				}
				if tc.coalesce {
					d := readFile(t, filepath.Join(p.directory, NormalDirectory, p.runID, first.filename()))
					assert.EqualValues(t, 3, d.Normal.Latest.ID)
				}
				time.Sleep(normalPublicationEvery)
				require.NoError(t, p.publishNormal(t.Context()))
				files, err := ListNormalFiles(p.directory)
				require.NoError(t, err)
				require.Len(t, files, 2)
				if tc.replace {
					d := readFile(t, filepath.Join(p.directory, NormalDirectory, p.runID, first.filename()))
					require.NotEqual(t, first.runtime, d.Normal.RuntimeID)
					require.EqualValues(t, 5, d.Normal.Latest.ID)
					first.Update(&normalTestCut{attempt: 99})
					require.NoError(t, p.publishNormal(t.Context()))
					assert.Empty(t, p.normal.queue)
				}
				if tc.concurrentUpdate {
					d := readFile(t, filepath.Join(p.directory, NormalDirectory, p.runID, first.filename()))
					assert.EqualValues(t, 4, d.Normal.Latest.ID)
				}
			})
		})
	}
}

func TestNormalRunRetentionAndRetry(t *testing.T) {
	for name, tc := range map[string]struct{ failWrite, failActivation, failCleanup bool }{
		"successful run":                           {},
		"device write fails before activation":     {failWrite: true},
		"activation fails after complete file":     {failActivation: true},
		"old run cleanup is retried independently": {failCleanup: true},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				root := t.TempDir()
				publish := func(p *Publisher) {
					p.ReplaceNormal("", "device", 1).Update(&normalTestCut{attempt: 1})
					require.NoError(t, p.publishNormal(t.Context()))
				}
				old := NewPublisher(ddsnmp.NewDeviceStore(), root)
				publish(old)
				empty := NewPublisher(ddsnmp.NewDeviceStore(), root)
				require.NoError(t, empty.publishNormal(t.Context()))
				runs, err := ReadNormalRuns(old.directory)
				require.NoError(t, err)
				assert.Equal(t, old.runID, runs.Current)
				current := NewPublisher(ddsnmp.NewDeviceStore(), root)
				current.ReplaceNormal("", "device", 1).Update(&normalTestCut{attempt: 1})
				write, rename := current.writeFile, current.rename
				if tc.failWrite {
					current.writeFile = func(context.Context, string, Document, func(string, string) error) error {
						return errors.New("write failure")
					}
				}
				if tc.failActivation {
					current.rename = func(from, to string) error {
						if filepath.Base(to) == normalRunsFilename {
							return errors.New("index failure")
						}
						return os.Rename(from, to)
					}
				}
				err = current.publishNormal(t.Context())
				if tc.failWrite || tc.failActivation {
					require.Error(t, err)
					runs, err = ReadNormalRuns(old.directory)
					require.NoError(t, err)
					assert.Equal(t, old.runID, runs.Current)
					current.writeFile, current.rename = write, rename
					time.Sleep(publicationRetryEvery)
					require.NoError(t, current.publishNormal(t.Context()))
				} else {
					require.NoError(t, err)
				}
				runs, err = ReadNormalRuns(old.directory)
				require.NoError(t, err)
				assert.Equal(t, NormalRuns{Current: current.runID, Previous: old.runID}, runs)
				next := NewPublisher(ddsnmp.NewDeviceStore(), root)
				if tc.failCleanup {
					next.remove = func(path string) error {
						if filepath.Dir(path) == filepath.Join(next.directory, NormalDirectory, old.runID) {
							return errors.New("cleanup failure")
						}
						return os.Remove(path)
					}
				}
				next.ReplaceNormal("", "device", 1).Update(&normalTestCut{attempt: 1})
				err = next.publishNormal(t.Context())
				if tc.failCleanup {
					require.Error(t, err)
					next.remove = os.Remove
					require.NoError(t, next.publishNormal(t.Context()))
				} else {
					require.NoError(t, err)
				}
				runs, err = ReadNormalRuns(old.directory)
				require.NoError(t, err)
				assert.Equal(t, NormalRuns{Current: next.runID, Previous: current.runID}, runs)
				require.NoDirExists(t, filepath.Join(old.directory, NormalDirectory, old.runID))
				files, err := ListNormalFiles(old.directory)
				require.NoError(t, err)
				require.Len(t, files, 2)
			})
		})
	}
}

func TestNormalRegistrationRetirementRetries(t *testing.T) {
	for name, tc := range map[string]struct{ replacement bool }{"removed": {}, "replaced": {true}} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := NewPublisher(ddsnmp.NewDeviceStore(), t.TempDir())
				writer := p.ReplaceNormal("", "device", 1)
				writer.Update(&normalTestCut{attempt: 1})
				require.NoError(t, p.publishNormal(t.Context()))
				if tc.replacement {
					p.ReplaceNormal("device", "device", 1)
				} else {
					p.RemoveNormal("device")
				}
				writer.Update(&normalTestCut{attempt: 2})
				p.remove = func(string) error { return errors.New("retirement failure") }
				require.Error(t, p.publishNormal(t.Context()))
				p.remove = os.Remove
				require.NoError(t, p.publishNormal(t.Context()))
				files, err := ListNormalFiles(p.directory)
				require.NoError(t, err)
				assert.Empty(t, files)
			})
		})
	}
}

type normalPublicationBenchmarkCut struct{ device *NormalDevice }

func (c normalPublicationBenchmarkCut) CaptureNormal() (*NormalDevice, error) { return c.device, nil }

// Measures complete per-device validation, serial encoder reuse, atomic file
// replacement and run-index handling, bypassing the wait between writes.
// Timings are workstation trends only.
func BenchmarkNormalPublication(b *testing.B) {
	for name, rows := range map[string]int{"small": 32, "many_rows": 4096} {
		b.Run(name, func(b *testing.B) {
			p := NewPublisher(ddsnmp.NewDeviceStore(), b.TempDir())
			now := time.Now().UTC()
			source := &ddsnmp.SourceOperation{ContextID: 1, Ordinal: 1, StartedAt: now, Method: "bulk_walk", RequestedOIDs: []string{"1.3.6.1.4.1.99999"}, ResultPresent: true}
			samples := make(map[string]int64, rows)
			for i := range rows {
				oid := fmt.Sprintf("1.3.6.1.4.1.99999.1.%d", i+1)
				source.PDUs = append(source.PDUs, ddsnmp.SourcePDU{OID: oid, Type: 65, Value: ddsnmp.SourceValue{Kind: "unsigned", Text: "7"}})
				samples[oid] = 7
			}
			cut := normalPublicationBenchmarkCut{&NormalDevice{Hostname: "benchmark", CapturedAt: now, Sources: []*ddsnmp.SourceOperation{source},
				Latest: &NormalAttempt{ID: 1, Phase: "collect", StartedAt: now, CompletedAt: now, Sources: []SourceRef{{1, 1}}, Samples: samples},
			}}
			writer := p.ReplaceNormal("", "benchmark", 1)
			writer.Update(cut)
			require.NoError(b, p.publishNormal(b.Context()))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				writer.Update(cut)
				if err := p.publishNormalPending(b.Context(), true); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			info, err := os.Stat(filepath.Join(p.directory, NormalDirectory, p.runID, writer.filename()))
			require.NoError(b, err)
			b.ReportMetric(float64(info.Size()), "compressed_bytes")
		})
	}
}
