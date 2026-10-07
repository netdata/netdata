// SPDX-License-Identifier: GPL-3.0-or-later

package ipmiapi

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bougou/go-ipmi/pkg/client"
	"github.com/bougou/go-ipmi/pkg/command/app"
	"github.com/bougou/go-ipmi/pkg/command/sensor"
	"github.com/bougou/go-ipmi/pkg/command/storage"
	"github.com/bougou/go-ipmi/pkg/types"
)

type fakeTransport struct {
	records             [][]byte
	readings            map[uint8][]byte
	failures            map[uint8]error
	repoError, selError error
	before              func(types.Request) error
	calls               []types.Request
	connects, closes    int
	date                uint32
	partial             bool
}

func (f *fakeTransport) Connect(context.Context) error { f.connects++; return nil }
func (f *fakeTransport) Close(context.Context) error   { f.closes++; return nil }
func (f *fakeTransport) Exchange(ctx context.Context, req types.Request, res types.Response) error {
	f.calls = append(f.calls, req)
	if f.before != nil {
		if err := f.before(req); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var data []byte
	switch q := req.(type) {
	case *app.GetDeviceIDRequest:
		data = make([]byte, 15)
	case *storage.GetSDRRepoInfoRequest:
		if f.repoError != nil {
			return f.repoError
		}
		data = make([]byte, 14)
		data[0] = 0x51
		binary.LittleEndian.PutUint16(data[1:], uint16(len(f.records)))
		binary.LittleEndian.PutUint32(data[5:], f.date)
		data[13] = 2
	case *storage.ReserveSDRRepoRequest:
		data = []byte{1, 0}
	case *storage.GetSDRRequest:
		index := int(q.RecordID)
		if index == 0 {
			index = 1
		}
		index--
		if index >= len(f.records) {
			return fmt.Errorf("bad fake record %d", index)
		}
		if f.partial && q.ReadBytes == 255 {
			return types.NewResponseError(types.CodeCannotReturnRequestedDataBytes, "partial required")
		}
		next := uint16(index + 2)
		if index == len(f.records)-1 {
			next = 0xffff
		}
		record := f.records[index]
		if q.ReadBytes != 255 {
			record = record[int(q.ReadOffset) : int(q.ReadOffset)+int(q.ReadBytes)]
		}
		data = make([]byte, 2+len(record))
		binary.LittleEndian.PutUint16(data, next)
		copy(data[2:], record)
	case *sensor.GetSensorReadingRequest:
		if client.GetCommandContext(ctx) == nil {
			return errors.New("missing owner addressing context")
		}
		if err := f.failures[q.SensorNumber]; err != nil {
			return err
		}
		data = f.readings[q.SensorNumber]
	case *sensor.GetSensorReadingFactorsRequest:
		// Factors depend on the exact reading; M=2, Rexp=-1.
		if q.Reading != 25 {
			return fmt.Errorf("wrong factor input %d", q.Reading)
		}
		data = []byte{0, 2, 0, 0, 0, 0, 0xf0}
	case *storage.GetSELInfoRequest:
		if f.selError != nil {
			return f.selError
		}
		data = make([]byte, 14)
		binary.LittleEndian.PutUint16(data[1:], 659)
	default:
		return fmt.Errorf("unexpected command %T", req)
	}
	// Decode real bytes rather than constructing the adapter's expected state.
	if err := res.Unpack(data); err != nil {
		return types.NewResponseError(0, err.Error())
	}
	return nil
}
func newFakeReader(t *testing.T, f *fakeTransport) *Reader {
	t.Helper()
	r, err := New(Config{Driver: "open", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	r.factory = func() (transport, error) { return f, nil }
	return r
}
func fullRecord(id uint16, number uint8, name string) []byte {
	data := make([]byte, 48+len(name))
	binary.LittleEndian.PutUint16(data, id)
	data[2] = 0x51
	data[3] = 1
	data[4] = byte(len(data) - 5)
	data[5] = 0x20
	data[7] = number
	data[12] = 1
	data[13] = 1
	data[20] = 0
	data[21] = 1
	data[24] = 1
	data[47] = 0xc0 | byte(len(name))
	copy(data[48:], name)
	return data
}
func compactRecord(id uint16, number uint8, kind uint8, name string) []byte {
	data := make([]byte, 32+len(name))
	binary.LittleEndian.PutUint16(data, id)
	data[2] = 0x51
	data[3] = 2
	data[4] = byte(len(data) - 5)
	data[5] = 0x20
	data[7] = number
	data[12] = kind
	data[13] = 0x6f
	data[20] = 0xc0
	data[31] = 0xc0 | byte(len(name))
	copy(data[32:], name)
	return data
}
func baseFake() *fakeTransport {
	return &fakeTransport{records: [][]byte{fullRecord(1, 10, "CPU Temp"), compactRecord(2, 11, 7, "CPU Presence")}, readings: map[uint8][]byte{10: {25, 0xc0, 0}, 11: {0, 0xc0, 0x80, 0}}, failures: map[uint8]error{}, date: 0xffffffff}
}
func TestReaderCheckAndCollect(t *testing.T) {
	f := baseFake()
	r := newFakeReader(t, f)
	if err := r.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("Check issued %d commands", len(f.calls))
	}
	got, err := r.Collect(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sensors) != 2 || got.SEL == nil || *got.SEL != 659 || len(got.Warnings) != 0 {
		t.Fatalf("snapshot: %+v", got)
	}
	s := got.Sensors[0]
	if s.Key != "i1_n10_t2_u1_CPU Temp" || s.State != "nominal" || s.Value == nil || *s.Value != 25 || s.Metric != "temperature_c" {
		t.Fatalf("sensor: %+v", s)
	}
	if got.Sensors[1].Key != "i2_n11_t255_u0_CPU Presence" || got.Sensors[1].State != "nominal" || got.Sensors[1].Value != nil {
		t.Fatalf("discrete: %+v", got.Sensors[1])
	}
	if err := r.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.closes != 1 {
		t.Fatalf("close count %d", f.closes)
	}
}
func TestReaderUnavailableAndRecovery(t *testing.T) {
	f := baseFake()
	r := newFakeReader(t, f)
	first, err := r.Collect(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, flags := range []byte{0xe0, 0x80} {
		f.readings[10] = []byte{99, flags, 0x10}
		got, err := r.Collect(context.Background(), false)
		if err != nil {
			t.Fatal(err)
		}
		if got.Sensors[0].Key != first.Sensors[0].Key || got.Sensors[0].Value != nil || got.Sensors[0].State != "unknown" || got.Sensors[1].State != "nominal" {
			t.Fatalf("unavailable: %+v", got)
		}
	}
	f.readings[10] = []byte{30, 0x40, 0x10} // Event messages disabled, scanning still enabled.
	got, err := r.Collect(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sensors[0].Value == nil || *got.Sensors[0].Value != 30 || got.Sensors[0].State != "critical" {
		t.Fatalf("recovery: %+v", got)
	}
	if *first.Sensors[0].Value != 25 {
		t.Fatal("previous snapshot mutated")
	}
}
func TestReaderPartialSensorFailureAndSELFailure(t *testing.T) {
	f := baseFake()
	r := newFakeReader(t, f)
	f.failures[10] = types.NewResponseError(0xcb, "not present")
	f.selError = errors.New("SEL transport failure")
	got, err := r.Collect(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sensors[0].Value != nil || got.Sensors[0].State != "unknown" || got.Sensors[1].State != "nominal" || got.SEL != nil || len(got.Warnings) != 2 {
		t.Fatalf("partial: %+v", got)
	}
	delete(f.failures, 10)
	f.selError = nil
	got, err = r.Collect(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sensors[0].Value == nil || got.SEL == nil || len(got.Warnings) != 0 {
		t.Fatalf("recovery: %+v", got)
	}
}
func TestReaderCoreFailureReconnect(t *testing.T) {
	for _, location := range []string{"inventory", "reading"} {
		t.Run(location, func(t *testing.T) {
			f := baseFake()
			r := newFakeReader(t, f)
			if location == "inventory" {
				f.repoError = errors.New("transport failed")
			} else {
				f.failures[10] = errors.New("transport failed")
			}
			if got, err := r.Collect(context.Background(), false); err == nil || got != nil {
				t.Fatalf("got %+v, %v", got, err)
			}
			if f.closes != 1 {
				t.Fatalf("connection not closed")
			}
			f.repoError = nil
			delete(f.failures, 10)
			got, err := r.Collect(context.Background(), false)
			if err != nil || got == nil {
				t.Fatalf("recovery: %+v %v", got, err)
			}
			if f.connects != 2 {
				t.Fatalf("connect count %d", f.connects)
			}
		})
	}
}
func TestReaderRediscovery(t *testing.T) {
	f := baseFake()
	r := newFakeReader(t, f)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }
	for n := 0; n < 2; n++ {
		if _, err := r.Collect(context.Background(), false); err != nil {
			t.Fatal(err)
		}
	}
	countSDR := func() int {
		n := 0
		for _, c := range f.calls {
			if _, ok := c.(*storage.GetSDRRequest); ok {
				n++
			}
		}
		return n
	}
	if countSDR() != 2 {
		t.Fatalf("cache missed: %d", countSDR())
	}
	f.records[0] = fullRecord(1, 10, "New Name")
	now = now.Add(inventoryRefresh)
	got, err := r.Collect(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sensors[0].Name != "New Name" || countSDR() != 4 {
		t.Fatalf("sentinel refresh failed: %+v", got)
	}
	f.date = 123
	f.records = f.records[:1]
	got, err = r.Collect(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sensors) != 1 || countSDR() != 5 {
		t.Fatalf("metadata refresh failed: %+v", got)
	}
}
func TestReaderCancellation(t *testing.T) {
	for _, stage := range []string{"before", "inventory", "reading", "sel"} {
		t.Run(stage, func(t *testing.T) {
			f := baseFake()
			r := newFakeReader(t, f)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if stage == "before" {
				cancel()
			} else {
				f.before = func(req types.Request) error {
					switch req.(type) {
					case *storage.GetSDRRequest:
						if stage == "inventory" {
							cancel()
						}
					case *sensor.GetSensorReadingRequest:
						if stage == "reading" {
							cancel()
						}
					case *storage.GetSELInfoRequest:
						if stage == "sel" {
							cancel()
						}
					}
					return nil
				}
			}
			got, err := r.Collect(ctx, true)
			if !errors.Is(err, context.Canceled) || got != nil {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}
func TestReaderOwnershipAndSharing(t *testing.T) {
	f := baseFake()
	shared := compactRecord(3, 20, 8, "PSU")
	shared[23] = 3
	shared[24] = 5
	satellite := fullRecord(4, 50, "Remote")
	satellite[5] = 0x22
	lun := fullRecord(5, 30, "BMC LUN")
	lun[6] = 1
	channel := fullRecord(6, 40, "Channel")
	channel[6] = 0x10
	f.records = [][]byte{shared, satellite, lun, channel}
	f.readings[20] = []byte{0, 0xc0, 1, 0}
	f.readings[21] = []byte{0, 0xc0, 2, 0}
	f.readings[22] = []byte{0, 0xc0, 128, 0}
	f.readings[30] = []byte{30, 0xc0, 0}
	got, err := newFakeReader(t, f).Collect(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sensors) != 6 || got.Sensors[0].Name != "PSU 5" || got.Sensors[1].State != "critical" || got.Sensors[2].State != "warning" || got.Sensors[3].Value != nil || got.Sensors[4].Value == nil || got.Sensors[5].Value != nil {
		t.Fatalf("ownership: %+v", got)
	}
	for _, req := range f.calls {
		if q, ok := req.(*sensor.GetSensorReadingRequest); ok && (q.SensorNumber == 50 || q.SensorNumber == 40) {
			t.Fatal("queried unsupported owner")
		}
	}
	if !reflect.DeepEqual(got.Warnings, []string{"2 sensor records have unsupported ownership, sharing, or ID encoding"}) {
		t.Fatalf("warnings: %v", got.Warnings)
	}
}
func TestReaderNonlinearFactorsAndEXP10(t *testing.T) {
	for _, tc := range []struct {
		name   string
		linear byte
		raw    byte
		want   float64
	}{{"nonlinear", 0x70, 25, 5}, {"exp10", 5, 15, 31.622776601683793}} {
		t.Run(tc.name, func(t *testing.T) {
			f := baseFake()
			f.records = f.records[:1]
			f.records[0][23] = tc.linear
			f.records[0][29] = 0xf0
			f.readings[10] = []byte{tc.raw, 0xc0, 0}
			got, err := newFakeReader(t, f).Collect(context.Background(), false)
			if err != nil {
				t.Fatal(err)
			}
			if got.Sensors[0].Value == nil || math.Abs(*got.Sensors[0].Value-tc.want) > 1e-12 {
				t.Fatalf("got %+v", got.Sensors[0])
			}
		})
	}
	f := baseFake()
	f.records = f.records[:1]
	f.records[0][23] = 0x70
	f.before = func(req types.Request) error {
		if _, ok := req.(*sensor.GetSensorReadingFactorsRequest); ok {
			return types.NewResponseError(0xc1, "unsupported")
		}
		return nil
	}
	got, err := newFakeReader(t, f).Collect(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sensors[0].Value != nil || got.Sensors[0].State != "nominal" || len(got.Warnings) != 1 {
		t.Fatalf("factor failure: %+v", got)
	}
}
func TestReaderPartialSDR(t *testing.T) {
	f := baseFake()
	f.partial = true
	got, err := newFakeReader(t, f).Collect(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sensors) != 2 || got.Sensors[0].Value == nil {
		t.Fatalf("got %+v", got)
	}
}
func TestNewValidation(t *testing.T) {
	for _, c := range []Config{{Driver: "other", Timeout: time.Second}, {Driver: "open", Device: -1, Timeout: time.Second}, {Driver: "open"}, {Driver: "lan", Timeout: time.Second}, {Driver: "lanplus", Timeout: time.Second, Hostname: "host", Port: 65536}} {
		if _, err := New(c); err == nil {
			t.Fatalf("accepted invalid config %+v", c)
		}
	}
	for _, driver := range []string{"open", "lan", "lanplus"} {
		if _, err := New(Config{Driver: driver, Hostname: "host", Port: 623, Timeout: time.Second}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReaderMissingAndMalformedReading(t *testing.T) {
	for _, data := range [][]byte{{25, 0xc0}, {25}} {
		f := baseFake()
		f.readings[10] = data
		got, err := newFakeReader(t, f).Collect(context.Background(), false)
		if err != nil {
			t.Fatal(err)
		}
		if got.Sensors[0].State != "unknown" || got.Sensors[1].State != "nominal" || len(got.Warnings) != 1 {
			t.Fatalf("got %+v", got)
		}
		if len(data) == 2 && (got.Sensors[0].Value == nil || *got.Sensors[0].Value != 25) {
			t.Fatal("valid numeric byte lost")
		}
		if len(data) == 1 && got.Sensors[0].Value != nil {
			t.Fatal("malformed response emitted value")
		}
	}
}
func TestReaderEmptyInventory(t *testing.T) {
	f := baseFake()
	f.records = nil
	got, err := newFakeReader(t, f).Collect(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sensors) != 0 || got.SEL == nil {
		t.Fatalf("got %+v", got)
	}
}
func TestReaderChangedDuringDiscovery(t *testing.T) {
	f := baseFake()
	f.before = func(req types.Request) error {
		if _, ok := req.(*storage.GetSDRRequest); ok {
			f.date++
		}
		return nil
	}
	if got, err := newFakeReader(t, f).Collect(context.Background(), false); err == nil || got != nil {
		t.Fatalf("published inconsistent inventory: %+v %v", got, err)
	}
}
func TestReaderMalformedAndCyclicInventory(t *testing.T) {
	for _, tc := range []string{"short", "cycle"} {
		t.Run(tc, func(t *testing.T) {
			f := baseFake()
			if tc == "short" {
				f.records[0] = f.records[0][:5]
			} else {
				f.before = func(req types.Request) error {
					if q, ok := req.(*storage.GetSDRRequest); ok {
						q.RecordID = 0
					}
					return nil
				}
			}
			if got, err := newFakeReader(t, f).Collect(context.Background(), false); err == nil || got != nil {
				t.Fatalf("got %+v %v", got, err)
			}
		})
	}
}
func TestPartialSDRMaximumWireSize(t *testing.T) {
	f := baseFake()
	f.partial = true
	f.records = [][]byte{make([]byte, 260)}
	f.records[0][2] = 0x51
	f.records[0][3] = 0xc0
	f.records[0][4] = 255
	got, err := newFakeReader(t, f).Collect(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sensors) != 0 {
		t.Fatal("OEM record treated as sensor")
	}
	var final *storage.GetSDRRequest
	for _, req := range f.calls {
		if q, ok := req.(*storage.GetSDRRequest); ok {
			final = q
		}
	}
	if final.ReadOffset != 255 || final.ReadBytes != 5 {
		t.Fatalf("last offset wrapped: %+v", final)
	}
}
func TestReaderCommandDeadlineAndSafeErrors(t *testing.T) {
	f := baseFake()
	r := newFakeReader(t, f)
	r.config.Username = "synthetic-user"
	r.config.Password = "synthetic-password"
	f.repoError = fmt.Errorf("failed synthetic-user synthetic-password")
	_, err := r.Collect(context.Background(), false)
	if err == nil || strings.Contains(err.Error(), "synthetic-") {
		t.Fatalf("unsanitized error %v", err)
	}
	f.repoError = nil
	f.before = func(req types.Request) error {
		if _, ok := req.(*storage.GetSDRRepoInfoRequest); ok {
			return context.DeadlineExceeded
		}
		return nil
	}
	_, err = r.Collect(context.Background(), false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline identity lost: %v", err)
	}
}

func TestRealClientFactory(t *testing.T) {
	for _, driver := range []string{"open", "lan", "lanplus"} {
		t.Run(driver, func(t *testing.T) {
			r, err := New(Config{Driver: driver, Hostname: "bmc.example", Port: 623, Username: "monitor", Password: "synthetic-secret", Timeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			transport, err := r.factory()
			if err != nil {
				t.Fatal(err)
			}
			c := transport.(*connection)
			if c.Interface != client.Interface(driver) || c.SessionPrivilegeLevel() != types.PrivilegeLevelUser {
				t.Fatalf("unexpected interface/privilege: %s/%s", c.Interface, c.SessionPrivilegeLevel())
			}
			if driver == "open" {
				// The pinned SDK has no public backend-state accessor. Read (never mutate)
				// this constructor invariant: ConnectOpen dereferences it after device open.
				// Fake transport tests cannot exercise successful physical device opening.
				backend := reflect.ValueOf(c.Client).Elem().FieldByName("openipmi")
				if !backend.IsValid() || backend.IsNil() {
					t.Fatal("local SDK client lacks OpenIPMI state required by ConnectOpen")
				}
				if c.Host != "" || c.Username != "" || c.Password != "" {
					t.Fatal("local SDK client retained unused LAN credentials")
				}
				if err := c.Close(context.Background()); err != nil {
					t.Fatalf("close unconnected local client: %v", err)
				}
			} else if c.Host != "bmc.example" || c.Port != 623 || c.Username != "monitor" {
				t.Fatal("LAN endpoint not configured")
			}
		})
	}
}
