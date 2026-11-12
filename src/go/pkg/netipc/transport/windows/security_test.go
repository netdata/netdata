//go:build windows

package windows

import (
	"errors"
	"runtime"
	"syscall"
	"testing"
	"unsafe"
)

var (
	procConvertSDToStringSD        = modadvapi32.NewProc("ConvertSecurityDescriptorToStringSecurityDescriptorW")
	procImpersonateNamedPipeClient = modadvapi32.NewProc("ImpersonateNamedPipeClient")
	procRevertToSelf               = modadvapi32.NewProc("RevertToSelf")
	procOpenThreadToken            = modadvapi32.NewProc("OpenThreadToken")
	procGetCurrentThread           = modkernel32.NewProc("GetCurrentThread")
)

// securityIdentificationLevel is SecurityIdentification in
// SECURITY_IMPERSONATION_LEVEL.
const securityIdentificationLevel = 1

// objectDACLSDDL returns the object's DACL in SDDL form.
func objectDACLSDDL(t *testing.T, handle syscall.Handle) string {
	t.Helper()
	var sd uintptr
	r, _, _ := procGetSecurityInfo.Call(uintptr(handle), _SE_KERNEL_OBJECT,
		_DACL_SECURITY_INFORMATION, 0, 0, 0, 0, uintptr(unsafe.Pointer(&sd)))
	if r != 0 {
		t.Fatalf("GetSecurityInfo: %v", syscall.Errno(r))
	}
	defer syscall.LocalFree(syscall.Handle(sd))
	return descriptorDACLSDDL(t, sd)
}

func descriptorDACLSDDL(t *testing.T, sd uintptr) string {
	t.Helper()
	var str *uint16
	r, _, err := procConvertSDToStringSD.Call(sd, _SDDL_REVISION_1,
		_DACL_SECURITY_INFORMATION, uintptr(unsafe.Pointer(&str)), 0)
	if r == 0 {
		t.Fatalf("ConvertSecurityDescriptorToStringSecurityDescriptor: %v", err)
	}
	defer syscall.LocalFree(syscall.Handle(unsafe.Pointer(str)))
	return syscall.UTF16ToString(unsafe.Slice(str, 512))
}

// ownerOnlyDACL returns the expected DACL as Windows prints it. The round
// trip applies Windows' own SID aliases, such as LA for the built-in
// Administrator account that CI runners use.
func ownerOnlyDACL(t *testing.T, mask string) string {
	t.Helper()
	me, err := processUserSID()
	if err != nil {
		t.Fatalf("processUserSID: %v", err)
	}
	sddl, err := syscall.UTF16PtrFromString(
		"D:P(A;;" + mask + ";;;SY)(A;;" + mask + ";;;BA)(A;;" + mask + ";;;" + me + ")")
	if err != nil {
		t.Fatalf("UTF16PtrFromString: %v", err)
	}
	var sd uintptr
	r, _, callErr := procConvertStringSDToSD.Call(
		uintptr(unsafe.Pointer(sddl)), _SDDL_REVISION_1, uintptr(unsafe.Pointer(&sd)), 0)
	if r == 0 {
		t.Fatalf("ConvertStringSecurityDescriptorToSecurityDescriptor: %v", callErr)
	}
	defer syscall.LocalFree(syscall.Handle(sd))
	return descriptorDACLSDDL(t, sd)
}

// clientImpersonationLevel returns the impersonation level a pipe client
// granted this server. Impersonation is per thread, so the goroutine stays on
// its thread meanwhile.
func clientImpersonationLevel(pipe syscall.Handle) (uint32, bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if r, _, _ := procImpersonateNamedPipeClient.Call(uintptr(pipe)); r == 0 {
		return 0, false
	}
	defer procRevertToSelf.Call()
	thread, _, _ := procGetCurrentThread.Call()
	var token syscall.Token
	if r, _, _ := procOpenThreadToken.Call(thread, syscall.TOKEN_QUERY, 1,
		uintptr(unsafe.Pointer(&token))); r == 0 {
		return 0, false
	}
	defer token.Close()
	var level, n uint32
	if err := syscall.GetTokenInformation(token, syscall.TokenImpersonationLevel,
		(*byte)(unsafe.Pointer(&level)), 4, &n); err != nil {
		return 0, false
	}
	return level, true
}

func TestPipeDACLOwnerOnly(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()

	want := ownerOnlyDACL(t, "FA")
	if got := objectDACLSDDL(t, listener.handle); got != want {
		t.Fatalf("first instance DACL = %q, want %q", got, want)
	}

	// Instances created after an accept carry the same DACL.
	pipeName, err := BuildPipeName(testPipeRunDir, service)
	if err != nil {
		t.Fatalf("BuildPipeName: %v", err)
	}
	client, err := createClientPipe(&pipeName[0])
	if err != nil {
		t.Fatalf("client open: %v", err)
	}
	defer syscall.CloseHandle(client)
	pending, err := listener.AcceptPending()
	if err != nil {
		t.Fatalf("AcceptPending: %v", err)
	}
	defer pending.Close()
	if got := objectDACLSDDL(t, listener.handle); got != want {
		t.Fatalf("next instance DACL = %q, want %q", got, want)
	}
}

func TestPipeRejectsRemoteClients(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()

	// The same pipe through the SMB loopback is a remote client.
	pipeName, err := BuildPipeName(testPipeRunDir, service)
	if err != nil {
		t.Fatalf("BuildPipeName: %v", err)
	}
	remote, err := syscall.UTF16PtrFromString(`\\127.0.0.1\` + syscall.UTF16ToString(pipeName[4:]))
	if err != nil {
		t.Fatalf("remote name: %v", err)
	}
	h, err := syscall.CreateFile(remote, _GENERIC_READ|_GENERIC_WRITE, 0, nil, _OPEN_EXISTING, 0, 0)
	if err == nil {
		syscall.CloseHandle(h)
		t.Fatal("remote open succeeded, want refused")
	}
}

func TestClientRejectsUntrustedServer(t *testing.T) {
	service := uniquePipeService(t)
	listener := startListener(t, testPipeRunDir, service, defaultServerConfig())

	// BUILTIN\Users never owns the test pipe. Nobody accepts, so a client that
	// skipped the check fails on the deadline instead of hanging.
	trustedOwnerForTests = "S-1-5-32-545"
	_, err := ConnectTimeout(testPipeRunDir, service, defaultClientConfigPtr(), 2000, nil)
	trustedOwnerForTests = ""
	if !errors.Is(err, ErrUntrustedServer) {
		t.Fatalf("connect to untrusted owner = %v, want ErrUntrustedServer", err)
	}
	listener.Close()

	// Trusting the real owner lets the client through, and the server sees
	// only an identification-level token for it.
	service = uniquePipeService(t)
	listener = startListener(t, testPipeRunDir, service, defaultServerConfig())
	defer listener.Close()
	owner, err := objectOwnerSID(listener.handle)
	if err != nil {
		t.Fatalf("pipe owner: %v", err)
	}
	type result struct {
		level uint32
		ok    bool
	}
	levels := make(chan result, 1)
	go func() {
		session, err := listener.Accept()
		if err != nil {
			levels <- result{}
			return
		}
		defer session.Close()
		level, ok := clientImpersonationLevel(session.handle)
		levels <- result{level, ok}
	}()

	trustedOwnerForTests = owner
	client, err := ConnectTimeout(testPipeRunDir, service, defaultClientConfigPtr(), 10000, nil)
	trustedOwnerForTests = ""
	if err != nil {
		listener.Close()
		t.Fatalf("connect to trusted owner: %v", err)
	}
	defer client.Close()
	got := <-levels
	if !got.ok || got.level != securityIdentificationLevel {
		t.Fatalf("client impersonation level = %d (ok=%v), want SecurityIdentification", got.level, got.ok)
	}
}

func TestWinShmObjectsDACLOwnerOnly(t *testing.T) {
	ctx, err := WinShmServerCreate(t.TempDir(), "dacl", 0x4243, 23, WinShmProfileHybrid, 4096, 4096)
	if err != nil {
		t.Fatalf("WinShmServerCreate: %v", err)
	}
	defer ctx.WinShmDestroy()

	// SECTION_ALL_ACCESS and EVENT_ALL_ACCESS.
	if got, want := objectDACLSDDL(t, ctx.mapping), ownerOnlyDACL(t, "CCDCLCSWRPSDRCWDWO"); got != want {
		t.Fatalf("mapping DACL = %q, want %q", got, want)
	}
	if got, want := objectDACLSDDL(t, ctx.reqEvent), ownerOnlyDACL(t, "0x1f0003"); got != want {
		t.Fatalf("request event DACL = %q, want %q", got, want)
	}
	if got, want := objectDACLSDDL(t, ctx.respEvent), ownerOnlyDACL(t, "0x1f0003"); got != want {
		t.Fatalf("response event DACL = %q, want %q", got, want)
	}
}
