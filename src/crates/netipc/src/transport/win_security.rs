//! Access policy shared by the Windows transports.
//!
//! Every NetIPC pipe, SHM mapping and event grants access only to SYSTEM,
//! Administrators and the account of the process that creates it. Clients
//! accept a server only when the pipe owner is one of those same principals.

use std::ffi::{CStr, CString, c_char, c_void};
use std::ptr;

#[allow(non_snake_case, dead_code)]
mod ffi {
    use std::ffi::{c_char, c_void};

    pub type HANDLE = isize;
    pub type BOOL = i32;

    pub const TOKEN_QUERY: u32 = 0x0008;
    pub const TOKEN_USER_CLASS: u32 = 1;
    pub const SE_KERNEL_OBJECT: i32 = 6;
    pub const OWNER_SECURITY_INFORMATION: u32 = 0x1;
    pub const DACL_SECURITY_INFORMATION: u32 = 0x4;
    pub const SDDL_REVISION_1: u32 = 1;

    #[repr(C)]
    pub struct SECURITY_ATTRIBUTES {
        pub nLength: u32,
        pub lpSecurityDescriptor: *mut c_void,
        pub bInheritHandle: BOOL,
    }

    #[link(name = "advapi32")]
    unsafe extern "system" {
        pub fn OpenProcessToken(process: HANDLE, access: u32, token: *mut HANDLE) -> BOOL;
        pub fn GetTokenInformation(
            token: HANDLE,
            class: u32,
            info: *mut c_void,
            len: u32,
            ret_len: *mut u32,
        ) -> BOOL;
        pub fn ConvertSidToStringSidA(sid: *mut c_void, out: *mut *mut c_char) -> BOOL;
        pub fn ConvertStringSecurityDescriptorToSecurityDescriptorA(
            sddl: *const c_char,
            revision: u32,
            sd: *mut *mut c_void,
            sd_len: *mut u32,
        ) -> BOOL;
        pub fn ConvertSecurityDescriptorToStringSecurityDescriptorA(
            sd: *mut c_void,
            revision: u32,
            info: u32,
            out: *mut *mut c_char,
            out_len: *mut u32,
        ) -> BOOL;
        pub fn GetSecurityInfo(
            handle: HANDLE,
            object_type: i32,
            info: u32,
            owner: *mut *mut c_void,
            group: *mut *mut c_void,
            dacl: *mut *mut c_void,
            sacl: *mut *mut c_void,
            sd: *mut *mut c_void,
        ) -> u32;
    }

    unsafe extern "system" {
        pub fn GetCurrentProcess() -> HANDLE;
        pub fn CloseHandle(handle: HANDLE) -> BOOL;
        pub fn LocalFree(mem: *mut c_void) -> *mut c_void;
    }
}

const SYSTEM_SID: &str = "S-1-5-18";
const ADMINISTRATORS_SID: &str = "S-1-5-32-544";

/// A SID or descriptor as an owned string; the Win32 buffer is LocalFree'd.
unsafe fn take_local_string(s: *mut c_char) -> String {
    let out = unsafe { CStr::from_ptr(s) }.to_string_lossy().into_owned();
    unsafe { ffi::LocalFree(s.cast()) };
    out
}

/// The SID string of this process's account.
pub(crate) fn process_user_sid() -> Option<String> {
    let mut token: ffi::HANDLE = 0;
    if unsafe { ffi::OpenProcessToken(ffi::GetCurrentProcess(), ffi::TOKEN_QUERY, &mut token) } == 0
    {
        return None;
    }

    let mut len = 0u32;
    unsafe { ffi::GetTokenInformation(token, ffi::TOKEN_USER_CLASS, ptr::null_mut(), 0, &mut len) };
    // TOKEN_USER starts with the SID pointer; u64 keeps it aligned.
    let mut buf = vec![0u64; (len as usize).div_ceil(8).max(1)];
    let ok = unsafe {
        ffi::GetTokenInformation(
            token,
            ffi::TOKEN_USER_CLASS,
            buf.as_mut_ptr().cast(),
            (buf.len() * 8) as u32,
            &mut len,
        )
    } != 0;
    unsafe { ffi::CloseHandle(token) };
    if !ok {
        return None;
    }

    let sid = unsafe { *(buf.as_ptr() as *const *mut c_void) };
    let mut s: *mut c_char = ptr::null_mut();
    if unsafe { ffi::ConvertSidToStringSidA(sid, &mut s) } == 0 {
        return None;
    }
    Some(unsafe { take_local_string(s) })
}

/// Security attributes carrying the owner-only DACL. The pointer from
/// `as_ptr` is valid while this value lives.
pub(crate) struct OwnerOnlyAttributes {
    sa: ffi::SECURITY_ATTRIBUTES,
}

impl OwnerOnlyAttributes {
    /// Builds the descriptor, or returns the Win32 error.
    pub(crate) fn new() -> Result<Self, u32> {
        let sid = process_user_sid().ok_or_else(last_error)?;
        // Protected DACL: nothing is inherited, and no ACE for Everyone or
        // Anonymous, so other accounts cannot even open an instance read-only.
        let sddl = CString::new(format!("D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;{sid})"))
            .map_err(|_| 87u32)?;
        let mut sd: *mut c_void = ptr::null_mut();
        if unsafe {
            ffi::ConvertStringSecurityDescriptorToSecurityDescriptorA(
                sddl.as_ptr(),
                ffi::SDDL_REVISION_1,
                &mut sd,
                ptr::null_mut(),
            )
        } == 0
        {
            return Err(last_error());
        }
        Ok(Self {
            sa: ffi::SECURITY_ATTRIBUTES {
                nLength: std::mem::size_of::<ffi::SECURITY_ATTRIBUTES>() as u32,
                lpSecurityDescriptor: sd,
                bInheritHandle: 0,
            },
        })
    }

    pub(crate) fn as_ptr(&self) -> *const c_void {
        (&self.sa as *const ffi::SECURITY_ATTRIBUTES).cast()
    }
}

impl Drop for OwnerOnlyAttributes {
    fn drop(&mut self) {
        unsafe { ffi::LocalFree(self.sa.lpSecurityDescriptor) };
    }
}

fn last_error() -> u32 {
    std::io::Error::last_os_error().raw_os_error().unwrap_or(0) as u32
}

/// The object's owner as a SID string.
pub(crate) fn object_owner_sid(handle: isize) -> Option<String> {
    let mut owner: *mut c_void = ptr::null_mut();
    let mut sd: *mut c_void = ptr::null_mut();
    if unsafe {
        ffi::GetSecurityInfo(
            handle,
            ffi::SE_KERNEL_OBJECT,
            ffi::OWNER_SECURITY_INFORMATION,
            &mut owner,
            ptr::null_mut(),
            ptr::null_mut(),
            ptr::null_mut(),
            &mut sd,
        )
    } != 0
    {
        return None;
    }
    let mut s: *mut c_char = ptr::null_mut();
    let ok = !owner.is_null() && unsafe { ffi::ConvertSidToStringSidA(owner, &mut s) } != 0;
    unsafe { ffi::LocalFree(sd) };
    ok.then(|| unsafe { take_local_string(s) })
}

#[cfg(test)]
thread_local! {
    /// Clients on this thread trust only pipes owned by this SID instead of
    /// their own account, SYSTEM and Administrators.
    pub(crate) static TRUSTED_OWNER_FOR_TESTS: std::cell::RefCell<Option<String>> =
        const { std::cell::RefCell::new(None) };
}

/// True when the object's owner is this process's account, SYSTEM or
/// Administrators.
pub(crate) fn owner_trusted(handle: isize) -> bool {
    let Some(owner) = object_owner_sid(handle) else {
        return false;
    };
    #[cfg(test)]
    if let Some(trusted) = TRUSTED_OWNER_FOR_TESTS.with(|t| t.borrow().clone()) {
        return owner == trusted;
    }
    owner == SYSTEM_SID
        || owner == ADMINISTRATORS_SID
        || process_user_sid().is_some_and(|me| owner == me)
}

/// The object's DACL in SDDL form, for tests.
#[cfg(test)]
pub(crate) fn object_dacl_sddl(handle: isize) -> Option<String> {
    let mut sd: *mut c_void = ptr::null_mut();
    if unsafe {
        ffi::GetSecurityInfo(
            handle,
            ffi::SE_KERNEL_OBJECT,
            ffi::DACL_SECURITY_INFORMATION,
            ptr::null_mut(),
            ptr::null_mut(),
            ptr::null_mut(),
            ptr::null_mut(),
            &mut sd,
        )
    } != 0
    {
        return None;
    }
    let out = descriptor_dacl_sddl(sd);
    unsafe { ffi::LocalFree(sd) };
    out
}

/// The owner-only DACL for `mask` as Windows prints it, for tests. The round
/// trip applies Windows' own SID aliases, such as LA for the built-in
/// Administrator account that CI runners use.
#[cfg(test)]
pub(crate) fn owner_only_dacl_sddl(mask: &str) -> Option<String> {
    let me = process_user_sid()?;
    let sddl = CString::new(format!(
        "D:P(A;;{mask};;;SY)(A;;{mask};;;BA)(A;;{mask};;;{me})"
    ))
    .ok()?;
    let mut sd: *mut c_void = ptr::null_mut();
    if unsafe {
        ffi::ConvertStringSecurityDescriptorToSecurityDescriptorA(
            sddl.as_ptr(),
            ffi::SDDL_REVISION_1,
            &mut sd,
            ptr::null_mut(),
        )
    } == 0
    {
        return None;
    }
    let out = descriptor_dacl_sddl(sd);
    unsafe { ffi::LocalFree(sd) };
    out
}

#[cfg(test)]
fn descriptor_dacl_sddl(sd: *mut c_void) -> Option<String> {
    let mut s: *mut c_char = ptr::null_mut();
    let ok = unsafe {
        ffi::ConvertSecurityDescriptorToStringSecurityDescriptorA(
            sd,
            ffi::SDDL_REVISION_1,
            ffi::DACL_SECURITY_INFORMATION,
            &mut s,
            ptr::null_mut(),
        )
    } != 0;
    ok.then(|| unsafe { take_local_string(s) })
}

/// SecurityIdentification in SECURITY_IMPERSONATION_LEVEL.
#[cfg(test)]
pub(crate) const SECURITY_IDENTIFICATION_LEVEL: u32 = 1;

/// The impersonation level a pipe client granted this server, for tests.
#[cfg(test)]
pub(crate) fn client_impersonation_level(pipe: isize) -> Option<u32> {
    use test_ffi::*;
    const TOKEN_IMPERSONATION_LEVEL_CLASS: u32 = 9;

    if unsafe { ImpersonateNamedPipeClient(pipe) } == 0 {
        return None;
    }
    let mut token = 0isize;
    let mut level = None;
    if unsafe { OpenThreadToken(GetCurrentThread(), ffi::TOKEN_QUERY, 1, &mut token) } != 0 {
        let mut value = 0u32;
        let mut len = 0u32;
        if unsafe {
            GetTokenInformation(
                token,
                TOKEN_IMPERSONATION_LEVEL_CLASS,
                (&mut value as *mut u32).cast(),
                4,
                &mut len,
            )
        } != 0
        {
            level = Some(value);
        }
        unsafe { CloseHandle(token) };
    }
    unsafe { RevertToSelf() };
    level
}

#[cfg(test)]
mod test_ffi {
    use std::ffi::c_void;

    #[link(name = "advapi32")]
    unsafe extern "system" {
        pub fn ImpersonateNamedPipeClient(pipe: isize) -> i32;
        pub fn RevertToSelf() -> i32;
        pub fn OpenThreadToken(thread: isize, access: u32, as_self: i32, token: *mut isize) -> i32;
        pub fn GetTokenInformation(
            token: isize,
            class: u32,
            info: *mut c_void,
            len: u32,
            ret_len: *mut u32,
        ) -> i32;
    }

    unsafe extern "system" {
        pub fn GetCurrentThread() -> isize;
        pub fn CloseHandle(handle: isize) -> i32;
    }
}
