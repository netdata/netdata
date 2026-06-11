// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"testing"
)

func TestJournalFieldNeedsBinary_SafeText(t *testing.T) {
	safe := []string{
		"hello world",
		"simple message",
		"127.0.0.1",
		"GigabitEthernet0/1",
		"1.3.6.1.4.1.9.9.315.0.1",
		"{\"key\": \"value\"}",
	}
	for _, s := range safe {
		if journalFieldNeedsBinaryString(s) {
			t.Fatalf("expected safe text, got binary: %q", s)
		}
	}
}

func TestJournalFieldNeedsBinary_Newline(t *testing.T) {
	if !journalFieldNeedsBinaryString("hello\nworld") {
		t.Fatal("newline should be binary")
	}
}

func TestJournalFieldNeedsBinary_NUL(t *testing.T) {
	if !journalFieldNeedsBinary([]byte{0x48, 0x00, 0x49}) {
		t.Fatal("NUL byte should be binary")
	}
}

func TestJournalFieldNeedsBinary_ControlChars(t *testing.T) {
	if !journalFieldNeedsBinary([]byte{0x01, 0x02, 0x03}) {
		t.Fatal("control chars should be binary")
	}
}

func TestJournalFieldNeedsBinary_TabAndSpace(t *testing.T) {
	if journalFieldNeedsBinary([]byte{'\t', ' ', 'A'}) {
		t.Fatal("tab and space should not trigger binary")
	}
}

func TestJournalFieldNeedsBinary_InvalidUTF8(t *testing.T) {
	if !journalFieldNeedsBinary([]byte{0xFF, 0xFE, 0xFD}) {
		t.Fatal("invalid UTF-8 should be binary")
	}
}

func TestJournalFieldNeedsBinary_DEL(t *testing.T) {
	if !journalFieldNeedsBinary([]byte{0x7f}) {
		t.Fatal("DEL byte should be binary")
	}
}

func TestJournalFieldNeedsBinary_Empty(t *testing.T) {
	if journalFieldNeedsBinaryString("") {
		t.Fatal("empty should not be binary")
	}
}

func TestBinaryEncodedFieldCount(t *testing.T) {
	fields := []JournalField{
		{Name: "MESSAGE", Value: []byte("documented\nmulti-line message")},
		{Name: "SAFE", Value: []byte("hello")},
		{Name: "UNSAFE", Value: []byte("hello\nworld")},
		{Name: "SAFE2", Value: []byte("world")},
		{Name: "NUL", Value: []byte{0x48, 0x00}},
	}

	count := binaryEncodedFieldCount(fields)
	if count != 2 {
		t.Fatalf("expected 2 binary-encoded fields, got %d", count)
	}
}

func TestCWE117_InjectionPrevention(t *testing.T) {
	injection := "real_value\nFAKE_FIELD=spoofed\nANOTHER=more"
	if !journalFieldNeedsBinaryString(injection) {
		t.Fatal("injection string should be classified as binary to prevent CWE-117")
	}
}
