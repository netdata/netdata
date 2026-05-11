package main

import (
	"reflect"
	"testing"
)

type structFieldExpectation struct {
	name string
	kind reflect.Kind
	typ  string
	len  int
}

func assertStructLayout(t *testing.T, typ reflect.Type, want []structFieldExpectation) {
	t.Helper()

	if typ.Kind() != reflect.Struct {
		t.Fatalf("type %s is %s, want struct", typ, typ.Kind())
	}

	if got := typ.NumField(); got != len(want) {
		t.Fatalf("field count = %d, want %d", got, len(want))
	}

	for i, exp := range want {
		field := typ.Field(i)
		if field.Name != exp.name {
			t.Fatalf("field %d name = %q, want %q", i, field.Name, exp.name)
		}
		if field.Type.Kind() != exp.kind {
			t.Fatalf("field %s kind = %s, want %s", field.Name, field.Type.Kind(), exp.kind)
		}
		if exp.typ != "" && field.Type.String() != exp.typ {
			t.Fatalf("field %s type = %s, want %s", field.Name, field.Type.String(), exp.typ)
		}
		if exp.len >= 0 && field.Type.Kind() == reflect.Array && field.Type.Len() != exp.len {
			t.Fatalf("field %s len = %d, want %d", field.Name, field.Type.Len(), exp.len)
		}
	}
}

func TestEBPFPidStatSchemaLayouts(t *testing.T) {
	tests := map[string]struct {
		typ  reflect.Type
		want []structFieldExpectation
	}{
		"pid-stat": {
			typ: reflect.TypeOf(ebpfPidStat{}),
			want: []structFieldExpectation{
				{name: "pid", kind: reflect.Uint32, typ: "uint32"},
				{name: "comm", kind: reflect.Array, typ: "[96]uint8", len: EBPF_MAX_COMPARE_NAME + 1},
				{name: "ppid", kind: reflect.Uint32, typ: "uint32"},
				{name: "cachestat", kind: reflect.Struct, typ: "main.netdataPublishCachestat"},
				{name: "dc", kind: reflect.Struct, typ: "main.netdataPublishDCStat"},
				{name: "fd", kind: reflect.Struct, typ: "main.netdataPublishFDStat"},
				{name: "process", kind: reflect.Struct, typ: "main.ebpfProcessStat"},
				{name: "shm", kind: reflect.Struct, typ: "main.netdataPublishSHM"},
				{name: "swap", kind: reflect.Struct, typ: "main.netdataPublishSwap"},
				{name: "socket", kind: reflect.Struct, typ: "main.ebpfSocketPublishApps"},
				{name: "vfs", kind: reflect.Struct, typ: "main.netdataPublishVFS"},
			},
		},
		"cachestat": {
			typ: reflect.TypeOf(netdataPublishCachestat{}),
			want: []structFieldExpectation{
				{name: "Ct", kind: reflect.Uint64, typ: "uint64"},
				{name: "Ratio", kind: reflect.Int64, typ: "int64"},
				{name: "Dirty", kind: reflect.Int64, typ: "int64"},
				{name: "Hit", kind: reflect.Int64, typ: "int64"},
				{name: "Miss", kind: reflect.Int64, typ: "int64"},
				{name: "Current", kind: reflect.Struct, typ: "main.netdataCachestat"},
				{name: "Prev", kind: reflect.Struct, typ: "main.netdataCachestat"},
			},
		},
		"dcstat": {
			typ: reflect.TypeOf(netdataPublishDCStat{}),
			want: []structFieldExpectation{
				{name: "Ct", kind: reflect.Uint64, typ: "uint64"},
				{name: "Ratio", kind: reflect.Int64, typ: "int64"},
				{name: "CacheAccess", kind: reflect.Int64, typ: "int64"},
				{name: "Curr", kind: reflect.Struct, typ: "main.netdataPublishDCStatPid"},
				{name: "Prev", kind: reflect.Struct, typ: "main.netdataPublishDCStatPid"},
			},
		},
		"fdstat": {
			typ: reflect.TypeOf(netdataPublishFDStat{}),
			want: []structFieldExpectation{
				{name: "Ct", kind: reflect.Uint64, typ: "uint64"},
				{name: "OpenCall", kind: reflect.Uint32, typ: "uint32"},
				{name: "CloseCall", kind: reflect.Uint32, typ: "uint32"},
				{name: "OpenErr", kind: reflect.Uint32, typ: "uint32"},
				{name: "CloseErr", kind: reflect.Uint32, typ: "uint32"},
			},
		},
		"process": {
			typ: reflect.TypeOf(ebpfProcessStat{}),
			want: []structFieldExpectation{
				{name: "Ct", kind: reflect.Uint64, typ: "uint64"},
				{name: "Uid", kind: reflect.Uint32, typ: "uint32"},
				{name: "Gid", kind: reflect.Uint32, typ: "uint32"},
				{name: "Name", kind: reflect.Array, typ: "[16]uint8", len: TASK_COMM_LEN},
				{name: "Tgid", kind: reflect.Uint32, typ: "uint32"},
				{name: "Pid", kind: reflect.Uint32, typ: "uint32"},
				{name: "ExitCall", kind: reflect.Uint32, typ: "uint32"},
				{name: "ReleaseCall", kind: reflect.Uint32, typ: "uint32"},
				{name: "CreateProcess", kind: reflect.Uint32, typ: "uint32"},
				{name: "CreateThread", kind: reflect.Uint32, typ: "uint32"},
				{name: "TaskErr", kind: reflect.Uint32, typ: "uint32"},
			},
		},
		"shm": {
			typ: reflect.TypeOf(netdataPublishSHM{}),
			want: []structFieldExpectation{
				{name: "Ct", kind: reflect.Uint64, typ: "uint64"},
				{name: "Get", kind: reflect.Uint32, typ: "uint32"},
				{name: "At", kind: reflect.Uint32, typ: "uint32"},
				{name: "Dt", kind: reflect.Uint32, typ: "uint32"},
				{name: "Ctl", kind: reflect.Uint32, typ: "uint32"},
			},
		},
		"swap": {
			typ: reflect.TypeOf(netdataPublishSwap{}),
			want: []structFieldExpectation{
				{name: "Ct", kind: reflect.Uint64, typ: "uint64"},
				{name: "Read", kind: reflect.Uint32, typ: "uint32"},
				{name: "Write", kind: reflect.Uint32, typ: "uint32"},
			},
		},
		"socket": {
			typ: reflect.TypeOf(ebpfSocketPublishApps{}),
			want: []structFieldExpectation{
				{name: "BytesSent", kind: reflect.Uint64, typ: "uint64"},
				{name: "BytesReceived", kind: reflect.Uint64, typ: "uint64"},
				{name: "CallTCPSent", kind: reflect.Uint64, typ: "uint64"},
				{name: "CallTCPReceived", kind: reflect.Uint64, typ: "uint64"},
				{name: "Retransmit", kind: reflect.Uint64, typ: "uint64"},
				{name: "CallUDPSent", kind: reflect.Uint64, typ: "uint64"},
				{name: "CallUDPReceived", kind: reflect.Uint64, typ: "uint64"},
				{name: "CallClose", kind: reflect.Uint64, typ: "uint64"},
				{name: "CallTCPV4Connection", kind: reflect.Uint64, typ: "uint64"},
				{name: "CallTCPV6Connection", kind: reflect.Uint64, typ: "uint64"},
			},
		},
		"vfs": {
			typ: reflect.TypeOf(netdataPublishVFS{}),
			want: []structFieldExpectation{
				{name: "Ct", kind: reflect.Uint64, typ: "uint64"},
				{name: "WriteCall", kind: reflect.Uint32, typ: "uint32"},
				{name: "WritevCall", kind: reflect.Uint32, typ: "uint32"},
				{name: "ReadCall", kind: reflect.Uint32, typ: "uint32"},
				{name: "ReadvCall", kind: reflect.Uint32, typ: "uint32"},
				{name: "UnlinkCall", kind: reflect.Uint32, typ: "uint32"},
				{name: "FsyncCall", kind: reflect.Uint32, typ: "uint32"},
				{name: "OpenCall", kind: reflect.Uint32, typ: "uint32"},
				{name: "CreateCall", kind: reflect.Uint32, typ: "uint32"},
				{name: "WriteBytes", kind: reflect.Uint64, typ: "uint64"},
				{name: "WritevBytes", kind: reflect.Uint64, typ: "uint64"},
				{name: "ReadvBytes", kind: reflect.Uint64, typ: "uint64"},
				{name: "ReadBytes", kind: reflect.Uint64, typ: "uint64"},
				{name: "WriteErr", kind: reflect.Uint32, typ: "uint32"},
				{name: "WritevErr", kind: reflect.Uint32, typ: "uint32"},
				{name: "ReadErr", kind: reflect.Uint32, typ: "uint32"},
				{name: "ReadvErr", kind: reflect.Uint32, typ: "uint32"},
				{name: "UnlinkErr", kind: reflect.Uint32, typ: "uint32"},
				{name: "FsyncErr", kind: reflect.Uint32, typ: "uint32"},
				{name: "OpenErr", kind: reflect.Uint32, typ: "uint32"},
				{name: "CreateErr", kind: reflect.Uint32, typ: "uint32"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assertStructLayout(t, tc.typ, tc.want)
		})
	}
}
