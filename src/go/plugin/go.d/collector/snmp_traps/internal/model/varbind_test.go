// SPDX-License-Identifier: GPL-3.0-or-later

package model

import (
	"strconv"
	"testing"
)

func TestFindVarbindForProfileOID(t *testing.T) {
	const oid = "1.3.6.1.2.1.2.2.1.8"
	values := []VarbindValue{
		{OID: oid + ".2", Value: "second"},
		{OID: oid, Value: "exact"},
		{OID: oid + ".1", Value: "first"},
	}

	got, ok := FindVarbindForProfileOID(values, oid)
	if !ok || got.Value != "exact" {
		t.Fatalf("exact lookup = %#v/%v, want exact/true", got, ok)
	}

	got, ok = FindVarbindForProfileOID(values[:1], oid)
	if !ok || got.Value != "second" {
		t.Fatalf("column lookup = %#v/%v, want first observed/true", got, ok)
	}

	if _, ok := FindVarbindForProfileOID(values, ""); ok {
		t.Fatal("empty OID matched")
	}
	if _, ok := FindVarbindForProfileOID(nil, oid); ok {
		t.Fatal("nil varbinds matched")
	}
}

func TestFindVarbindByName(t *testing.T) {
	values := []VarbindValue{{Name: "ifIndex.1", Value: 1}, {Name: "ifIndex.2", Value: 2}}
	got, ok := FindVarbindByName(values, "ifIndex.2")
	if !ok || got.Value != 2 {
		t.Fatalf("lookup = %#v/%v, want second value/true", got, ok)
	}
	if _, ok := FindVarbindByName(values, ""); ok {
		t.Fatal("empty name matched")
	}
}

func TestIsSensitiveVarbind(t *testing.T) {
	baseOID := "1.3.6.1.6.3.18.1.4"
	for _, test := range []struct {
		name  string
		value VarbindValue
		want  bool
	}{
		{name: "scalar OID", value: VarbindValue{OID: SNMPTrapCommunityOID}, want: true},
		{name: "base OID", value: VarbindValue{OID: baseOID}, want: true},
		{name: "leading dot", value: VarbindValue{OID: "." + SNMPTrapCommunityOID}, want: true},
		{name: "instance name", value: VarbindValue{Name: "snmpTrapCommunity.0"}, want: true},
		{name: "base name", value: VarbindValue{Name: "snmpTrapCommunity"}, want: true},
		{name: "name wins over unrelated OID", value: VarbindValue{Name: "snmpTrapCommunity.0", OID: "1.3.6.1"}, want: true},
		{name: "neighboring OID", value: VarbindValue{OID: baseOID + "1"}},
		{name: "neighboring name", value: VarbindValue{Name: "snmpTrapCommunityValue"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := IsSensitiveVarbind(test.value); got != test.want {
				t.Fatalf("IsSensitiveVarbind(%#v) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

func TestVarbindRawValue(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
		want  string
	}{
		{name: "nil", value: nil, want: ""},
		{name: "string", value: "value", want: "value"},
		{name: "bytes", value: []byte("raw"), want: "raw"},
		{name: "signed", value: int64(-2), want: "-2"},
		{name: "unsigned", value: uint64(2), want: "2"},
		{name: "float", value: float64(1.25), want: "1.25"},
		{name: "true", value: true, want: "true"},
		{name: "false", value: false, want: "false"},
		{name: "fallback", value: []int{1, 2}, want: "[1 2]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := VarbindRawValue(VarbindValue{Value: test.value}); got != test.want {
				t.Fatalf("VarbindRawValue(%#v) = %q, want %q", test.value, got, test.want)
			}
		})
	}
	if RedactedVarbindValue != "<redacted>" {
		t.Fatalf("RedactedVarbindValue = %q", RedactedVarbindValue)
	}
}

func BenchmarkFindVarbindForProfileOID(b *testing.B) {
	const target = "1.3.6.1.2.1.2.2.1.8"
	for _, size := range []int{2, 50, 256} {
		values := make([]VarbindValue, size)
		for i := range values {
			values[i].OID = "1.3.6.1.4.1.999.1.1"
		}
		values[size-1].OID = target + ".1"
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			b.ReportAllocs()
			var got VarbindValue
			var ok bool
			for b.Loop() {
				got, ok = FindVarbindForProfileOID(values, target)
			}
			if !ok || got.OID != target+".1" {
				b.Fatalf("lookup = %#v/%v, want target instance/true", got, ok)
			}
		})
	}
}
