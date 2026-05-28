//go:build windows

package windows

import (
	"errors"
	"strings"
	"syscall"
	"testing"
)

func TestApplyDefault(t *testing.T) {
	if got := applyDefault(0, 42); got != 42 {
		t.Fatalf("applyDefault(0, 42) = %d, want 42", got)
	}
	if got := applyDefault(7, 42); got != 7 {
		t.Fatalf("applyDefault(7, 42) = %d, want 7", got)
	}
}

func TestMinU32(t *testing.T) {
	if got := minU32(1, 9); got != 1 {
		t.Fatalf("minU32(1, 9) = %d, want 1", got)
	}
	if got := minU32(9, 1); got != 1 {
		t.Fatalf("minU32(9, 1) = %d, want 1", got)
	}
	if got := minU32(5, 5); got != 5 {
		t.Fatalf("minU32(5, 5) = %d, want 5", got)
	}
}

func TestHighestBit(t *testing.T) {
	cases := []struct {
		mask uint32
		want uint32
	}{
		{0, 0},
		{1, 1},
		{2, 2},
		{3, 2},
		{0x10, 0x10},
		{0x101, 0x100},
		{0x80000001, 0x80000000},
	}

	for _, tc := range cases {
		if got := highestBit(tc.mask); got != tc.want {
			t.Fatalf("highestBit(0x%x) = 0x%x, want 0x%x", tc.mask, got, tc.want)
		}
	}
}

func TestIsDisconnectError(t *testing.T) {
	for _, errno := range []syscall.Errno{
		_ERROR_BROKEN_PIPE,
		_ERROR_NO_DATA,
		_ERROR_PIPE_NOT_CONNECTED,
	} {
		if !isDisconnectError(errno) {
			t.Fatalf("isDisconnectError(%v) = false, want true", errno)
		}
	}

	for _, err := range []error{
		nil,
		errors.New("plain"),
		syscall.Errno(_ERROR_ACCESS_DENIED),
	} {
		if isDisconnectError(err) {
			t.Fatalf("isDisconnectError(%v) = true, want false", err)
		}
	}
}

func TestFNV1a64Empty(t *testing.T) {
	got := FNV1a64([]byte{})
	if got != fnv1aOffsetBasis {
		t.Errorf("FNV1a64(empty) = %x, want %x", got, fnv1aOffsetBasis)
	}
}

func TestFNV1a64Deterministic(t *testing.T) {
	data := []byte("/var/run/netdata")
	h1 := FNV1a64(data)
	h2 := FNV1a64(data)
	if h1 != h2 {
		t.Errorf("FNV1a64 not deterministic: %x != %x", h1, h2)
	}
}

func TestFNV1a64DifferentInputs(t *testing.T) {
	h1 := FNV1a64([]byte("/var/run/netdata"))
	h2 := FNV1a64([]byte("/tmp/netdata"))
	if h1 == h2 {
		t.Error("FNV1a64 should produce different hashes for different inputs")
	}
}

func TestValidateServiceName(t *testing.T) {
	valid := []string{
		"cgroups-snapshot",
		"test_service.v1",
		"A-Z_09",
		"a",
		"abc123",
	}
	for _, name := range valid {
		if err := validateServiceName(name); err != nil {
			t.Errorf("validateServiceName(%q) = %v, want nil", name, err)
		}
	}

	invalid := []string{
		"",
		".",
		"..",
		"has space",
		"has/slash",
		"has\\backslash",
		"has@at",
		"has:colon",
	}
	for _, name := range invalid {
		if err := validateServiceName(name); err == nil {
			t.Errorf("validateServiceName(%q) = nil, want error", name)
		}
	}
}

func TestBuildPipeName(t *testing.T) {
	name, err := BuildPipeName("/var/run/netdata", "cgroups-snapshot")
	if err != nil {
		t.Fatalf("BuildPipeName failed: %v", err)
	}

	// Should end with NUL
	if name[len(name)-1] != 0 {
		t.Error("pipe name should end with NUL")
	}

	// Convert to narrow string for checking
	narrow := make([]byte, len(name)-1)
	for i, c := range name[:len(name)-1] {
		narrow[i] = byte(c)
	}
	s := string(narrow)

	if s[:len(`\\.\pipe\netipc-`)] != `\\.\pipe\netipc-` {
		t.Errorf("unexpected prefix: %s", s)
	}

	// Should end with service name
	suffix := "-cgroups-snapshot"
	if s[len(s)-len(suffix):] != suffix {
		t.Errorf("unexpected suffix in: %s", s)
	}
}

func TestBuildPipeNameDeterministic(t *testing.T) {
	n1, _ := BuildPipeName("/var/run", "svc")
	n2, _ := BuildPipeName("/var/run", "svc")
	if len(n1) != len(n2) {
		t.Fatal("different lengths")
	}
	for i := range n1 {
		if n1[i] != n2[i] {
			t.Fatalf("mismatch at %d", i)
		}
	}
}

func TestBuildPipeNameDifferentRunDir(t *testing.T) {
	n1, _ := BuildPipeName("/var/run/netdata", "svc")
	n2, _ := BuildPipeName("/tmp/netdata", "svc")
	// They should differ (different hash)
	if len(n1) == len(n2) {
		same := true
		for i := range n1 {
			if n1[i] != n2[i] {
				same = false
				break
			}
		}
		if same {
			t.Error("different run_dir should produce different pipe names")
		}
	}
}

func TestBuildPipeNameInvalidService(t *testing.T) {
	_, err := BuildPipeName("/var/run", "")
	if err == nil {
		t.Error("expected error for empty service name")
	}

	_, err = BuildPipeName("/var/run", "bad/name")
	if err == nil {
		t.Error("expected error for service with /")
	}

	_, err = BuildPipeName("/var/run", ".")
	if err == nil {
		t.Error("expected error for '.'")
	}
}

func TestBuildPipeNameTooLong(t *testing.T) {
	service := strings.Repeat("a", maxPipeNameChars)
	if _, err := BuildPipeName("/var/run", service); !errors.Is(err, ErrPipeName) {
		t.Fatalf("BuildPipeName(long) = %v, want ErrPipeName", err)
	}
}

func TestSetNamedPipeHandleStateInvalidHandle(t *testing.T) {
	mode := uint32(_PIPE_READMODE_MESSAGE)
	if err := setNamedPipeHandleState(syscall.InvalidHandle, &mode); err == nil {
		t.Fatal("setNamedPipeHandleState on invalid handle should fail")
	}
}
