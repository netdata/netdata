//! L1 Windows SHM transport.
//!
//! Shared memory data plane with spin + kernel event synchronization.
//! Uses CreateFileMappingW/MapViewOfFile for the region and auto-reset
//! kernel events for signaling (SHM_HYBRID profile).
//!
//! Wire-compatible with the C and Go implementations.

#[cfg(windows)]
mod ffi {
    #![allow(non_snake_case, non_camel_case_types, dead_code)]

    pub type HANDLE = isize;
    pub type DWORD = u32;
    pub type BOOL = i32;
    pub type LPCWSTR = *const u16;
    pub type LONG = i32;
    pub type LONG64 = i64;

    pub const INVALID_HANDLE_VALUE: HANDLE = -1;
    pub const PAGE_READWRITE: DWORD = 0x04;
    pub const FILE_MAP_ALL_ACCESS: DWORD = 0x000F001F;
    pub const EVENT_MODIFY_STATE: DWORD = 0x0002;
    pub const SYNCHRONIZE: DWORD = 0x00100000;
    pub const INFINITE: DWORD = 0xFFFFFFFF;
    pub const WAIT_OBJECT_0: DWORD = 0x00000000;
    pub const WAIT_TIMEOUT: DWORD = 0x00000102;

    extern "system" {
        pub fn CreateFileMappingW(
            hFile: HANDLE,
            lpFileMappingAttributes: *const core::ffi::c_void,
            flProtect: DWORD,
            dwMaximumSizeHigh: DWORD,
            dwMaximumSizeLow: DWORD,
            lpName: LPCWSTR,
        ) -> HANDLE;

        pub fn OpenFileMappingW(
            dwDesiredAccess: DWORD,
            bInheritHandle: BOOL,
            lpName: LPCWSTR,
        ) -> HANDLE;

        pub fn MapViewOfFile(
            hFileMappingObject: HANDLE,
            dwDesiredAccess: DWORD,
            dwFileOffsetHigh: DWORD,
            dwFileOffsetLow: DWORD,
            dwNumberOfBytesToMap: usize,
        ) -> *mut core::ffi::c_void;

        pub fn UnmapViewOfFile(lpBaseAddress: *const core::ffi::c_void) -> BOOL;

        pub fn CreateEventW(
            lpEventAttributes: *const core::ffi::c_void,
            bManualReset: BOOL,
            bInitialState: BOOL,
            lpName: LPCWSTR,
        ) -> HANDLE;

        pub fn OpenEventW(dwDesiredAccess: DWORD, bInheritHandle: BOOL, lpName: LPCWSTR) -> HANDLE;

        pub fn SetEvent(hEvent: HANDLE) -> BOOL;

        pub fn WaitForSingleObject(hHandle: HANDLE, dwMilliseconds: DWORD) -> DWORD;

        pub fn CloseHandle(hObject: HANDLE) -> BOOL;

        pub fn GetLastError() -> DWORD;
        pub fn SetLastError(dwErrCode: DWORD);

        pub fn GetTickCount() -> DWORD;
        pub fn GetTickCount64() -> u64;

        // Note: InterlockedXxx are MSVC compiler intrinsics, not linkable
        // symbols. We use Rust's std::sync::atomic instead (see helpers below).
    }
}

// ---------------------------------------------------------------------------
//  Constants
// ---------------------------------------------------------------------------

/// Magic value: "NSWH" as u32 LE.
pub const REGION_MAGIC: u32 = 0x4e535748;
pub const REGION_VERSION: u32 = 3;
pub const HEADER_LEN: u32 = 128;
pub const CACHELINE_SIZE: u32 = 64;
pub const DEFAULT_SPIN_TRIES: u32 = 1024;
pub const BUSYWAIT_POLL_MASK: u32 = 1023;

pub const PROFILE_HYBRID: u32 = 0x02;
pub const PROFILE_BUSYWAIT: u32 = 0x04;
const ERROR_ALREADY_EXISTS: u32 = 183;

// Header field byte offsets
const OFF_MAGIC: usize = 0;
const OFF_VERSION: usize = 4;
const OFF_HEADER_LEN: usize = 8;
const OFF_PROFILE: usize = 12;
const OFF_REQ_OFFSET: usize = 16;
const OFF_REQ_CAPACITY: usize = 20;
const OFF_RESP_OFFSET: usize = 24;
const OFF_RESP_CAPACITY: usize = 28;
const OFF_SPIN_TRIES: usize = 32;
const OFF_REQ_LEN: usize = 36;
const OFF_RESP_LEN: usize = 40;
const OFF_REQ_CLIENT_CLOSED: usize = 44;
const OFF_REQ_SERVER_WAITING: usize = 48;
const OFF_RESP_SERVER_CLOSED: usize = 52;
const OFF_RESP_CLIENT_WAITING: usize = 56;
const OFF_REQ_SEQ: usize = 64;
const OFF_RESP_SEQ: usize = 72;

// FNV-1a 64-bit constants
const FNV1A_OFFSET_BASIS: u64 = 0xcbf29ce484222325;
const FNV1A_PRIME: u64 = 0x00000100000001B3;

#[cfg(all(test, windows))]
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum WinShmFaultSite {
    CreateFileMapping,
    OpenFileMapping,
    MapViewOfFile,
    CreateEvent,
    OpenEvent,
}

#[cfg(all(test, windows))]
#[derive(Debug, Clone, Copy)]
struct WinShmFaultHook {
    site: WinShmFaultSite,
    error: u32,
    skip_matches: u32,
}

#[cfg(all(test, windows))]
thread_local! {
    static WIN_SHM_FAULT_HOOK: std::cell::RefCell<Option<WinShmFaultHook>> =
        const { std::cell::RefCell::new(None) };
}

// ---------------------------------------------------------------------------
//  Errors
// ---------------------------------------------------------------------------

/// Windows SHM transport errors.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum WinShmError {
    /// Invalid argument.
    BadParam(String),
    /// CreateFileMappingW failed.
    CreateMapping(u32),
    /// OpenFileMappingW failed.
    OpenMapping(u32),
    /// MapViewOfFile failed.
    MapView(u32),
    /// CreateEventW failed.
    CreateEvent(u32),
    /// OpenEventW failed.
    OpenEvent(u32),
    /// Named mapping/event already exists.
    AddrInUse,
    /// Header magic mismatch.
    BadMagic,
    /// Header version mismatch.
    BadVersion,
    /// header_len mismatch.
    BadHeader,
    /// Profile mismatch.
    BadProfile,
    /// Message exceeds area capacity.
    MsgTooLarge,
    /// Wait timed out.
    Timeout,
    /// Peer closed.
    Disconnected,
}

impl std::fmt::Display for WinShmError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            WinShmError::BadParam(s) => write!(f, "bad parameter: {s}"),
            WinShmError::CreateMapping(e) => write!(f, "CreateFileMappingW failed: {e}"),
            WinShmError::OpenMapping(e) => write!(f, "OpenFileMappingW failed: {e}"),
            WinShmError::MapView(e) => write!(f, "MapViewOfFile failed: {e}"),
            WinShmError::CreateEvent(e) => write!(f, "CreateEventW failed: {e}"),
            WinShmError::OpenEvent(e) => write!(f, "OpenEventW failed: {e}"),
            WinShmError::AddrInUse => {
                write!(f, "Windows SHM object name already in use by live server")
            }
            WinShmError::BadMagic => write!(f, "SHM header magic mismatch"),
            WinShmError::BadVersion => write!(f, "SHM header version mismatch"),
            WinShmError::BadHeader => write!(f, "SHM header_len mismatch"),
            WinShmError::BadProfile => write!(f, "SHM profile mismatch"),
            WinShmError::MsgTooLarge => write!(f, "message exceeds SHM area capacity"),
            WinShmError::Timeout => write!(f, "SHM wait timed out"),
            WinShmError::Disconnected => write!(f, "peer closed SHM session"),
        }
    }
}

impl std::error::Error for WinShmError {}

// ---------------------------------------------------------------------------
//  Role
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum WinShmRole {
    Server = 1,
    Client = 2,
}

// ---------------------------------------------------------------------------
//  Compile-time header size assertion
// ---------------------------------------------------------------------------

// We don't map a Rust struct onto the header; we use raw pointer arithmetic.
// Assert that our offset constants are consistent.
const _: () = assert!(OFF_REQ_SEQ == 64);
const _: () = assert!(OFF_RESP_SEQ == 72);
const _: () = assert!(HEADER_LEN == 128);

#[cfg(all(test, windows))]
fn install_fault_hook(site: WinShmFaultSite, error: u32, skip_matches: u32) {
    WIN_SHM_FAULT_HOOK.with(|slot| {
        let mut slot = slot.borrow_mut();
        assert!(slot.is_none(), "win_shm fault hook already installed");
        *slot = Some(WinShmFaultHook {
            site,
            error,
            skip_matches,
        });
    });
}

#[cfg(all(test, windows))]
fn clear_fault_hook() {
    WIN_SHM_FAULT_HOOK.with(|slot| {
        *slot.borrow_mut() = None;
    });
}

#[cfg(all(test, windows))]
fn take_fault_hook(site: WinShmFaultSite) -> Option<u32> {
    WIN_SHM_FAULT_HOOK.with(|slot| {
        let mut slot = slot.borrow_mut();
        match *slot {
            Some(mut hook) if hook.site == site => {
                if hook.skip_matches > 0 {
                    hook.skip_matches -= 1;
                    *slot = Some(hook);
                    None
                } else {
                    *slot = None;
                    Some(hook.error)
                }
            }
            _ => None,
        }
    })
}

#[cfg(windows)]
unsafe fn create_file_mapping(
    h_file: ffi::HANDLE,
    attrs: *const core::ffi::c_void,
    protect: u32,
    size_high: u32,
    size_low: u32,
    name: *const u16,
) -> ffi::HANDLE {
    #[cfg(test)]
    if let Some(error) = take_fault_hook(WinShmFaultSite::CreateFileMapping) {
        ffi::SetLastError(error);
        return 0;
    }

    ffi::CreateFileMappingW(h_file, attrs, protect, size_high, size_low, name)
}

#[cfg(windows)]
unsafe fn open_file_mapping(access: u32, inherit: i32, name: *const u16) -> ffi::HANDLE {
    #[cfg(test)]
    if let Some(error) = take_fault_hook(WinShmFaultSite::OpenFileMapping) {
        ffi::SetLastError(error);
        return 0;
    }

    ffi::OpenFileMappingW(access, inherit, name)
}

#[cfg(windows)]
unsafe fn map_view_of_file(
    mapping: ffi::HANDLE,
    access: u32,
    offset_high: u32,
    offset_low: u32,
    bytes: usize,
) -> *mut core::ffi::c_void {
    #[cfg(test)]
    if let Some(error) = take_fault_hook(WinShmFaultSite::MapViewOfFile) {
        ffi::SetLastError(error);
        return std::ptr::null_mut();
    }

    ffi::MapViewOfFile(mapping, access, offset_high, offset_low, bytes)
}

#[cfg(windows)]
unsafe fn create_event(
    attrs: *const core::ffi::c_void,
    manual_reset: i32,
    initial_state: i32,
    name: *const u16,
) -> ffi::HANDLE {
    #[cfg(test)]
    if let Some(error) = take_fault_hook(WinShmFaultSite::CreateEvent) {
        ffi::SetLastError(error);
        return 0;
    }

    ffi::CreateEventW(attrs, manual_reset, initial_state, name)
}

#[cfg(windows)]
unsafe fn open_event(access: u32, inherit: i32, name: *const u16) -> ffi::HANDLE {
    #[cfg(test)]
    if let Some(error) = take_fault_hook(WinShmFaultSite::OpenEvent) {
        ffi::SetLastError(error);
        return 0;
    }

    ffi::OpenEventW(access, inherit, name)
}

// ---------------------------------------------------------------------------
//  Context
// ---------------------------------------------------------------------------

/// A handle to a Windows SHM region.
#[cfg(windows)]
pub struct WinShmContext {
    role: WinShmRole,
    mapping: ffi::HANDLE,
    base: *mut u8,
    region_size: usize,

    req_event: ffi::HANDLE,
    resp_event: ffi::HANDLE,

    profile: u32,
    request_offset: u32,
    request_capacity: u32,
    response_offset: u32,
    response_capacity: u32,
    spin_tries: u32,

    local_req_seq: i64,
    local_resp_seq: i64,
}

#[cfg(windows)]
unsafe impl Send for WinShmContext {}

#[cfg(windows)]
impl WinShmContext {
    pub fn role(&self) -> WinShmRole {
        self.role
    }

    /// Create a per-session Windows SHM region (server side).
    pub fn server_create(
        run_dir: &str,
        service_name: &str,
        auth_token: u64,
        session_id: u64,
        profile: u32,
        req_capacity: u32,
        resp_capacity: u32,
    ) -> Result<Self, WinShmError> {
        validate_service_name(service_name)?;
        validate_profile(profile)?;

        let hash = compute_shm_hash(run_dir, service_name, auth_token);
        let mapping_name = build_object_name(hash, service_name, profile, session_id, "mapping")?;

        let req_cap = align_cacheline(req_capacity);
        let resp_cap = align_cacheline(resp_capacity);
        let req_off = align_cacheline(HEADER_LEN);
        let resp_off = req_off
            .checked_add(req_cap)
            .map(align_cacheline)
            .ok_or_else(|| WinShmError::BadParam("region offset overflow".into()))?;
        let region_size = resp_off
            .checked_add(resp_cap)
            .ok_or_else(|| WinShmError::BadParam("region size overflow".into()))?
            as usize;

        // Create page-file backed mapping
        unsafe { ffi::SetLastError(0) };
        let mapping = unsafe {
            create_file_mapping(
                ffi::INVALID_HANDLE_VALUE,
                std::ptr::null(),
                ffi::PAGE_READWRITE,
                (region_size >> 32) as u32,
                (region_size & 0xFFFFFFFF) as u32,
                mapping_name.as_ptr(),
            )
        };
        if mapping == 0 {
            return Err(WinShmError::CreateMapping(last_error()));
        }
        let mapping_err = last_error();
        if mapping_err == ERROR_ALREADY_EXISTS {
            unsafe { ffi::CloseHandle(mapping) };
            return Err(WinShmError::AddrInUse);
        }

        let base =
            unsafe { map_view_of_file(mapping, ffi::FILE_MAP_ALL_ACCESS, 0, 0, region_size) };
        if base.is_null() {
            let e = last_error();
            unsafe { ffi::CloseHandle(mapping) };
            return Err(WinShmError::MapView(e));
        }
        let base = base as *mut u8;

        // Zero region
        unsafe { std::ptr::write_bytes(base, 0, region_size) };

        // Write header
        write_u32(base, OFF_MAGIC, REGION_MAGIC);
        write_u32(base, OFF_VERSION, REGION_VERSION);
        write_u32(base, OFF_HEADER_LEN, HEADER_LEN);
        write_u32(base, OFF_PROFILE, profile);
        write_u32(base, OFF_REQ_OFFSET, req_off);
        write_u32(base, OFF_REQ_CAPACITY, req_cap);
        write_u32(base, OFF_RESP_OFFSET, resp_off);
        write_u32(base, OFF_RESP_CAPACITY, resp_cap);
        write_u32(base, OFF_SPIN_TRIES, DEFAULT_SPIN_TRIES);

        // Memory barrier
        std::sync::atomic::fence(std::sync::atomic::Ordering::Release);

        // Create events for HYBRID
        let (req_event, resp_event) = if profile == PROFILE_HYBRID {
            let re_name = build_object_name(hash, service_name, profile, session_id, "req_event")?;
            unsafe { ffi::SetLastError(0) };
            let re = unsafe { create_event(std::ptr::null(), 0, 0, re_name.as_ptr()) };
            if re == 0 {
                let e = last_error();
                unsafe {
                    ffi::UnmapViewOfFile(base as *const _);
                    ffi::CloseHandle(mapping);
                }
                return Err(WinShmError::CreateEvent(e));
            }
            if last_error() == ERROR_ALREADY_EXISTS {
                unsafe {
                    ffi::CloseHandle(re);
                    ffi::UnmapViewOfFile(base as *const _);
                    ffi::CloseHandle(mapping);
                }
                return Err(WinShmError::AddrInUse);
            }

            let rsp_name =
                build_object_name(hash, service_name, profile, session_id, "resp_event")?;
            unsafe { ffi::SetLastError(0) };
            let rsp = unsafe { create_event(std::ptr::null(), 0, 0, rsp_name.as_ptr()) };
            if rsp == 0 {
                let e = last_error();
                unsafe {
                    ffi::CloseHandle(re);
                    ffi::UnmapViewOfFile(base as *const _);
                    ffi::CloseHandle(mapping);
                }
                return Err(WinShmError::CreateEvent(e));
            }
            if last_error() == ERROR_ALREADY_EXISTS {
                unsafe {
                    ffi::CloseHandle(rsp);
                    ffi::CloseHandle(re);
                    ffi::UnmapViewOfFile(base as *const _);
                    ffi::CloseHandle(mapping);
                }
                return Err(WinShmError::AddrInUse);
            }
            (re, rsp)
        } else {
            (ffi::INVALID_HANDLE_VALUE, ffi::INVALID_HANDLE_VALUE)
        };

        Ok(WinShmContext {
            role: WinShmRole::Server,
            mapping,
            base,
            region_size,
            req_event,
            resp_event,
            profile,
            request_offset: req_off,
            request_capacity: req_cap,
            response_offset: resp_off,
            response_capacity: resp_cap,
            spin_tries: DEFAULT_SPIN_TRIES,
            local_req_seq: 0,
            local_resp_seq: 0,
        })
    }

    /// Attach to an existing per-session Windows SHM region (client side).
    pub fn client_attach(
        run_dir: &str,
        service_name: &str,
        auth_token: u64,
        session_id: u64,
        profile: u32,
    ) -> Result<Self, WinShmError> {
        validate_service_name(service_name)?;
        validate_profile(profile)?;

        let hash = compute_shm_hash(run_dir, service_name, auth_token);
        let mapping_name = build_object_name(hash, service_name, profile, session_id, "mapping")?;

        let mapping =
            unsafe { open_file_mapping(ffi::FILE_MAP_ALL_ACCESS, 0, mapping_name.as_ptr()) };
        if mapping == 0 {
            return Err(WinShmError::OpenMapping(last_error()));
        }

        let base = unsafe { map_view_of_file(mapping, ffi::FILE_MAP_ALL_ACCESS, 0, 0, 0) };
        if base.is_null() {
            let e = last_error();
            unsafe { ffi::CloseHandle(mapping) };
            return Err(WinShmError::MapView(e));
        }
        let base = base as *mut u8;

        // Acquire barrier
        std::sync::atomic::fence(std::sync::atomic::Ordering::Acquire);

        // Validate header
        let magic = read_u32(base, OFF_MAGIC);
        if magic != REGION_MAGIC {
            unsafe {
                ffi::UnmapViewOfFile(base as *const _);
                ffi::CloseHandle(mapping);
            }
            return Err(WinShmError::BadMagic);
        }

        let version = read_u32(base, OFF_VERSION);
        if version != REGION_VERSION {
            unsafe {
                ffi::UnmapViewOfFile(base as *const _);
                ffi::CloseHandle(mapping);
            }
            return Err(WinShmError::BadVersion);
        }

        let hdr_len = read_u32(base, OFF_HEADER_LEN);
        if hdr_len != HEADER_LEN {
            unsafe {
                ffi::UnmapViewOfFile(base as *const _);
                ffi::CloseHandle(mapping);
            }
            return Err(WinShmError::BadHeader);
        }

        let hdr_profile = read_u32(base, OFF_PROFILE);
        if hdr_profile != profile {
            unsafe {
                ffi::UnmapViewOfFile(base as *const _);
                ffi::CloseHandle(mapping);
            }
            return Err(WinShmError::BadProfile);
        }

        let req_off = read_u32(base, OFF_REQ_OFFSET);
        let req_cap = read_u32(base, OFF_REQ_CAPACITY);
        let resp_off = read_u32(base, OFF_RESP_OFFSET);
        let resp_cap = read_u32(base, OFF_RESP_CAPACITY);
        let spin = read_u32(base, OFF_SPIN_TRIES);

        // Validate header fields from shared memory
        if req_off == 0
            || req_cap == 0
            || resp_off == 0
            || resp_cap == 0
            || req_off % 64 != 0
            || req_cap % 64 != 0
            || resp_off % 64 != 0
            || resp_cap % 64 != 0
            || req_off
                .checked_add(req_cap)
                .map_or(true, |end| resp_off < end)
        {
            unsafe {
                ffi::UnmapViewOfFile(base as *const _);
                ffi::CloseHandle(mapping);
            }
            return Err(WinShmError::BadHeader);
        }

        let region_size = (resp_off as usize) + (resp_cap as usize);

        // Read current sequence numbers via interlocked
        let cur_req_seq = interlocked_read_i64(base, OFF_REQ_SEQ);
        let cur_resp_seq = interlocked_read_i64(base, OFF_RESP_SEQ);

        // Open events for HYBRID
        let (req_event, resp_event) = if profile == PROFILE_HYBRID {
            let re_name = build_object_name(hash, service_name, profile, session_id, "req_event")?;
            let re = unsafe {
                open_event(
                    ffi::EVENT_MODIFY_STATE | ffi::SYNCHRONIZE,
                    0,
                    re_name.as_ptr(),
                )
            };
            if re == 0 {
                let e = last_error();
                unsafe {
                    ffi::UnmapViewOfFile(base as *const _);
                    ffi::CloseHandle(mapping);
                }
                return Err(WinShmError::OpenEvent(e));
            }

            let rsp_name =
                build_object_name(hash, service_name, profile, session_id, "resp_event")?;
            let rsp = unsafe {
                open_event(
                    ffi::EVENT_MODIFY_STATE | ffi::SYNCHRONIZE,
                    0,
                    rsp_name.as_ptr(),
                )
            };
            if rsp == 0 {
                let e = last_error();
                unsafe {
                    ffi::CloseHandle(re);
                    ffi::UnmapViewOfFile(base as *const _);
                    ffi::CloseHandle(mapping);
                }
                return Err(WinShmError::OpenEvent(e));
            }
            (re, rsp)
        } else {
            (ffi::INVALID_HANDLE_VALUE, ffi::INVALID_HANDLE_VALUE)
        };

        Ok(WinShmContext {
            role: WinShmRole::Client,
            mapping,
            base,
            region_size,
            req_event,
            resp_event,
            profile,
            request_offset: req_off,
            request_capacity: req_cap,
            response_offset: resp_off,
            response_capacity: resp_cap,
            spin_tries: spin,
            local_req_seq: cur_req_seq,
            local_resp_seq: cur_resp_seq,
        })
    }

    /// Publish a message. Client sends to request area; server to response.
    pub fn send(&mut self, msg: &[u8]) -> Result<(), WinShmError> {
        if self.base.is_null() || msg.is_empty() {
            return Err(WinShmError::BadParam(
                "null context or empty message".into(),
            ));
        }

        let (area_off, area_cap, len_off, seq_off, peer_waiting_off, peer_event) = match self.role {
            WinShmRole::Client => (
                self.request_offset,
                self.request_capacity,
                OFF_REQ_LEN,
                OFF_REQ_SEQ,
                OFF_REQ_SERVER_WAITING,
                self.req_event,
            ),
            WinShmRole::Server => (
                self.response_offset,
                self.response_capacity,
                OFF_RESP_LEN,
                OFF_RESP_SEQ,
                OFF_RESP_CLIENT_WAITING,
                self.resp_event,
            ),
        };

        if msg.len() > area_cap as usize || msg.len() > i32::MAX as usize {
            return Err(WinShmError::MsgTooLarge);
        }

        // 1. Write message data
        unsafe {
            std::ptr::copy_nonoverlapping(
                msg.as_ptr(),
                self.base.add(area_off as usize),
                msg.len(),
            );
        }

        // 2. Store message length (interlocked exchange)
        interlocked_exchange_i32(self.base, len_off, msg.len() as i32);

        // 3. Increment sequence number
        interlocked_increment_i64(self.base, seq_off);

        // 4. If HYBRID and peer waiting, signal event
        if self.profile == PROFILE_HYBRID {
            let waiting = interlocked_read_i32(self.base, peer_waiting_off);
            if waiting != 0 {
                unsafe { ffi::SetEvent(peer_event) };
            }
        }

        match self.role {
            WinShmRole::Client => self.local_req_seq += 1,
            WinShmRole::Server => self.local_resp_seq += 1,
        }

        Ok(())
    }

    /// Receive a message into the caller-provided buffer.
    pub fn receive(&mut self, buf: &mut [u8], timeout_ms: u32) -> Result<usize, WinShmError> {
        if self.base.is_null() {
            return Err(WinShmError::BadParam("null context".into()));
        }
        if buf.is_empty() {
            return Err(WinShmError::BadParam("empty buffer".into()));
        }

        let (
            area_off,
            area_cap,
            len_off,
            seq_off,
            self_waiting_off,
            peer_closed_off,
            wait_event,
            expected_seq,
        ) = match self.role {
            WinShmRole::Server => (
                self.request_offset,
                self.request_capacity,
                OFF_REQ_LEN,
                OFF_REQ_SEQ,
                OFF_REQ_SERVER_WAITING,
                OFF_REQ_CLIENT_CLOSED,
                self.req_event,
                self.local_req_seq + 1,
            ),
            WinShmRole::Client => (
                self.response_offset,
                self.response_capacity,
                OFF_RESP_LEN,
                OFF_RESP_SEQ,
                OFF_RESP_CLIENT_WAITING,
                OFF_RESP_SERVER_CLOSED,
                self.resp_event,
                self.local_resp_seq + 1,
            ),
        };

        // The copy ceiling is the smaller of the caller buffer and the
        // SHM area capacity. Prevents out-of-bounds reads on forged lengths.
        let max_copy = std::cmp::min(buf.len(), area_cap as usize);

        // Phase 1: spin
        let mut observed = false;
        let mut mlen: i32 = 0;
        for _ in 0..self.spin_tries {
            let cur = interlocked_read_i64(self.base, seq_off);
            if cur >= expected_seq {
                mlen = interlocked_read_i32(self.base, len_off);
                if mlen > 0 && (mlen as usize) <= max_copy {
                    unsafe {
                        std::ptr::copy_nonoverlapping(
                            self.base.add(area_off as usize),
                            buf.as_mut_ptr(),
                            mlen as usize,
                        );
                    }
                }
                observed = true;
                break;
            }
            cpu_relax();
        }

        // Phase 2: kernel wait or busy-wait (deadline-based retry for
        // spurious wakes — same pattern as POSIX SHM Phase H6 fix).
        if !observed {
            if self.profile == PROFILE_HYBRID {
                let deadline_ms = if timeout_ms == 0 {
                    ffi::INFINITE
                } else {
                    timeout_ms
                };
                let start_tick = unsafe { ffi::GetTickCount64() };

                loop {
                    interlocked_exchange_i32(self.base, self_waiting_off, 1);
                    std::sync::atomic::fence(std::sync::atomic::Ordering::SeqCst);

                    let cur = interlocked_read_i64(self.base, seq_off);
                    if cur >= expected_seq {
                        interlocked_exchange_i32(self.base, self_waiting_off, 0);
                        break; // data available
                    }

                    // Compute remaining wait time
                    let wait_ms = if deadline_ms == ffi::INFINITE {
                        ffi::INFINITE
                    } else {
                        let elapsed = (unsafe { ffi::GetTickCount64() } - start_tick) as u32;
                        if elapsed >= deadline_ms {
                            interlocked_exchange_i32(self.base, self_waiting_off, 0);
                            return Err(WinShmError::Timeout);
                        }
                        deadline_ms - elapsed
                    };

                    let ret = unsafe { ffi::WaitForSingleObject(wait_event, wait_ms) };
                    interlocked_exchange_i32(self.base, self_waiting_off, 0);

                    // Check sequence — data may have arrived
                    let cur = interlocked_read_i64(self.base, seq_off);
                    if cur >= expected_seq {
                        break; // data available
                    }

                    // No data — check peer close
                    if interlocked_read_i32(self.base, peer_closed_off) != 0 {
                        let cur = interlocked_read_i64(self.base, seq_off);
                        if cur >= expected_seq {
                            break;
                        }
                        self.advance_seq(expected_seq);
                        return Err(WinShmError::Disconnected);
                    }

                    // Actual timeout (not spurious)
                    if ret == ffi::WAIT_TIMEOUT {
                        return Err(WinShmError::Timeout);
                    }

                    // Spurious wake — retry with remaining deadline
                }

                // Copy after waking
                mlen = interlocked_read_i32(self.base, len_off);
                if mlen > 0 && (mlen as usize) <= max_copy {
                    unsafe {
                        std::ptr::copy_nonoverlapping(
                            self.base.add(area_off as usize),
                            buf.as_mut_ptr(),
                            mlen as usize,
                        );
                    }
                }
            } else {
                // BUSYWAIT
                let start = unsafe { ffi::GetTickCount() };
                loop {
                    let cur = interlocked_read_i64(self.base, seq_off);
                    if cur >= expected_seq {
                        mlen = interlocked_read_i32(self.base, len_off);
                        if mlen > 0 && (mlen as usize) <= max_copy {
                            unsafe {
                                std::ptr::copy_nonoverlapping(
                                    self.base.add(area_off as usize),
                                    buf.as_mut_ptr(),
                                    mlen as usize,
                                );
                            }
                        }
                        break;
                    }

                    if timeout_ms > 0 {
                        let elapsed = unsafe { ffi::GetTickCount() }.wrapping_sub(start);
                        if elapsed >= timeout_ms {
                            return Err(WinShmError::Timeout);
                        }
                    }

                    if interlocked_read_i32(self.base, peer_closed_off) != 0 {
                        let cur = interlocked_read_i64(self.base, seq_off);
                        if cur >= expected_seq {
                            mlen = interlocked_read_i32(self.base, len_off);
                            if mlen > 0 && (mlen as usize) <= max_copy {
                                unsafe {
                                    std::ptr::copy_nonoverlapping(
                                        self.base.add(area_off as usize),
                                        buf.as_mut_ptr(),
                                        mlen as usize,
                                    );
                                }
                            }
                            break;
                        }
                        self.advance_seq(expected_seq);
                        return Err(WinShmError::Disconnected);
                    }

                    cpu_relax();
                }
            }
        }

        self.advance_seq(expected_seq);

        // mlen==0 after sequence advance indicates SHM corruption (send rejects 0-length)
        if mlen == 0 {
            return Err(WinShmError::BadHeader);
        }

        if (mlen as usize) > max_copy {
            return Err(WinShmError::MsgTooLarge);
        }

        Ok(mlen as usize)
    }

    fn advance_seq(&mut self, expected_seq: i64) {
        match self.role {
            WinShmRole::Server => self.local_req_seq = expected_seq,
            WinShmRole::Client => self.local_resp_seq = expected_seq,
        }
    }

    /// Close client (unmap, close handles, set close flag).
    pub fn close(&mut self) {
        if !self.base.is_null() {
            // Set close flag
            if self.role == WinShmRole::Client {
                interlocked_exchange_i32(self.base, OFF_REQ_CLIENT_CLOSED, 1);
            }
            std::sync::atomic::fence(std::sync::atomic::Ordering::SeqCst);
        }

        if self.profile == PROFILE_HYBRID && self.req_event != ffi::INVALID_HANDLE_VALUE {
            if self.role == WinShmRole::Client {
                unsafe { ffi::SetEvent(self.req_event) };
            }
        }

        self.cleanup_handles();
    }

    /// Destroy server (set close flag, signal, unmap, close handles).
    pub fn destroy(&mut self) {
        if !self.base.is_null() {
            interlocked_exchange_i32(self.base, OFF_RESP_SERVER_CLOSED, 1);
            std::sync::atomic::fence(std::sync::atomic::Ordering::SeqCst);
        }

        if self.profile == PROFILE_HYBRID && self.resp_event != ffi::INVALID_HANDLE_VALUE {
            unsafe { ffi::SetEvent(self.resp_event) };
        }

        self.cleanup_handles();
    }

    fn cleanup_handles(&mut self) {
        if !self.base.is_null() {
            unsafe { ffi::UnmapViewOfFile(self.base as *const _) };
            self.base = std::ptr::null_mut();
        }
        if self.mapping != 0 {
            unsafe { ffi::CloseHandle(self.mapping) };
            self.mapping = 0;
        }
        if self.req_event != ffi::INVALID_HANDLE_VALUE {
            unsafe { ffi::CloseHandle(self.req_event) };
            self.req_event = ffi::INVALID_HANDLE_VALUE;
        }
        if self.resp_event != ffi::INVALID_HANDLE_VALUE {
            unsafe { ffi::CloseHandle(self.resp_event) };
            self.resp_event = ffi::INVALID_HANDLE_VALUE;
        }
        self.region_size = 0;
    }
}

#[cfg(windows)]
impl Drop for WinShmContext {
    fn drop(&mut self) {
        match self.role {
            WinShmRole::Server => self.destroy(),
            WinShmRole::Client => self.close(),
        }
    }
}

// ---------------------------------------------------------------------------
//  Internal helpers
// ---------------------------------------------------------------------------

fn align_cacheline(v: u32) -> u32 {
    (v + (CACHELINE_SIZE - 1)) & !(CACHELINE_SIZE - 1)
}

fn validate_service_name(name: &str) -> Result<(), WinShmError> {
    if name.is_empty() {
        return Err(WinShmError::BadParam("empty service name".into()));
    }
    if name == "." || name == ".." {
        return Err(WinShmError::BadParam(
            "service name cannot be '.' or '..'".into(),
        ));
    }
    for c in name.bytes() {
        match c {
            b'a'..=b'z' | b'A'..=b'Z' | b'0'..=b'9' | b'.' | b'_' | b'-' => {}
            _ => {
                return Err(WinShmError::BadParam(format!(
                    "service name contains invalid character: {:?}",
                    c as char,
                )))
            }
        }
    }
    Ok(())
}

fn validate_profile(profile: u32) -> Result<(), WinShmError> {
    if profile != PROFILE_HYBRID && profile != PROFILE_BUSYWAIT {
        return Err(WinShmError::BadParam(format!("invalid profile: {profile}")));
    }
    Ok(())
}

/// FNV-1a 64-bit hash.
pub fn fnv1a_64(data: &[u8]) -> u64 {
    let mut hash = FNV1A_OFFSET_BASIS;
    for &b in data {
        hash ^= b as u64;
        hash = hash.wrapping_mul(FNV1A_PRIME);
    }
    hash
}

fn compute_shm_hash(run_dir: &str, service_name: &str, auth_token: u64) -> u64 {
    let input = format!("{}\n{}\n{}", run_dir, service_name, auth_token);
    fnv1a_64(input.as_bytes())
}

fn build_object_name(
    hash: u64,
    service_name: &str,
    profile: u32,
    session_id: u64,
    suffix: &str,
) -> Result<Vec<u16>, WinShmError> {
    let narrow = format!(
        "Local\\netipc-{:016x}-{}-p{}-s{:016x}-{}",
        hash, service_name, profile, session_id, suffix
    );
    if narrow.len() >= 256 {
        return Err(WinShmError::BadParam("object name too long".into()));
    }
    let mut wide: Vec<u16> = narrow.encode_utf16().collect();
    wide.push(0);
    Ok(wide)
}

#[cfg(windows)]
fn last_error() -> u32 {
    unsafe { ffi::GetLastError() }
}

fn read_u32(base: *mut u8, offset: usize) -> u32 {
    unsafe { std::ptr::read_unaligned(base.add(offset) as *const u32) }
}

fn write_u32(base: *mut u8, offset: usize, val: u32) {
    unsafe { std::ptr::write_unaligned(base.add(offset) as *mut u32, val) };
}

#[cfg(windows)]
fn interlocked_read_i32(base: *mut u8, offset: usize) -> i32 {
    use std::sync::atomic::{AtomicI32, Ordering};
    unsafe {
        let ptr = base.add(offset) as *const AtomicI32;
        (*ptr).load(Ordering::Acquire)
    }
}

#[cfg(windows)]
fn interlocked_exchange_i32(base: *mut u8, offset: usize, val: i32) {
    use std::sync::atomic::{AtomicI32, Ordering};
    unsafe {
        let ptr = base.add(offset) as *const AtomicI32;
        (*ptr).store(val, Ordering::Release);
    }
}

#[cfg(windows)]
fn interlocked_read_i64(base: *mut u8, offset: usize) -> i64 {
    use std::sync::atomic::{AtomicI64, Ordering};
    unsafe {
        let ptr = base.add(offset) as *const AtomicI64;
        (*ptr).load(Ordering::Acquire)
    }
}

#[cfg(windows)]
fn interlocked_increment_i64(base: *mut u8, offset: usize) {
    use std::sync::atomic::{AtomicI64, Ordering};
    unsafe {
        let ptr = base.add(offset) as *const AtomicI64;
        (*ptr).fetch_add(1, Ordering::Release);
    }
}

#[cfg(windows)]
#[inline]
fn cpu_relax() {
    #[cfg(any(target_arch = "x86_64", target_arch = "x86"))]
    unsafe {
        std::arch::asm!("pause", options(nomem, nostack));
    }
    #[cfg(target_arch = "aarch64")]
    unsafe {
        std::arch::asm!("yield", options(nomem, nostack));
    }
    #[cfg(not(any(target_arch = "x86_64", target_arch = "x86", target_arch = "aarch64")))]
    {
        std::sync::atomic::fence(std::sync::atomic::Ordering::SeqCst);
    }
}

// ---------------------------------------------------------------------------
//  Tests (non-Windows: compile-check only)
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[cfg(windows)]
    use std::sync::atomic::{AtomicU64, Ordering};
    #[cfg(windows)]
    use std::thread;
    #[cfg(windows)]
    use std::time::Duration;

    #[cfg(windows)]
    static WIN_SHM_TEST_COUNTER: AtomicU64 = AtomicU64::new(0);

    #[cfg(windows)]
    fn test_run_dir() -> String {
        let dir = std::env::temp_dir().join("nipc_win_shm_rust_test");
        let _ = std::fs::create_dir_all(&dir);
        dir.display().to_string()
    }

    #[cfg(windows)]
    fn unique_service(prefix: &str) -> String {
        format!(
            "{}_{}_{}",
            prefix,
            std::process::id(),
            WIN_SHM_TEST_COUNTER.fetch_add(1, Ordering::Relaxed) + 1
        )
    }

    #[cfg(windows)]
    struct FaultHookGuard;

    #[cfg(windows)]
    impl FaultHookGuard {
        fn install(site: WinShmFaultSite, error: u32) -> Self {
            install_fault_hook(site, error, 0);
            Self
        }

        fn install_after(site: WinShmFaultSite, error: u32, skip_matches: u32) -> Self {
            install_fault_hook(site, error, skip_matches);
            Self
        }
    }

    #[cfg(windows)]
    impl Drop for FaultHookGuard {
        fn drop(&mut self) {
            clear_fault_hook();
        }
    }

    #[test]
    fn test_fnv1a_64_deterministic() {
        let h1 = fnv1a_64(b"C:\\Temp\\netdata\ncgroups-snapshot\n12345");
        let h2 = fnv1a_64(b"C:\\Temp\\netdata\ncgroups-snapshot\n12345");
        assert_eq!(h1, h2);
        assert_ne!(h1, 0);
    }

    #[test]
    fn test_fnv1a_64_different_tokens() {
        let h1 = fnv1a_64(b"dir\nsvc\n100");
        let h2 = fnv1a_64(b"dir\nsvc\n200");
        assert_ne!(h1, h2);
    }

    #[test]
    fn test_validate_service_name() {
        assert!(validate_service_name("cgroups-snapshot").is_ok());
        assert!(validate_service_name("test.v1").is_ok());
        assert!(validate_service_name("").is_err());
        assert!(validate_service_name(".").is_err());
        assert!(validate_service_name("..").is_err());
        assert!(validate_service_name("has/slash").is_err());
        assert!(validate_service_name("has space").is_err());
    }

    #[test]
    fn test_align_cacheline() {
        assert_eq!(align_cacheline(0), 0);
        assert_eq!(align_cacheline(1), 64);
        assert_eq!(align_cacheline(64), 64);
        assert_eq!(align_cacheline(65), 128);
        assert_eq!(align_cacheline(128), 128);
    }

    #[test]
    fn test_object_name_format() {
        let name = build_object_name(0xDEADBEEF, "test-svc", 2, 0x42, "mapping").unwrap();
        // Check it's NUL-terminated
        assert_eq!(*name.last().unwrap(), 0);
        let narrow: String = name[..name.len() - 1]
            .iter()
            .map(|&c| c as u8 as char)
            .collect();
        assert!(narrow.starts_with("Local\\netipc-"));
        assert!(narrow.contains("-test-svc-p2-s0000000000000042-mapping"));
    }

    #[test]
    fn test_error_display_variants() {
        let cases = [
            (WinShmError::BadParam("oops".into()), "bad parameter: oops"),
            (
                WinShmError::CreateMapping(5),
                "CreateFileMappingW failed: 5",
            ),
            (WinShmError::OpenMapping(6), "OpenFileMappingW failed: 6"),
            (WinShmError::MapView(7), "MapViewOfFile failed: 7"),
            (WinShmError::CreateEvent(8), "CreateEventW failed: 8"),
            (WinShmError::OpenEvent(9), "OpenEventW failed: 9"),
            (
                WinShmError::AddrInUse,
                "Windows SHM object name already in use by live server",
            ),
            (WinShmError::BadMagic, "SHM header magic mismatch"),
            (WinShmError::BadVersion, "SHM header version mismatch"),
            (WinShmError::BadHeader, "SHM header_len mismatch"),
            (WinShmError::BadProfile, "SHM profile mismatch"),
            (
                WinShmError::MsgTooLarge,
                "message exceeds SHM area capacity",
            ),
            (WinShmError::Timeout, "SHM wait timed out"),
            (WinShmError::Disconnected, "peer closed SHM session"),
        ];

        for (err, expected) in cases {
            assert_eq!(err.to_string(), expected);
        }
    }

    #[test]
    fn test_build_object_name_too_long() {
        let long_service = "a".repeat(240);
        let err = build_object_name(0xDEADBEEF, &long_service, PROFILE_HYBRID, 1, "mapping")
            .expect_err("object name should be rejected");
        assert_eq!(err, WinShmError::BadParam("object name too long".into()));
    }

    #[test]
    fn test_validate_profile_rejects_invalid() {
        let err = validate_profile(0).expect_err("invalid profile should fail");
        assert_eq!(err, WinShmError::BadParam("invalid profile: 0".into()));
    }

    #[cfg(windows)]
    #[test]
    fn test_client_attach_bad_magic_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_bad_magic");
        let auth_token: u64 = 0x123456;
        let session_id: u64 = 15;

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create");

        write_u32(server.base, OFF_MAGIC, 0);
        let err = match WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        ) {
            Ok(_) => panic!("bad magic must fail"),
            Err(err) => err,
        };
        assert_eq!(err, WinShmError::BadMagic);

        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_server_create_rejects_existing_objects_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_addr_in_use");
        let auth_token: u64 = 0x424242;
        let session_id: u64 = 22;

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create");

        let err = match WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        ) {
            Ok(_) => panic!("duplicate server_create must fail"),
            Err(err) => err,
        };
        assert_eq!(err, WinShmError::AddrInUse);

        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_role_send_and_receive_guards_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_guards");
        let auth_token: u64 = 0xabcdef;
        let session_id: u64 = 31;

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            128,
            128,
        )
        .expect("server_create");
        assert_eq!(server.role(), WinShmRole::Server);

        let mut client = WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        )
        .expect("client_attach");
        assert_eq!(client.role(), WinShmRole::Client);

        let empty_send = client.send(&[]).expect_err("empty send must fail");
        assert_eq!(
            empty_send,
            WinShmError::BadParam("null context or empty message".into())
        );

        let oversize = vec![0u8; 256];
        let oversize_err = client.send(&oversize).expect_err("oversize send must fail");
        assert_eq!(oversize_err, WinShmError::MsgTooLarge);

        let empty_buf_err = server
            .receive(&mut [], 10)
            .expect_err("empty receive buffer must fail");
        assert_eq!(empty_buf_err, WinShmError::BadParam("empty buffer".into()));

        client.close();

        let mut buf = [0u8; 16];
        let null_ctx_err = client
            .receive(&mut buf, 10)
            .expect_err("closed client must reject receive");
        assert_eq!(null_ctx_err, WinShmError::BadParam("null context".into()));

        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_client_attach_bad_version_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_bad_version");
        let auth_token: u64 = 0x123457;
        let session_id: u64 = 16;

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create");

        write_u32(server.base, OFF_VERSION, REGION_VERSION + 1);
        let err = match WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        ) {
            Ok(_) => panic!("bad version must fail"),
            Err(err) => err,
        };
        assert_eq!(err, WinShmError::BadVersion);

        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_client_attach_bad_header_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_bad_header");
        let auth_token: u64 = 0x123458;
        let session_id: u64 = 17;

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create");

        write_u32(server.base, OFF_HEADER_LEN, HEADER_LEN + 64);
        let err = match WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        ) {
            Ok(_) => panic!("bad header_len must fail"),
            Err(err) => err,
        };
        assert_eq!(err, WinShmError::BadHeader);

        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_client_attach_bad_profile_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_bad_profile");
        let auth_token: u64 = 0x123459;
        let session_id: u64 = 18;

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create");

        write_u32(server.base, OFF_PROFILE, PROFILE_BUSYWAIT);
        let err = match WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        ) {
            Ok(_) => panic!("bad profile must fail"),
            Err(err) => err,
        };
        assert_eq!(err, WinShmError::BadProfile);

        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_receive_timeout_hybrid_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_timeout_h");
        let auth_token: u64 = 0x8912;
        let session_id: u64 = 19;

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create");
        let mut client = WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        )
        .expect("client_attach");

        let mut buf = [0u8; 128];
        assert_eq!(server.receive(&mut buf, 10), Err(WinShmError::Timeout));

        client.close();
        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_receive_timeout_busywait_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_timeout_b");
        let auth_token: u64 = 0x8913;
        let session_id: u64 = 20;

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_BUSYWAIT,
            4096,
            4096,
        )
        .expect("server_create");
        let mut client = WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_BUSYWAIT,
        )
        .expect("client_attach");

        let mut buf = [0u8; 128];
        assert_eq!(server.receive(&mut buf, 10), Err(WinShmError::Timeout));

        client.close();
        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_receive_detects_peer_closed_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_peer_closed");
        let auth_token: u64 = 0x789a;
        let session_id: u64 = 21;

        let server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create");
        let mut client = WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        )
        .expect("client_attach");

        let (sender, receiver) = std::sync::mpsc::channel();

        let server_ptr = std::sync::Arc::new(std::sync::Mutex::new(server));
        let recv_server = server_ptr.clone();
        let handle = thread::spawn(move || {
            let mut buf = [0u8; 128];
            let mut guard = recv_server.lock().unwrap();
            let recv = guard.receive(&mut buf, 1000);
            sender.send(recv).unwrap();
        });

        thread::sleep(Duration::from_millis(20));
        client.close();

        let recv = receiver
            .recv_timeout(Duration::from_secs(2))
            .expect("receive result");
        assert_eq!(recv, Err(WinShmError::Disconnected));

        handle.join().expect("receiver thread");
        let mutex = match std::sync::Arc::try_unwrap(server_ptr) {
            Ok(mutex) => mutex,
            Err(_) => panic!("receiver thread still holds the SHM server"),
        };
        let mut server = mutex.into_inner().expect("mutex");
        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_server_create_fault_create_mapping_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_fault_create_mapping");
        let auth_token: u64 = 0x5511;
        let session_id: u64 = 41;

        let _fault = FaultHookGuard::install(WinShmFaultSite::CreateFileMapping, 5);
        let err = match WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        ) {
            Ok(_) => panic!("CreateFileMappingW fault must fail"),
            Err(err) => err,
        };
        assert_eq!(err, WinShmError::CreateMapping(5));
        drop(_fault);

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create after CreateFileMappingW fault");
        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_server_create_fault_map_view_releases_mapping_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_fault_map_view");
        let auth_token: u64 = 0x5512;
        let session_id: u64 = 42;

        let _fault = FaultHookGuard::install(WinShmFaultSite::MapViewOfFile, 6);
        let err = match WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        ) {
            Ok(_) => panic!("MapViewOfFile fault must fail"),
            Err(err) => err,
        };
        assert_eq!(err, WinShmError::MapView(6));
        drop(_fault);

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create after MapViewOfFile fault");
        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_server_create_fault_req_event_releases_mapping_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_fault_req_event");
        let auth_token: u64 = 0x5513;
        let session_id: u64 = 43;

        let _fault = FaultHookGuard::install(WinShmFaultSite::CreateEvent, 7);
        let err = match WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        ) {
            Ok(_) => panic!("req_event CreateEventW fault must fail"),
            Err(err) => err,
        };
        assert_eq!(err, WinShmError::CreateEvent(7));
        drop(_fault);

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create after req_event fault");
        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_server_create_fault_resp_event_releases_partial_objects_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_fault_resp_event");
        let auth_token: u64 = 0x5514;
        let session_id: u64 = 44;

        let _fault = FaultHookGuard::install_after(WinShmFaultSite::CreateEvent, 8, 1);
        let err = match WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        ) {
            Ok(_) => panic!("second CreateEventW fault must fail"),
            Err(err) => err,
        };
        assert_eq!(err, WinShmError::CreateEvent(8));
        drop(_fault);

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create after resp_event fault");
        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_client_attach_fault_open_mapping_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_fault_open_mapping");
        let auth_token: u64 = 0x6611;
        let session_id: u64 = 51;

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create");

        let _fault = FaultHookGuard::install(WinShmFaultSite::OpenFileMapping, 9);
        let err = match WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        ) {
            Ok(_) => panic!("OpenFileMappingW fault must fail"),
            Err(err) => err,
        };
        assert_eq!(err, WinShmError::OpenMapping(9));
        drop(_fault);

        let mut client = WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        )
        .expect("client_attach after OpenFileMappingW fault");
        client.close();
        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_client_attach_fault_map_view_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_fault_attach_map_view");
        let auth_token: u64 = 0x6612;
        let session_id: u64 = 52;

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create");

        let _fault = FaultHookGuard::install(WinShmFaultSite::MapViewOfFile, 10);
        let err = match WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        ) {
            Ok(_) => panic!("MapViewOfFile attach fault must fail"),
            Err(err) => err,
        };
        assert_eq!(err, WinShmError::MapView(10));
        drop(_fault);

        let mut client = WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        )
        .expect("client_attach after MapViewOfFile fault");
        client.close();
        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_client_attach_fault_req_event_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_fault_req_open_event");
        let auth_token: u64 = 0x6613;
        let session_id: u64 = 53;

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create");

        let _fault = FaultHookGuard::install(WinShmFaultSite::OpenEvent, 11);
        let err = match WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        ) {
            Ok(_) => panic!("req_event OpenEventW fault must fail"),
            Err(err) => err,
        };
        assert_eq!(err, WinShmError::OpenEvent(11));
        drop(_fault);

        let mut client = WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        )
        .expect("client_attach after req_event OpenEventW fault");
        client.close();
        server.destroy();
    }

    #[cfg(windows)]
    #[test]
    fn test_client_attach_fault_resp_event_windows() {
        let run_dir = test_run_dir();
        let service = unique_service("rs_win_shm_fault_resp_open_event");
        let auth_token: u64 = 0x6614;
        let session_id: u64 = 54;

        let mut server = WinShmContext::server_create(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create");

        let _fault = FaultHookGuard::install_after(WinShmFaultSite::OpenEvent, 12, 1);
        let err = match WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        ) {
            Ok(_) => panic!("resp_event OpenEventW fault must fail"),
            Err(err) => err,
        };
        assert_eq!(err, WinShmError::OpenEvent(12));
        drop(_fault);

        let mut client = WinShmContext::client_attach(
            &run_dir,
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        )
        .expect("client_attach after resp_event OpenEventW fault");
        client.close();
        server.destroy();
    }
}
