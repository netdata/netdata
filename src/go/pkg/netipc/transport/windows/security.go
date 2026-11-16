//go:build windows

package windows

import (
	"syscall"
	"unsafe"
)

// Access policy shared by the Windows transports: every NetIPC pipe, SHM
// mapping and event grants access only to SYSTEM, Administrators and the
// account of the process that creates it. Clients accept a server only when
// the pipe owner is one of those same principals.

const (
	_PIPE_REJECT_REMOTE_CLIENTS = 0x00000008
	_SECURITY_SQOS_PRESENT      = 0x00100000
	_SECURITY_IDENTIFICATION    = 0x00010000
	_SE_KERNEL_OBJECT           = 6
	_OWNER_SECURITY_INFORMATION = 0x1
	_DACL_SECURITY_INFORMATION  = 0x4
	_SDDL_REVISION_1            = 1
	systemSID                   = "S-1-5-18"
	administratorsSID           = "S-1-5-32-544"
)

var (
	modadvapi32 = syscall.NewLazyDLL("advapi32.dll")

	procConvertStringSDToSD = modadvapi32.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
	procGetSecurityInfo     = modadvapi32.NewProc("GetSecurityInfo")
)

// trustedOwnerForTests, when set by tests, replaces the trusted owners with
// this one SID string.
var trustedOwnerForTests string

// processUserSID returns the SID string of this process's account.
func processUserSID() (string, error) {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return "", err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String()
}

// ownerOnlyAttributes holds security attributes with the owner-only DACL.
// Call free once the object is created.
type ownerOnlyAttributes struct {
	sa syscall.SecurityAttributes
}

func newOwnerOnlyAttributes() (*ownerOnlyAttributes, error) {
	sid, err := processUserSID()
	if err != nil {
		return nil, err
	}
	// Protected DACL: nothing is inherited, and no ACE for Everyone or
	// Anonymous, so other accounts cannot even open an instance read-only.
	sddl, err := syscall.UTF16PtrFromString("D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;" + sid + ")")
	if err != nil {
		return nil, err
	}
	var sd uintptr
	r, _, callErr := procConvertStringSDToSD.Call(
		uintptr(unsafe.Pointer(sddl)), _SDDL_REVISION_1, uintptr(unsafe.Pointer(&sd)), 0)
	if r == 0 {
		return nil, callErr
	}
	a := &ownerOnlyAttributes{}
	a.sa.Length = uint32(unsafe.Sizeof(a.sa))
	a.sa.SecurityDescriptor = sd
	return a, nil
}

func (a *ownerOnlyAttributes) ptr() uintptr { return uintptr(unsafe.Pointer(&a.sa)) }

func (a *ownerOnlyAttributes) free() {
	if a.sa.SecurityDescriptor != 0 {
		syscall.LocalFree(syscall.Handle(a.sa.SecurityDescriptor))
		a.sa.SecurityDescriptor = 0
	}
}

// objectOwnerSID returns the object's owner as a SID string.
func objectOwnerSID(handle syscall.Handle) (string, error) {
	var owner *syscall.SID
	var sd uintptr
	r, _, _ := procGetSecurityInfo.Call(uintptr(handle), _SE_KERNEL_OBJECT,
		_OWNER_SECURITY_INFORMATION, uintptr(unsafe.Pointer(&owner)), 0, 0, 0,
		uintptr(unsafe.Pointer(&sd)))
	if r != 0 {
		return "", syscall.Errno(r)
	}
	defer syscall.LocalFree(syscall.Handle(sd))
	return owner.String()
}

// ownerTrusted reports whether the object's owner is this process's account,
// SYSTEM or Administrators.
func ownerTrusted(handle syscall.Handle) bool {
	owner, err := objectOwnerSID(handle)
	if err != nil {
		return false
	}
	if trustedOwnerForTests != "" {
		return owner == trustedOwnerForTests
	}
	if owner == systemSID || owner == administratorsSID {
		return true
	}
	me, err := processUserSID()
	return err == nil && owner == me
}
