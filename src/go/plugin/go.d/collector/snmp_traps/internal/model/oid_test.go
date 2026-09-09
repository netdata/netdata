// SPDX-License-Identifier: GPL-3.0-or-later

package model

import "testing"

func TestNormalizeOID(t *testing.T) {
	for _, test := range []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "plain", in: "1.3.6", want: "1.3.6"},
		{name: "one leading dot", in: ".1.3.6", want: "1.3.6"},
		{name: "two leading dots", in: "..1.3.6", want: ".1.3.6"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := NormalizeOID(test.in); got != test.want {
				t.Fatalf("NormalizeOID(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestAlternateTrapOID(t *testing.T) {
	for _, test := range []struct {
		in   string
		want string
	}{
		{in: "1.3.6.1.4.1.9.9.41.2.0.1", want: "1.3.6.1.4.1.9.9.41.2.1"},
		{in: "1.3.6.1.4.1.9.9.41.2.1", want: "1.3.6.1.4.1.9.9.41.2.0.1"},
		{in: ""},
		{in: ".1.3.6"},
		{in: "1.3.6."},
		{in: "1..3.6"},
		{in: "1.3.x.6"},
		{in: "1.3.6"},
	} {
		want := test.want
		if want == "" {
			want = test.in
		}
		if got := AlternateTrapOID(test.in); got != want {
			t.Errorf("AlternateTrapOID(%q) = %q, want %q", test.in, got, want)
		}
	}
}

func TestIsNumericOID(t *testing.T) {
	for _, test := range []struct {
		oid  string
		want bool
	}{
		{oid: "1", want: true},
		{oid: "1.3.6.1", want: true},
		{oid: ""},
		{oid: ".1.3"},
		{oid: "1.3."},
		{oid: "1..3"},
		{oid: "1.a.3"},
	} {
		if got := IsNumericOID(test.oid); got != test.want {
			t.Errorf("IsNumericOID(%q) = %v, want %v", test.oid, got, test.want)
		}
	}
}

func TestOIDMatchesColumn(t *testing.T) {
	const column = "1.3.6.1.2.1.2.2.1.8"
	for _, test := range []struct {
		observed string
		want     bool
	}{
		{observed: column + ".1", want: true},
		{observed: column},
		{observed: column + "0.1"},
		{observed: ""},
	} {
		if got := OIDMatchesColumn(column, test.observed); got != test.want {
			t.Errorf("OIDMatchesColumn(%q, %q) = %v, want %v", column, test.observed, got, test.want)
		}
	}
}
