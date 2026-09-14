// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"testing"
)

func TestFieldNeedsBinary_SafeText(t *testing.T) {
	safe := []string{
		"hello world",
		"simple message",
		"127.0.0.1",
		"GigabitEthernet0/1",
		"1.3.6.1.4.1.9.9.315.0.1",
		"{\"key\": \"value\"}",
	}
	for _, s := range safe {
		if journalFieldNeedsBinary([]byte(s)) {
			t.Fatalf("expected safe text, got binary: %q", s)
		}
	}
}

func TestFieldNeedsBinary_Newline(t *testing.T) {
	if !journalFieldNeedsBinary([]byte("hello\nworld")) {
		t.Fatal("newline should be binary")
	}
}

func TestFieldNeedsBinary_NUL(t *testing.T) {
	if !journalFieldNeedsBinary([]byte{0x48, 0x00, 0x49}) {
		t.Fatal("NUL byte should be binary")
	}
}

func TestFieldNeedsBinary_ControlChars(t *testing.T) {
	if !journalFieldNeedsBinary([]byte{0x01, 0x02, 0x03}) {
		t.Fatal("control chars should be binary")
	}
}

func TestFieldNeedsBinary_TabAndSpace(t *testing.T) {
	if journalFieldNeedsBinary([]byte{'\t', ' ', 'A'}) {
		t.Fatal("tab and space should not trigger binary")
	}
}

func TestFieldNeedsBinary_InvalidUTF8(t *testing.T) {
	if !journalFieldNeedsBinary([]byte{0xFF, 0xFE, 0xFD}) {
		t.Fatal("invalid UTF-8 should be binary")
	}
}

func TestFieldNeedsBinary_DEL(t *testing.T) {
	if !journalFieldNeedsBinary([]byte{0x7f}) {
		t.Fatal("DEL byte should be binary")
	}
}

func TestFieldNeedsBinary_Empty(t *testing.T) {
	if journalFieldNeedsBinary(nil) {
		t.Fatal("empty should not be binary")
	}
}

func TestCWE117_InjectionPrevention(t *testing.T) {
	injection := "real_value\nFAKE_FIELD=spoofed\nANOTHER=more"
	if !journalFieldNeedsBinary([]byte(injection)) {
		t.Fatal("injection string should be classified as binary to prevent CWE-117")
	}
}
