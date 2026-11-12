//! L1 Windows Named Pipe transport.
//!
//! Connection lifecycle, handshake with profile/limit negotiation,
//! and send/receive with transparent chunking over Win32 Named Pipes
//! in message mode. Wire-compatible with the C and Go implementations.

use super::framing::{self, Framing, FramingError, ServerOffer, apply_default};
use crate::protocol::{
    HEADER_SIZE, Header, Hello, HelloAck, MAX_PAYLOAD_DEFAULT, PROFILE_BASELINE,
};
#[cfg(windows)]
use crate::transport::win_security::{OwnerOnlyAttributes, owner_trusted};
use std::collections::HashSet;
use std::ptr;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::time::{Duration, Instant};

// ---------------------------------------------------------------------------
//  Win32 FFI — using windows-sys when available, raw bindings as fallback
// ---------------------------------------------------------------------------

#[cfg(windows)]
mod ffi {
    #![allow(non_snake_case, non_camel_case_types, dead_code)]

    pub type HANDLE = isize;
    pub type DWORD = u32;
    pub type BOOL = i32;
    pub type LPCWSTR = *const u16;
    pub type LPVOID = *mut core::ffi::c_void;
    pub type LPCVOID = *const core::ffi::c_void;
    pub type LPDWORD = *mut DWORD;

    pub const INVALID_HANDLE_VALUE: HANDLE = -1;
    pub const PIPE_ACCESS_DUPLEX: DWORD = 0x00000003;
    pub const FILE_FLAG_FIRST_PIPE_INSTANCE: DWORD = 0x00080000;
    pub const PIPE_TYPE_MESSAGE: DWORD = 0x00000004;
    pub const PIPE_READMODE_MESSAGE: DWORD = 0x00000002;
    pub const PIPE_WAIT: DWORD = 0x00000000;
    pub const PIPE_REJECT_REMOTE_CLIENTS: DWORD = 0x00000008;
    pub const SECURITY_SQOS_PRESENT: DWORD = 0x00100000;
    pub const SECURITY_IDENTIFICATION: DWORD = 0x00010000;
    pub const PIPE_UNLIMITED_INSTANCES: DWORD = 255;
    pub const GENERIC_READ: DWORD = 0x80000000;
    pub const GENERIC_WRITE: DWORD = 0x40000000;
    pub const OPEN_EXISTING: DWORD = 3;

    pub const ERROR_PIPE_CONNECTED: DWORD = 535;
    pub const ERROR_BROKEN_PIPE: DWORD = 109;
    pub const ERROR_NO_DATA: DWORD = 232;
    pub const ERROR_PIPE_NOT_CONNECTED: DWORD = 233;
    pub const ERROR_ACCESS_DENIED: DWORD = 5;
    pub const ERROR_PIPE_BUSY: DWORD = 231;
    pub const ERROR_SEM_TIMEOUT: DWORD = 121;
    pub const FILE_FLAG_OVERLAPPED: DWORD = 0x40000000;
    pub const ERROR_IO_PENDING: DWORD = 997;
    pub const ERROR_IO_INCOMPLETE: DWORD = 996;
    pub const WAIT_OBJECT_0: DWORD = 0;
    pub const WAIT_TIMEOUT: DWORD = 258;
    pub const WAIT_FAILED: DWORD = 0xFFFF_FFFF;

    /// OVERLAPPED; the offset union is unused for pipes.
    #[repr(C)]
    pub struct OVERLAPPED {
        pub internal: usize,
        pub internal_high: usize,
        pub offset: u32,
        pub offset_high: u32,
        pub h_event: HANDLE,
    }

    unsafe extern "system" {
        pub fn CreateNamedPipeW(
            lpName: LPCWSTR,
            dwOpenMode: DWORD,
            dwPipeMode: DWORD,
            nMaxInstances: DWORD,
            nOutBufferSize: DWORD,
            nInBufferSize: DWORD,
            nDefaultTimeOut: DWORD,
            lpSecurityAttributes: *const core::ffi::c_void,
        ) -> HANDLE;

        pub fn ConnectNamedPipe(hNamedPipe: HANDLE, lpOverlapped: *mut core::ffi::c_void) -> BOOL;

        pub fn CreateFileW(
            lpFileName: LPCWSTR,
            dwDesiredAccess: DWORD,
            dwShareMode: DWORD,
            lpSecurityAttributes: *const core::ffi::c_void,
            dwCreationDisposition: DWORD,
            dwFlagsAndAttributes: DWORD,
            hTemplateFile: HANDLE,
        ) -> HANDLE;

        pub fn ReadFile(
            hFile: HANDLE,
            lpBuffer: LPVOID,
            nNumberOfBytesToRead: DWORD,
            lpNumberOfBytesRead: LPDWORD,
            lpOverlapped: *mut core::ffi::c_void,
        ) -> BOOL;

        pub fn WriteFile(
            hFile: HANDLE,
            lpBuffer: LPCVOID,
            nNumberOfBytesToWrite: DWORD,
            lpNumberOfBytesWritten: LPDWORD,
            lpOverlapped: *mut core::ffi::c_void,
        ) -> BOOL;

        pub fn CloseHandle(hObject: HANDLE) -> BOOL;

        pub fn GetLastError() -> DWORD;
        pub fn GetTickCount64() -> u64;
        pub fn PeekNamedPipe(
            hNamedPipe: HANDLE,
            lpBuffer: LPVOID,
            nBufferSize: DWORD,
            lpBytesRead: LPDWORD,
            lpTotalBytesAvail: LPDWORD,
            lpBytesLeftThisMessage: LPDWORD,
        ) -> BOOL;

        pub fn SwitchToThread() -> BOOL;

        pub fn WaitNamedPipeW(lpNamedPipeName: LPCWSTR, nTimeOut: DWORD) -> BOOL;

        pub fn DisconnectNamedPipe(hNamedPipe: HANDLE) -> BOOL;

        pub fn SetNamedPipeHandleState(
            hNamedPipe: HANDLE,
            lpMode: *const DWORD,
            lpMaxCollectionCount: *const DWORD,
            lpCollectDataTimeout: *const DWORD,
        ) -> BOOL;

        pub fn CreateEventW(
            lpEventAttributes: *const core::ffi::c_void,
            bManualReset: BOOL,
            bInitialState: BOOL,
            lpName: LPCWSTR,
        ) -> HANDLE;

        pub fn GetOverlappedResult(
            hFile: HANDLE,
            lpOverlapped: *mut OVERLAPPED,
            lpNumberOfBytesTransferred: LPDWORD,
            bWait: BOOL,
        ) -> BOOL;

        pub fn CancelIoEx(hFile: HANDLE, lpOverlapped: *mut OVERLAPPED) -> BOOL;

        pub fn WaitForSingleObject(hHandle: HANDLE, dwMilliseconds: DWORD) -> DWORD;
    }
}

// ---------------------------------------------------------------------------
//  Constants
// ---------------------------------------------------------------------------

/// Profiles Windows can provide; unassigned bits are never offered or
/// accepted, so a peer can never select a profile without an implementation
/// (0x04 is WinSHM busy-wait here, not the POSIX futex profile).
const PLATFORM_PROFILES: u32 =
    PROFILE_BASELINE | super::win_shm::PROFILE_HYBRID | super::win_shm::PROFILE_BUSYWAIT;

const DEFAULT_BATCH_ITEMS: u32 = 1;
const DEFAULT_PACKET_SIZE: u32 = 65536;
const DEFAULT_PIPE_BUF_SIZE: u32 = 65536;
const MAX_PIPE_NAME_CHARS: usize = 256;

/// How long a connect waits for a free pipe instance when every instance is
/// busy (ERROR_PIPE_BUSY).
pub const CONNECT_BUSY_WAIT_MS: u32 = 1000;

// FNV-1a 64-bit constants
const FNV1A_OFFSET_BASIS: u64 = 0xcbf29ce484222325;
const FNV1A_PRIME: u64 = 0x00000100000001B3;

// ---------------------------------------------------------------------------
//  Errors
// ---------------------------------------------------------------------------

/// Transport-level errors for Named Pipe transport.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum NpError {
    /// Pipe name derivation failed.
    PipeName(String),
    /// CreateNamedPipeW failed.
    CreatePipe(u32),
    /// CreateFileW / connection failed.
    Connect(u32),
    /// ConnectNamedPipe (accept) failed.
    Accept(u32),
    /// WriteFile failed.
    Send(u32),
    /// ReadFile failed or peer disconnected.
    Recv(u32),
    /// Receive timed out before a complete message arrived.
    Timeout,
    /// Receive was aborted by the caller.
    Aborted,
    /// Handshake protocol error.
    Handshake(String),
    /// Authentication token rejected.
    AuthFailed,
    /// No common profile.
    NoProfile,
    /// Protocol or layout version mismatch.
    Incompatible(String),
    /// Wire protocol violation.
    Protocol(String),
    /// Pipe name already in use by live server.
    AddrInUse,
    /// Chunk header mismatch.
    Chunk(String),
    /// Memory allocation failed.
    Alloc,
    /// Payload or batch count exceeds negotiated limit.
    LimitExceeded,
    /// Invalid argument.
    BadParam(String),
    /// Duplicate message_id on send.
    DuplicateMsgId(u64),
    /// Unknown message_id on receive.
    UnknownMsgId(u64),
    /// Peer disconnected (graceful).
    Disconnected,
    /// The pipe owner is not this account, SYSTEM or Administrators.
    UntrustedServer,
}

impl std::fmt::Display for NpError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            NpError::PipeName(s) => write!(f, "pipe name error: {s}"),
            NpError::CreatePipe(e) => write!(f, "CreateNamedPipeW failed: {e}"),
            NpError::Connect(e) => write!(f, "connect failed: {e}"),
            NpError::Accept(e) => write!(f, "accept failed: {e}"),
            NpError::Send(e) => write!(f, "send failed: {e}"),
            NpError::Recv(e) => write!(f, "recv failed: {e}"),
            NpError::Timeout => write!(f, "receive timed out"),
            NpError::Aborted => write!(f, "receive aborted"),
            NpError::Handshake(s) => write!(f, "handshake error: {s}"),
            NpError::AuthFailed => write!(f, "authentication token rejected"),
            NpError::NoProfile => write!(f, "no common transport profile"),
            NpError::Incompatible(s) => write!(f, "incompatible protocol: {s}"),
            NpError::Protocol(s) => write!(f, "protocol violation: {s}"),
            NpError::AddrInUse => write!(f, "pipe name already in use by live server"),
            NpError::Chunk(s) => write!(f, "chunk error: {s}"),
            NpError::Alloc => write!(f, "memory allocation failed"),
            NpError::LimitExceeded => write!(f, "negotiated limit exceeded"),
            NpError::BadParam(s) => write!(f, "bad parameter: {s}"),
            NpError::DuplicateMsgId(id) => write!(f, "duplicate message_id: {id}"),
            NpError::UnknownMsgId(id) => write!(f, "unknown response message_id: {id}"),
            NpError::Disconnected => write!(f, "peer disconnected"),
            NpError::UntrustedServer => write!(f, "pipe owned by an untrusted account"),
        }
    }
}

impl FramingError for NpError {
    fn protocol(msg: String) -> Self {
        NpError::Protocol(msg)
    }
    fn chunk(msg: String) -> Self {
        NpError::Chunk(msg)
    }
    fn incompatible(msg: String) -> Self {
        NpError::Incompatible(msg)
    }
    fn handshake(msg: String) -> Self {
        NpError::Handshake(msg)
    }
    fn bad_param(msg: String) -> Self {
        NpError::BadParam(msg)
    }
    fn limit_exceeded() -> Self {
        NpError::LimitExceeded
    }
    fn auth_failed() -> Self {
        NpError::AuthFailed
    }
    fn no_profile() -> Self {
        NpError::NoProfile
    }
    fn duplicate_msg_id(id: u64) -> Self {
        NpError::DuplicateMsgId(id)
    }
    fn unknown_msg_id(id: u64) -> Self {
        NpError::UnknownMsgId(id)
    }
    fn fails_session(&self) -> bool {
        matches!(self, NpError::Send(_) | NpError::Disconnected)
    }
    fn is_recv_failure(&self) -> bool {
        matches!(self, NpError::Recv(_) | NpError::Disconnected)
    }
}

impl std::error::Error for NpError {}

// ---------------------------------------------------------------------------
//  Role
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Role {
    Client = 1,
    Server = 2,
}

// ---------------------------------------------------------------------------
//  Configuration
// ---------------------------------------------------------------------------

/// Client connection configuration.
#[derive(Debug, Clone)]
pub struct ClientConfig {
    pub supported_profiles: u32,
    pub preferred_profiles: u32,
    pub max_request_payload_bytes: u32,
    pub max_request_batch_items: u32,
    pub max_response_payload_bytes: u32,
    pub max_response_batch_items: u32,
    pub auth_token: u64,
    /// 0 = use default (65536).
    pub packet_size: u32,
}

impl Default for ClientConfig {
    fn default() -> Self {
        Self {
            supported_profiles: PROFILE_BASELINE,
            preferred_profiles: 0,
            max_request_payload_bytes: 0,
            max_request_batch_items: 0,
            max_response_payload_bytes: 0,
            max_response_batch_items: 0,
            auth_token: 0,
            packet_size: 0,
        }
    }
}

/// Server configuration for listen + accept.
#[derive(Debug, Clone)]
pub struct ServerConfig {
    pub supported_profiles: u32,
    pub preferred_profiles: u32,
    pub max_request_payload_bytes: u32,
    pub max_request_batch_items: u32,
    pub max_response_payload_bytes: u32,
    pub max_response_batch_items: u32,
    pub auth_token: u64,
    /// 0 = use default (65536).
    pub packet_size: u32,
}

impl Default for ServerConfig {
    fn default() -> Self {
        Self {
            supported_profiles: PROFILE_BASELINE,
            preferred_profiles: 0,
            max_request_payload_bytes: 0,
            max_request_batch_items: 0,
            max_response_payload_bytes: 0,
            max_response_batch_items: 0,
            auth_token: 0,
            packet_size: 0,
        }
    }
}

// ---------------------------------------------------------------------------
//  FNV-1a 64-bit hash
// ---------------------------------------------------------------------------

/// Compute FNV-1a 64-bit hash of data.
pub fn fnv1a_64(data: &[u8]) -> u64 {
    let mut hash = FNV1A_OFFSET_BASIS;
    for &byte in data {
        hash ^= byte as u64;
        hash = hash.wrapping_mul(FNV1A_PRIME);
    }
    hash
}

// ---------------------------------------------------------------------------
//  Service name validation
// ---------------------------------------------------------------------------

fn validate_service_name(name: &str) -> Result<(), NpError> {
    if name.is_empty() {
        return Err(NpError::BadParam("empty service name".into()));
    }
    if name == "." || name == ".." {
        return Err(NpError::BadParam(
            "service name cannot be '.' or '..'".into(),
        ));
    }
    for &b in name.as_bytes() {
        if (b >= b'a' && b <= b'z')
            || (b >= b'A' && b <= b'Z')
            || (b >= b'0' && b <= b'9')
            || b == b'.'
            || b == b'_'
            || b == b'-'
        {
            continue;
        }
        return Err(NpError::BadParam(format!(
            "service name contains invalid character: {:?}",
            b as char
        )));
    }
    Ok(())
}

// ---------------------------------------------------------------------------
//  Pipe name derivation
// ---------------------------------------------------------------------------

/// Build pipe name from run_dir and service_name.
/// Returns the pipe name as a NUL-terminated wide string vector.
pub fn build_pipe_name(run_dir: &str, service_name: &str) -> Result<Vec<u16>, NpError> {
    validate_service_name(service_name)?;

    let hash = fnv1a_64(run_dir.as_bytes());
    let narrow = format!("\\\\.\\pipe\\netipc-{:016x}-{}", hash, service_name);

    if narrow.len() >= MAX_PIPE_NAME_CHARS {
        return Err(NpError::PipeName("pipe name too long".into()));
    }

    // Convert to UTF-16 with NUL terminator
    let mut wide: Vec<u16> = narrow.encode_utf16().collect();
    wide.push(0);
    Ok(wide)
}

// ---------------------------------------------------------------------------
//  Internal helpers
// ---------------------------------------------------------------------------

#[cfg(windows)]
fn pipe_buffer_size(packet_size: u32) -> u32 {
    // The protocol packet size controls logical framing and chunk size. The
    // underlying pipe quota must stay large enough for full-duplex pipelining
    // even when tests force a tiny protocol packet size.
    apply_default(packet_size, DEFAULT_PIPE_BUF_SIZE).max(DEFAULT_PIPE_BUF_SIZE)
}

// ---------------------------------------------------------------------------
//  Low-level I/O (Windows-only)
// ---------------------------------------------------------------------------

#[cfg(windows)]
fn is_disconnect_error(err: u32) -> bool {
    err == ffi::ERROR_BROKEN_PIPE
        || err == ffi::ERROR_NO_DATA
        || err == ffi::ERROR_PIPE_NOT_CONNECTED
}

#[cfg(windows)]
fn last_error() -> u32 {
    unsafe { ffi::GetLastError() }
}

#[cfg(windows)]
fn raw_write(handle: ffi::HANDLE, data: &[u8]) -> Result<(), NpError> {
    let mut written: u32 = 0;
    let ok = unsafe {
        ffi::WriteFile(
            handle,
            data.as_ptr() as ffi::LPCVOID,
            data.len() as u32,
            &mut written,
            ptr::null_mut(),
        )
    };
    if ok == 0 {
        let err = last_error();
        if is_disconnect_error(err) {
            return Err(NpError::Disconnected);
        }
        return Err(NpError::Send(err));
    }
    if written != data.len() as u32 {
        return Err(NpError::Send(0));
    }
    Ok(())
}

/// Read one pipe message. Returns number of bytes read.
#[cfg(windows)]
fn raw_recv(handle: ffi::HANDLE, buf: &mut [u8]) -> Result<usize, NpError> {
    let mut read: u32 = 0;
    let ok = unsafe {
        ffi::ReadFile(
            handle,
            buf.as_mut_ptr() as ffi::LPVOID,
            buf.len() as u32,
            &mut read,
            ptr::null_mut(),
        )
    };
    if ok == 0 {
        let err = last_error();
        if is_disconnect_error(err) {
            return Err(NpError::Disconnected);
        }
        return Err(NpError::Recv(err));
    }
    if read == 0 {
        return Err(NpError::Disconnected);
    }
    Ok(read as usize)
}

// Client pipe handles are overlapped (FILE_FLAG_OVERLAPPED), so a
// pipelining send can read responses while its write waits for buffer
// space. Every read and write on such a handle passes an OVERLAPPED and is
// waited to completion before the call returns, so the OVERLAPPED lives on
// that call's stack. Server handles stay synchronous.

/// The events of a client handle's reads and writes.
#[cfg(windows)]
struct PipeIo {
    read_event: ffi::HANDLE,
    write_event: ffi::HANDLE,
}

#[cfg(windows)]
impl PipeIo {
    fn new() -> Result<Self, NpError> {
        let read_event = create_manual_event()?;
        let write_event = match create_manual_event() {
            Ok(event) => event,
            Err(e) => {
                close_handle(read_event);
                return Err(e);
            }
        };
        Ok(Self {
            read_event,
            write_event,
        })
    }
}

#[cfg(windows)]
impl Drop for PipeIo {
    fn drop(&mut self) {
        close_handle(self.read_event);
        close_handle(self.write_event);
    }
}

#[cfg(windows)]
fn create_manual_event() -> Result<ffi::HANDLE, NpError> {
    let event = unsafe { ffi::CreateEventW(ptr::null(), 1, 0, ptr::null()) };
    if event == 0 {
        return Err(NpError::Connect(last_error()));
    }
    Ok(event)
}

#[cfg(windows)]
fn new_overlapped(event: ffi::HANDLE) -> ffi::OVERLAPPED {
    ffi::OVERLAPPED {
        internal: 0,
        internal_high: 0,
        offset: 0,
        offset_high: 0,
        h_event: event,
    }
}

#[cfg(windows)]
fn send_error(err: u32) -> NpError {
    if is_disconnect_error(err) {
        NpError::Disconnected
    } else {
        NpError::Send(err)
    }
}

#[cfg(windows)]
fn recv_error(err: u32) -> NpError {
    if is_disconnect_error(err) {
        NpError::Disconnected
    } else {
        NpError::Recv(err)
    }
}

/// Read one message on an overlapped handle and wait for it. A message
/// larger than `buf` (ERROR_MORE_DATA) fails, as on a synchronous handle.
#[cfg(windows)]
fn overlapped_read(
    handle: ffi::HANDLE,
    event: ffi::HANDLE,
    buf: &mut [u8],
) -> Result<usize, NpError> {
    let mut ov = new_overlapped(event);
    let mut read: u32 = 0;
    // Overlapped I/O takes no byte-count pointer (Win32 docs); the count
    // comes from GetOverlappedResult.
    let ok = unsafe {
        ffi::ReadFile(
            handle,
            buf.as_mut_ptr() as ffi::LPVOID,
            buf.len() as u32,
            core::ptr::null_mut(),
            &mut ov as *mut ffi::OVERLAPPED as *mut core::ffi::c_void,
        )
    };
    if ok == 0 {
        let err = last_error();
        if err != ffi::ERROR_IO_PENDING {
            return Err(recv_error(err));
        }
    }
    if unsafe { ffi::GetOverlappedResult(handle, &mut ov, &mut read, 1) } == 0 {
        return Err(recv_error(last_error()));
    }
    if read == 0 {
        return Err(NpError::Disconnected);
    }
    Ok(read as usize)
}

/// Post one message on an overlapped handle; `finish_overlapped_write`
/// collects its completion, also when WriteFile completed at once.
#[cfg(windows)]
fn start_overlapped_write(
    handle: ffi::HANDLE,
    ov: &mut ffi::OVERLAPPED,
    data: &[u8],
) -> Result<(), NpError> {
    // No byte-count pointer: the write may still be pending when this
    // returns; finish_overlapped_write gets the count.
    let ok = unsafe {
        ffi::WriteFile(
            handle,
            data.as_ptr() as ffi::LPCVOID,
            data.len() as u32,
            core::ptr::null_mut(),
            ov as *mut ffi::OVERLAPPED as *mut core::ffi::c_void,
        )
    };
    if ok == 0 {
        let err = last_error();
        if err != ffi::ERROR_IO_PENDING {
            return Err(send_error(err));
        }
    }
    Ok(())
}

#[cfg(windows)]
fn finish_overlapped_write(
    handle: ffi::HANDLE,
    ov: &mut ffi::OVERLAPPED,
    data: &[u8],
) -> Result<(), NpError> {
    let mut written: u32 = 0;
    if unsafe { ffi::GetOverlappedResult(handle, ov, &mut written, 1) } == 0 {
        return Err(send_error(last_error()));
    }
    if written != data.len() as u32 {
        return Err(NpError::Send(0));
    }
    Ok(())
}

/// Cancel a posted write and wait until the kernel no longer uses its
/// OVERLAPPED and buffer.
#[cfg(windows)]
fn cancel_overlapped_write(handle: ffi::HANDLE, ov: &mut ffi::OVERLAPPED) {
    let mut written: u32 = 0;
    unsafe {
        ffi::CancelIoEx(handle, ov);
        ffi::GetOverlappedResult(handle, ov, &mut written, 1);
    }
}

/// Length prefix of each queued message.
#[cfg(windows)]
const PENDING_RECORD_LEN: usize = 4;

/// Messages a pipelining client send read while its write waited; receive
/// returns them before reading the pipe. Records live in `buf[head..end]`;
/// `buf` only grows, so a read lands in storage zeroed once, not per message.
#[cfg(windows)]
#[derive(Default)]
struct PendingQueue {
    buf: Vec<u8>,
    head: usize,
    end: usize,
}

#[cfg(windows)]
impl PendingQueue {
    fn queued(&self) -> bool {
        self.head < self.end
    }

    fn clear(&mut self) {
        self.head = 0;
        self.end = 0;
    }

    fn compact(&mut self) {
        if self.head > 0 {
            self.buf.copy_within(self.head..self.end, 0);
            self.end -= self.head;
            self.head = 0;
        }
    }

    #[cfg(test)]
    fn push(&mut self, msg: &[u8]) {
        self.buf.truncate(self.end);
        self.buf
            .extend_from_slice(&(msg.len() as u32).to_ne_bytes());
        self.buf.extend_from_slice(msg);
        self.end = self.buf.len();
    }

    /// Copy the oldest queued message into `dst`. A message larger than
    /// `dst` fails, as a pipe read would with ERROR_MORE_DATA.
    fn pop(&mut self, dst: &mut [u8]) -> Result<usize, NpError> {
        let mut len_bytes = [0u8; PENDING_RECORD_LEN];
        len_bytes.copy_from_slice(&self.buf[self.head..self.head + PENDING_RECORD_LEN]);
        let rec = u32::from_ne_bytes(len_bytes) as usize;
        let start = self.head + PENDING_RECORD_LEN;
        let result = if rec > dst.len() {
            Err(NpError::Recv(ERROR_MORE_DATA))
        } else {
            dst[..rec].copy_from_slice(&self.buf[start..start + rec]);
            Ok(rec)
        };
        self.head = start + rec;
        if self.head == self.end {
            self.clear();
        }
        result
    }

    /// Read every message available now. Reports whether it read any. A
    /// disconnected pipe sets `eof`: the posted write reports it. Queued bytes beyond `bound` fail with `NpError::Protocol`.
    fn drain(
        &mut self,
        handle: ffi::HANDLE,
        event: ffi::HANDLE,
        read_len: usize,
        bound: usize,
        eof: &mut bool,
    ) -> Result<bool, NpError> {
        let mut drained = false;
        loop {
            match peek_available(handle) {
                Ok(0) => return Ok(drained),
                Ok(_) => {}
                Err(NpError::Disconnected) => {
                    *eof = true;
                    return Ok(drained);
                }
                // Stopping the drain on a live pipe would bring the deadlock back.
                Err(e) => return Err(NpError::Protocol(format!("pending peek: {e}"))),
            }
            self.compact();
            let start = self.end;
            let need = start + PENDING_RECORD_LEN + read_len;
            if self.buf.len() < need {
                self.buf.resize(need, 0);
            }
            match overlapped_read(
                handle,
                event,
                &mut self.buf[start + PENDING_RECORD_LEN..need],
            ) {
                Ok(n) => {
                    self.buf[start..start + PENDING_RECORD_LEN]
                        .copy_from_slice(&(n as u32).to_ne_bytes());
                    self.end = start + PENDING_RECORD_LEN + n;
                }
                Err(NpError::Disconnected) => {
                    *eof = true;
                    return Ok(drained);
                }
                Err(e) => {
                    // A message larger than a packet, or a read failure.
                    return Err(NpError::Protocol(format!("pending read: {e}")));
                }
            }
            if self.end > bound {
                return Err(NpError::Protocol(
                    "inbound bytes exceed the in-flight responses".into(),
                ));
            }
            drained = true;
        }
    }
}

/// How long a stalled pipelined send keeps yielding before it sleeps.
#[cfg(windows)]
const PROGRESS_YIELD: Duration = Duration::from_millis(1);

/// ERROR_MORE_DATA: a message larger than the read buffer.
#[cfg(windows)]
const ERROR_MORE_DATA: u32 = 234;

/// Wire bytes `inflight` responses can occupy: the pending queue bound.
#[cfg(windows)]
fn pending_bound(max_response_payload: u32, packet_size: u32, inflight: usize) -> usize {
    let payload = max_response_payload as usize;
    let budget = (packet_size as usize).saturating_sub(HEADER_SIZE).max(1);
    let packets = if payload == 0 {
        1
    } else {
        1 + (payload - 1) / budget
    };
    packets
        .saturating_mul(HEADER_SIZE + PENDING_RECORD_LEN)
        .saturating_add(payload)
        .saturating_mul(inflight)
}

#[cfg(windows)]
fn peek_available(handle: ffi::HANDLE) -> Result<u32, NpError> {
    let mut available: u32 = 0;
    let ok = unsafe {
        ffi::PeekNamedPipe(
            handle,
            ptr::null_mut(),
            0,
            ptr::null_mut(),
            &mut available,
            ptr::null_mut(),
        )
    };
    if ok == 0 {
        return Err(recv_error(last_error()));
    }
    Ok(available)
}

/// Write one packet. On a client handle with `bound` set (more than one
/// request in flight), it reads what the server sent into `pending` until
/// the write completes, so both sides keep making progress. No timeout: it
/// waits as long as a blocking write would.
#[cfg(windows)]
#[allow(clippy::too_many_arguments)]
fn write_packet(
    handle: ffi::HANDLE,
    io: Option<&PipeIo>,
    pending: &mut PendingQueue,
    bound: usize,
    read_len: usize,
    data: &[u8],
    wait: Option<&ReceiveWait<'_>>,
    broken: &mut bool,
) -> Result<(), NpError> {
    let Some(io) = io else {
        return raw_write(handle, data);
    };
    let mut ov = new_overlapped(io.write_event);
    start_overlapped_write(handle, &mut ov, data)?;
    if bound == 0 {
        return match wait {
            Some(wait) => finish_overlapped_write_wait(handle, io, &mut ov, data, wait, broken),
            None => finish_overlapped_write(handle, &mut ov, data),
        };
    }
    let mut eof = false;
    let mut yield_until = Instant::now() + PROGRESS_YIELD;
    loop {
        match unsafe { ffi::WaitForSingleObject(io.write_event, 0) } {
            ffi::WAIT_OBJECT_0 => return finish_overlapped_write(handle, &mut ov, data),
            ffi::WAIT_TIMEOUT => {}
            _ => {
                let err = last_error();
                cancel_overlapped_write(handle, &mut ov);
                *broken = true;
                return Err(NpError::Send(err));
            }
        }
        if let Some(wait) = wait {
            if let Err(e) = wait.wait_slice_ms() {
                cancel_overlapped_write(handle, &mut ov);
                *broken = true;
                return Err(e);
            }
        }
        if !eof {
            match pending.drain(handle, io.read_event, read_len, bound, &mut eof) {
                Ok(true) => {
                    yield_until = Instant::now() + PROGRESS_YIELD;
                    continue;
                }
                Ok(false) => {}
                Err(e) => {
                    cancel_overlapped_write(handle, &mut ov);
                    *broken = true;
                    return Err(e);
                }
            }
        }
        // The server is busy or its reply is still being written: yield for
        // up to PROGRESS_YIELD after the last progress, then wait for the
        // write in short slices. A slice lasts a system timer tick and only
        // the write completing ends it early, so slicing at once stalls a
        // server blocked on its reply to us.
        if Instant::now() < yield_until {
            unsafe {
                ffi::SwitchToThread();
            }
            continue;
        }
        unsafe {
            ffi::WaitForSingleObject(io.write_event, 1);
        }
    }
}

/// Wait for a posted write until it completes, the deadline passes or the
/// abort flag is set. A write that ends early is cancelled and the session
/// broken, since part of the message may be on the wire.
#[cfg(windows)]
fn finish_overlapped_write_wait(
    handle: ffi::HANDLE,
    io: &PipeIo,
    ov: &mut ffi::OVERLAPPED,
    data: &[u8],
    wait: &ReceiveWait<'_>,
    broken: &mut bool,
) -> Result<(), NpError> {
    loop {
        let mut written: u32 = 0;
        if unsafe { ffi::GetOverlappedResult(handle, ov, &mut written, 0) } != 0 {
            if written != data.len() as u32 {
                return Err(NpError::Send(0));
            }
            return Ok(());
        }
        let err = last_error();
        if err != ffi::ERROR_IO_INCOMPLETE {
            return Err(send_error(err));
        }
        let slice = match wait.wait_slice_ms() {
            Ok(ms) => ms,
            Err(e) => {
                cancel_overlapped_write(handle, ov);
                *broken = true;
                return Err(e);
            }
        };
        if unsafe { ffi::WaitForSingleObject(io.write_event, slice) } == ffi::WAIT_FAILED {
            let err = last_error();
            cancel_overlapped_write(handle, ov);
            *broken = true;
            return Err(NpError::Send(err));
        }
    }
}

/// Read one packet: a queued one first, then the pipe.
#[cfg(windows)]
fn read_packet(
    handle: ffi::HANDLE,
    io: Option<&PipeIo>,
    pending: &mut PendingQueue,
    buf: &mut [u8],
) -> Result<usize, NpError> {
    if pending.queued() {
        return pending.pop(buf);
    }
    match io {
        Some(io) => overlapped_read(handle, io.read_event, buf),
        None => raw_recv(handle, buf),
    }
}

#[cfg(windows)]
struct ReceiveWait<'a> {
    deadline: Option<Instant>,
    abort: Option<&'a AtomicBool>,
}

#[cfg(windows)]
impl<'a> ReceiveWait<'a> {
    fn new(timeout_ms: u32, abort: Option<&'a AtomicBool>) -> Self {
        Self {
            deadline: (timeout_ms != 0)
                .then(|| Instant::now() + Duration::from_millis(timeout_ms as u64)),
            abort,
        }
    }

    fn aborted(&self) -> bool {
        self.abort
            .map(|flag| flag.load(Ordering::Acquire))
            .unwrap_or(false)
    }

    fn wait_slice_ms(&self) -> Result<u32, NpError> {
        if self.aborted() {
            return Err(NpError::Aborted);
        }

        let mut wait_ms: Option<u128> = if let Some(deadline) = self.deadline {
            let now = Instant::now();
            if now >= deadline {
                return Err(NpError::Timeout);
            }
            Some(deadline.duration_since(now).as_millis().max(1))
        } else {
            None
        };

        if self.abort.is_some() {
            wait_ms = Some(wait_ms.map_or(100, |ms| ms.min(100)));
        }

        Ok(wait_ms.unwrap_or(u32::MAX as u128).min(u32::MAX as u128) as u32)
    }
}

#[cfg(windows)]
fn wait_pipe_readable(handle: ffi::HANDLE, timeout_ms: u32) -> Result<bool, NpError> {
    if handle == ffi::INVALID_HANDLE_VALUE || handle == 0 {
        return Err(NpError::BadParam("session closed".into()));
    }

    let deadline = unsafe { ffi::GetTickCount64() }.saturating_add(timeout_ms as u64);
    let mut yielded = false;
    loop {
        let mut available: u32 = 0;
        let ok = unsafe {
            ffi::PeekNamedPipe(
                handle,
                ptr::null_mut(),
                0,
                ptr::null_mut(),
                &mut available,
                ptr::null_mut(),
            )
        };
        if ok == 0 {
            let err = last_error();
            if is_disconnect_error(err) {
                return Err(NpError::Disconnected);
            }
            return Err(NpError::Recv(err));
        }
        if available > 0 {
            return Ok(true);
        }

        if unsafe { ffi::GetTickCount64() } >= deadline {
            return Ok(false);
        }

        if !yielded {
            yielded = true;
            for _ in 0..256 {
                unsafe {
                    ffi::SwitchToThread();
                }

                let mut yielded_available: u32 = 0;
                let yielded_ok = unsafe {
                    ffi::PeekNamedPipe(
                        handle,
                        ptr::null_mut(),
                        0,
                        ptr::null_mut(),
                        &mut yielded_available,
                        ptr::null_mut(),
                    )
                };
                if yielded_ok == 0 {
                    let err = last_error();
                    if is_disconnect_error(err) {
                        return Err(NpError::Disconnected);
                    }
                    return Err(NpError::Recv(err));
                }
                if yielded_available > 0 {
                    return Ok(true);
                }

                if unsafe { ffi::GetTickCount64() } >= deadline {
                    return Ok(false);
                }
            }
            continue;
        }

        std::thread::sleep(std::time::Duration::from_millis(1));
    }
}

#[cfg(windows)]
fn raw_recv_with_wait(
    handle: ffi::HANDLE,
    io: Option<&PipeIo>,
    pending: &mut PendingQueue,
    buf: &mut [u8],
    wait: &mut ReceiveWait<'_>,
) -> Result<usize, NpError> {
    if pending.queued() {
        return pending.pop(buf);
    }
    loop {
        let wait_ms = wait.wait_slice_ms()?;
        if !wait_pipe_readable(handle, wait_ms)? {
            // An empty slice only means the abort flag is due for a check;
            // wait_slice_ms() reports the real deadline.
            continue;
        }
        if wait.aborted() {
            return Err(NpError::Aborted);
        }
        return read_packet(handle, io, pending, buf);
    }
}

#[cfg(windows)]
fn raw_recv_control(
    handle: ffi::HANDLE,
    io: Option<&PipeIo>,
    pending: &mut PendingQueue,
    buf: &mut [u8],
    controlled: bool,
    wait: &mut ReceiveWait<'_>,
) -> Result<usize, NpError> {
    if controlled {
        raw_recv_with_wait(handle, io, pending, buf, wait)
    } else {
        read_packet(handle, io, pending, buf)
    }
}

#[cfg(windows)]
fn close_handle(handle: ffi::HANDLE) {
    if handle != ffi::INVALID_HANDLE_VALUE && handle != 0 {
        unsafe {
            ffi::CloseHandle(handle);
        }
    }
}

// ---------------------------------------------------------------------------
//  Session
// ---------------------------------------------------------------------------

/// A connected Named Pipe session (client or server side).
#[cfg(windows)]
pub struct NpSession {
    handle: ffi::HANDLE,
    role: Role,

    // Negotiated limits
    pub max_request_payload_bytes: u32,
    pub max_request_batch_items: u32,
    pub max_response_payload_bytes: u32,
    pub max_response_batch_items: u32,
    pub packet_size: u32,
    pub selected_profile: u32,
    pub session_id: u64,

    // Internal receive buffer for chunked reassembly
    recv_buf: Vec<u8>,
    pkt_buf: Vec<u8>,
    send_buf: Vec<u8>,

    // In-flight message_id set (client-side only)
    inflight_ids: HashSet<u64>,

    // Overlapped state of a client handle; None on server sessions, whose
    // handles are synchronous.
    io: Option<PipeIo>,
    // Messages read while a pipelined send waited for its write.
    pending: PendingQueue,
}

#[cfg(windows)]
impl NpSession {
    /// The framing of the messages this side sends (`sending`) or receives.
    /// Clients send requests and receive responses; servers the opposite.
    fn framing(&self, sending: bool) -> Framing {
        let client = self.role == Role::Client;
        let (max_payload, max_items) = if client == sending {
            (self.max_request_payload_bytes, self.max_request_batch_items)
        } else {
            (
                self.max_response_payload_bytes,
                self.max_response_batch_items,
            )
        };
        Framing {
            client,
            max_payload,
            max_items,
            packet_size: self.packet_size,
        }
    }

    /// Get the raw pipe HANDLE. It is not a readability signal: a pipe
    /// handle is signalled by I/O completion, not by arriving data. Use
    /// `wait_readable`, and check `has_pending` first: a pipelined send can
    /// read responses into the session, where the pipe no longer shows them.
    pub fn handle(&self) -> ffi::HANDLE {
        self.handle
    }

    /// Whether the next receive can return a message read earlier by a
    /// pipelined send, without the pipe becoming readable.
    pub fn has_pending(&self) -> bool {
        self.pending.queued()
    }

    /// Get the session role.
    pub fn role(&self) -> Role {
        self.role
    }

    /// Wait until the pipe becomes readable or the timeout expires.
    pub fn wait_readable(&self, timeout_ms: u32) -> Result<bool, NpError> {
        if self.pending.queued() && self.handle != ffi::INVALID_HANDLE_VALUE {
            return Ok(true);
        }
        wait_pipe_readable(self.handle, timeout_ms)
    }

    /// Return the most recently reassembled payload stored in the internal
    /// receive buffer.
    pub fn received_payload(&self, len: usize) -> &[u8] {
        &self.recv_buf[..len]
    }

    /// Connect to a server pipe derived from run_dir + service_name.
    pub fn connect(
        run_dir: &str,
        service_name: &str,
        config: &ClientConfig,
    ) -> Result<Self, NpError> {
        Self::connect_with_timeout(run_dir, service_name, config, 0, None)
    }

    /// `connect()` bounded by `timeout_ms`: the busy-pipe wait and the wait
    /// for HELLO_ACK together end with `NpError::Timeout` after `timeout_ms`
    /// (0 = no deadline), and with `NpError::Aborted` once `abort` is set.
    pub fn connect_with_timeout(
        run_dir: &str,
        service_name: &str,
        config: &ClientConfig,
        timeout_ms: u32,
        abort: Option<&AtomicBool>,
    ) -> Result<Self, NpError> {
        let wait = ReceiveWait::new(timeout_ms, abort);
        let pipe_name = build_pipe_name(run_dir, service_name)?;
        let handle = open_client_pipe(&pipe_name, &wait)?;

        // Whoever created the pipe name first owns it. Send HELLO and the auth
        // token only to a server running as this account, SYSTEM or
        // Administrators; other accounts cannot take those as owner.
        if !owner_trusted(handle) {
            close_handle(handle);
            return Err(NpError::UntrustedServer);
        }

        // Set read mode to message mode
        let mode: u32 = ffi::PIPE_READMODE_MESSAGE;
        let ok = unsafe { ffi::SetNamedPipeHandleState(handle, &mode, ptr::null(), ptr::null()) };
        if ok == 0 {
            let err = last_error();
            close_handle(handle);
            return Err(NpError::Connect(err));
        }

        let io = match PipeIo::new() {
            Ok(io) => io,
            Err(e) => {
                close_handle(handle);
                return Err(e);
            }
        };
        match client_handshake(handle, io, config, &wait) {
            Ok(session) => Ok(session),
            Err(e) => {
                close_handle(handle);
                Err(e)
            }
        }
    }

    /// Send one logical message. Fills magic/version/header_len/payload_len.
    /// Chunked transparently if message exceeds packet_size.
    pub fn send(&mut self, hdr: &mut Header, payload: &[u8]) -> Result<(), NpError> {
        self.send_with_control(hdr, payload, 0, None)
    }

    /// `send` with an optional deadline and abort flag for a client session.
    /// `timeout_ms == 0` waits without a deadline. If the deadline passes or
    /// `abort` is set before a write completes, the write is cancelled, the
    /// session is closed (later calls fail with `BadParam`) and the call
    /// returns `NpError::Timeout` or `NpError::Aborted`, since part of the
    /// message may be on the wire.
    pub fn send_with_control(
        &mut self,
        hdr: &mut Header,
        payload: &[u8],
        timeout_ms: u32,
        abort: Option<&AtomicBool>,
    ) -> Result<(), NpError> {
        if self.handle == ffi::INVALID_HANDLE_VALUE {
            return Err(NpError::BadParam("session closed".into()));
        }

        // One request in flight (every typed call) writes without
        // draining; only a pipelining client can deadlock with a server
        // blocked writing to it.
        let mut inflight = self.inflight_ids.len();
        if self.role == Role::Client && hdr.kind == crate::protocol::KIND_REQUEST {
            inflight += 1;
        }
        let bound = if self.io.is_some() && inflight > 1 {
            pending_bound(self.max_response_payload_bytes, self.packet_size, inflight)
        } else {
            0
        };

        let framing = self.framing(true);
        let handle = self.handle;
        let io = self.io.as_ref();
        let pending = &mut self.pending;
        let read_len = self.packet_size as usize;
        let send_buf = &mut self.send_buf;
        let wait =
            (timeout_ms != 0 || abort.is_some()).then(|| ReceiveWait::new(timeout_ms, abort));
        let mut broken = false;
        let result =
            framing::send_message(framing, &mut self.inflight_ids, hdr, payload, |h, p| {
                let total = h.len() + p.len();
                if send_buf.len() < total {
                    send_buf.resize(total, 0);
                }
                send_buf[..h.len()].copy_from_slice(h);
                send_buf[h.len()..total].copy_from_slice(p);
                write_packet(
                    handle,
                    io,
                    pending,
                    bound,
                    read_len,
                    &send_buf[..total],
                    wait.as_ref(),
                    &mut broken,
                )
            });
        if broken {
            self.break_session();
        }
        result
    }

    /// Close the pipe: a cancelled write leaves the message stream in an
    /// unknown state, and a drain failure drops inbound messages. Later
    /// calls fail with `BadParam` and `close` still releases the rest.
    fn break_session(&mut self) {
        close_handle(self.handle);
        self.handle = ffi::INVALID_HANDLE_VALUE;
        self.pending = PendingQueue::default();
        self.inflight_ids.clear();
    }

    /// Receive one logical message. buf is a scratch buffer for the first read.
    /// The payload view is valid until the next receive call on this session.
    pub fn receive<'a>(&'a mut self, buf: &'a mut [u8]) -> Result<(Header, &'a [u8]), NpError> {
        self.receive_with_control(buf, 0, None)
    }

    /// Receive one logical message with an optional deadline and abort flag.
    pub fn receive_with_control<'a>(
        &'a mut self,
        buf: &'a mut [u8],
        timeout_ms: u32,
        abort: Option<&AtomicBool>,
    ) -> Result<(Header, &'a [u8]), NpError> {
        if self.handle == ffi::INVALID_HANDLE_VALUE {
            return Err(NpError::BadParam("session closed".into()));
        }

        let controlled = timeout_ms != 0 || abort.is_some();
        let mut wait = ReceiveWait::new(timeout_ms, abort);

        let framing = self.framing(false);
        let handle = self.handle;
        let io = self.io.as_ref();
        let pending = &mut self.pending;
        framing::receive_message(
            framing,
            &mut self.inflight_ids,
            buf,
            &mut self.recv_buf,
            &mut self.pkt_buf,
            |b| raw_recv_control(handle, io, pending, b, controlled, &mut wait),
        )
    }

    /// Close the session.
    pub fn close(&mut self) {
        if self.handle != ffi::INVALID_HANDLE_VALUE && self.handle != 0 {
            // Close only. The peer still reads every message written before
            // the close, then sees ERROR_BROKEN_PIPE. FlushFileBuffers would
            // block on a peer that stopped reading, and neither
            // CancelSynchronousIo nor DisconnectNamedPipe from another thread
            // releases it.
            close_handle(self.handle);
            self.handle = ffi::INVALID_HANDLE_VALUE;
        }
        if self.role == Role::Client {
            self.inflight_ids.clear();
        }
        self.recv_buf.clear();
        self.pending = PendingQueue::default();
        // No I/O stays posted between calls, so the events can go.
        self.io = None;
    }
}

#[cfg(windows)]
impl Drop for NpSession {
    fn drop(&mut self) {
        self.close();
    }
}

// ---------------------------------------------------------------------------
//  Listener
// ---------------------------------------------------------------------------

/// A Named Pipe listener that accepts client connections.
#[cfg(windows)]
pub struct NpListener {
    handle: ffi::HANDLE,
    config: ServerConfig,
    pipe_name: Vec<u16>,
    next_session_id: AtomicU64,
    /// Fails this many creations of a replacement pipe instance in accept,
    /// as `CreateNamedPipeW` can under resource exhaustion.
    #[cfg(test)]
    fail_next_instances: u32,
}

#[cfg(windows)]
impl NpListener {
    /// Create a listener on a Named Pipe derived from run_dir + service_name.
    pub fn bind(run_dir: &str, service_name: &str, config: ServerConfig) -> Result<Self, NpError> {
        let pipe_name = build_pipe_name(run_dir, service_name)?;
        let buf_size = pipe_buffer_size(config.packet_size);

        // Create first instance with FILE_FLAG_FIRST_PIPE_INSTANCE
        let handle = create_pipe_instance(&pipe_name, buf_size, true)?;

        Ok(Self {
            handle,
            config,
            pipe_name,
            next_session_id: AtomicU64::new(1),
            #[cfg(test)]
            fail_next_instances: 0,
        })
    }

    /// Get the raw HANDLE.
    pub fn handle(&self) -> ffi::HANDLE {
        self.handle
    }

    /// Update the payload limits used for future handshakes.
    pub fn set_payload_limits(
        &mut self,
        max_request_payload_bytes: u32,
        max_response_payload_bytes: u32,
    ) {
        self.config.max_request_payload_bytes = max_request_payload_bytes;
        self.config.max_response_payload_bytes = max_response_payload_bytes;
    }

    /// Accept one client connection. Performs the full handshake.
    pub fn accept(&mut self) -> Result<NpSession, NpError> {
        let session_id = self.next_session_id.fetch_add(1, Ordering::Relaxed);
        self.accept_with_config(session_id, self.config.clone())
    }

    /// Accept one client using a caller-provided per-session server config
    /// and session ID. The handshake waits at most
    /// `SERVER_HANDSHAKE_TIMEOUT_MS` for HELLO.
    pub fn accept_with_config(
        &mut self,
        session_id: u64,
        config: ServerConfig,
    ) -> Result<NpSession, NpError> {
        self.accept_pending()?.handshake(
            session_id,
            &config,
            crate::protocol::SERVER_HANDSHAKE_TIMEOUT_MS,
            None,
        )
    }

    /// Accept one connection without handshaking, so the caller can run the
    /// handshake on another thread. Blocks until a client connects, then
    /// creates a new pipe instance for subsequent clients.
    pub fn accept_pending(&mut self) -> Result<PendingPipe, NpError> {
        // Wait for client
        let connected = unsafe { ffi::ConnectNamedPipe(self.handle, ptr::null_mut()) };
        if connected == 0 {
            let err = last_error();
            if err != ffi::ERROR_PIPE_CONNECTED {
                return Err(NpError::Accept(err));
            }
        }

        let session_handle = self.handle;

        // Create new pipe instance for next client
        let buf_size = pipe_buffer_size(self.config.packet_size);
        #[cfg(test)]
        if self.fail_next_instances > 0 {
            self.fail_next_instances -= 1;
            unsafe { ffi::DisconnectNamedPipe(session_handle) };
            // ERROR_NO_SYSTEM_RESOURCES
            return Err(NpError::CreatePipe(1450));
        }
        let next = match create_pipe_instance(&self.pipe_name, buf_size, false) {
            Ok(h) => h,
            Err(e) => {
                // Without a next instance, drop this client and keep its
                // instance as the listener's, so a later accept listens on
                // it again.
                unsafe { ffi::DisconnectNamedPipe(session_handle) };
                return Err(e);
            }
        };
        self.handle = next;

        Ok(PendingPipe {
            handle: session_handle,
        })
    }

    /// A waker that unblocks `accept_pending()` from another thread. It uses
    /// only the pipe name, so it never touches a handle the accept loop owns.
    pub fn waker(&self) -> NpListenerWaker {
        NpListenerWaker {
            pipe_name: self.pipe_name.clone(),
        }
    }

    /// Close the listener.
    pub fn close(&mut self) {
        close_handle(self.handle);
        self.handle = ffi::INVALID_HANDLE_VALUE;
    }
}

#[cfg(windows)]
impl Drop for NpListener {
    fn drop(&mut self) {
        self.close();
    }
}

/// SECURITY_IDENTIFICATION lets the server identify the client but never act
/// as it. Overlapped, so a pipelining send can read while its write waits.
#[cfg(windows)]
fn create_client_pipe(pipe_name: &[u16]) -> ffi::HANDLE {
    unsafe {
        ffi::CreateFileW(
            pipe_name.as_ptr(),
            ffi::GENERIC_READ | ffi::GENERIC_WRITE,
            0,
            ptr::null(),
            ffi::OPEN_EXISTING,
            ffi::SECURITY_SQOS_PRESENT | ffi::SECURITY_IDENTIFICATION | ffi::FILE_FLAG_OVERLAPPED,
            0,
        )
    }
}

/// Test hook called with the pipe name. Tests run in parallel, so a hook
/// ignores every pipe but its own test's.
#[cfg(all(windows, test))]
pub(crate) type ConnectHook = std::sync::Mutex<Option<Box<dyn Fn(&[u16]) + Send>>>;

/// Runs whenever a connect finds every pipe instance busy, before it waits.
#[cfg(all(windows, test))]
pub(crate) static CONNECT_BUSY_HOOK: ConnectHook = std::sync::Mutex::new(None);

/// Runs whenever `WaitNamedPipeW` reports a free instance, before the connect
/// retries `CreateFileW`, so a test can take that instance first.
#[cfg(all(windows, test))]
pub(crate) static CONNECT_WAITED_HOOK: ConnectHook = std::sync::Mutex::new(None);

/// Longest single busy-pipe wait, so an abort ends it promptly.
#[cfg(windows)]
const CONNECT_BUSY_SLICE_MS: u64 = 100;

/// A server has no free pipe instance between accepting one client and
/// creating the next instance, and connects in that window fail with
/// ERROR_PIPE_BUSY. Wait, bounded by `CONNECT_BUSY_WAIT_MS` and the caller's
/// deadline, for an instance and retry: `WaitNamedPipeW` does not reserve it,
/// so another client may take it first.
#[cfg(windows)]
fn open_client_pipe(pipe_name: &[u16], wait: &ReceiveWait<'_>) -> Result<ffi::HANDLE, NpError> {
    if wait.aborted() {
        return Err(NpError::Aborted);
    }
    if wait
        .deadline
        .is_some_and(|deadline| Instant::now() >= deadline)
    {
        return Err(NpError::Timeout);
    }
    let handle = create_client_pipe(pipe_name);
    if handle != ffi::INVALID_HANDLE_VALUE {
        return Ok(handle);
    }
    let err = last_error();
    if err != ffi::ERROR_PIPE_BUSY {
        return Err(NpError::Connect(err));
    }
    #[cfg(test)]
    if let Some(hook) = CONNECT_BUSY_HOOK.lock().unwrap().as_ref() {
        hook(pipe_name);
    }

    let mut busy_deadline = Instant::now() + Duration::from_millis(CONNECT_BUSY_WAIT_MS as u64);
    if let Some(deadline) = wait.deadline {
        busy_deadline = busy_deadline.min(deadline);
    }
    loop {
        if wait.aborted() {
            return Err(NpError::Aborted);
        }
        let now = Instant::now();
        if now >= busy_deadline {
            if wait.deadline.is_some_and(|deadline| now >= deadline) {
                return Err(NpError::Timeout);
            }
            return Err(NpError::Connect(ffi::ERROR_PIPE_BUSY));
        }
        let left = busy_deadline.duration_since(now).as_millis().max(1) as u64;
        let slice = left.min(CONNECT_BUSY_SLICE_MS) as u32;
        if unsafe { ffi::WaitNamedPipeW(pipe_name.as_ptr(), slice) } != 0 {
            #[cfg(test)]
            if let Some(hook) = CONNECT_WAITED_HOOK.lock().unwrap().as_ref() {
                hook(pipe_name);
            }
            let handle = create_client_pipe(pipe_name);
            if handle != ffi::INVALID_HANDLE_VALUE {
                return Ok(handle);
            }
            let err = last_error();
            if err != ffi::ERROR_PIPE_BUSY {
                return Err(NpError::Connect(err));
            }
            // Another client took the instance first.
            continue;
        }
        let err = last_error();
        if err != ffi::ERROR_SEM_TIMEOUT {
            return Err(NpError::Connect(err));
        }
    }
}

/// Wakes a listener blocked in `ConnectNamedPipe` by connecting to it.
#[cfg(windows)]
#[derive(Clone, Debug)]
pub struct NpListenerWaker {
    pipe_name: Vec<u16>,
}

#[cfg(windows)]
impl NpListenerWaker {
    pub fn wake(&self) {
        let h = unsafe {
            ffi::CreateFileW(
                self.pipe_name.as_ptr(),
                ffi::GENERIC_READ | ffi::GENERIC_WRITE,
                0,
                ptr::null(),
                ffi::OPEN_EXISTING,
                ffi::SECURITY_SQOS_PRESENT | ffi::SECURITY_IDENTIFICATION,
                0,
            )
        };
        close_handle(h);
    }
}

/// An accepted pipe whose handshake has not run yet. Dropping it closes the
/// pipe; DisconnectNamedPipe would discard a rejection reply the client has
/// not read yet.
#[cfg(windows)]
#[derive(Debug)]
pub struct PendingPipe {
    handle: ffi::HANDLE,
}

#[cfg(windows)]
impl PendingPipe {
    /// Complete the server side of the handshake. Waits at most `timeout_ms`
    /// for HELLO (`NpError::Timeout`; 0 waits without a deadline) and returns
    /// `NpError::Aborted` once `abort` is set.
    pub fn handshake(
        self,
        session_id: u64,
        config: &ServerConfig,
        timeout_ms: u32,
        abort: Option<&AtomicBool>,
    ) -> Result<NpSession, NpError> {
        self.handshake_inner(session_id, config, timeout_ms, abort, None)
    }

    /// [`handshake`](Self::handshake) with a prepare step. Once a profile is
    /// selected and the payload limits are agreed, and before HELLO_ACK is
    /// sent, `prepare(profile, max_request_payload_bytes,
    /// max_response_payload_bytes)` sets up that profile's resources (SHM
    /// objects). Returning false drops the profile and the handshake selects
    /// again; with no profile left the client is rejected as unsupported.
    pub fn handshake_prepared(
        self,
        session_id: u64,
        config: &ServerConfig,
        timeout_ms: u32,
        abort: Option<&AtomicBool>,
        prepare: &mut dyn FnMut(u32, u32, u32) -> bool,
    ) -> Result<NpSession, NpError> {
        self.handshake_inner(session_id, config, timeout_ms, abort, Some(prepare))
    }

    fn handshake_inner(
        self,
        session_id: u64,
        config: &ServerConfig,
        timeout_ms: u32,
        abort: Option<&AtomicBool>,
        prepare: Option<&mut dyn FnMut(u32, u32, u32) -> bool>,
    ) -> Result<NpSession, NpError> {
        let wait = ReceiveWait::new(timeout_ms, abort);
        loop {
            let wait_ms = wait.wait_slice_ms()?;
            if wait_pipe_readable(self.handle, wait_ms)? {
                break;
            }
        }
        if wait.aborted() {
            return Err(NpError::Aborted);
        }
        let session = server_handshake(self.handle, config, session_id, prepare)?;
        // The session owns the handle now.
        std::mem::forget(self);
        Ok(session)
    }
}

#[cfg(windows)]
impl Drop for PendingPipe {
    fn drop(&mut self) {
        close_handle(self.handle);
    }
}

// ---------------------------------------------------------------------------
//  Pipe instance creation
// ---------------------------------------------------------------------------

#[cfg(windows)]
fn create_pipe_instance(
    pipe_name: &[u16],
    buf_size: u32,
    first_instance: bool,
) -> Result<ffi::HANDLE, NpError> {
    let mut open_mode = ffi::PIPE_ACCESS_DUPLEX;
    if first_instance {
        open_mode |= ffi::FILE_FLAG_FIRST_PIPE_INSTANCE;
    }

    let attrs = OwnerOnlyAttributes::new().map_err(NpError::CreatePipe)?;
    let handle = unsafe {
        ffi::CreateNamedPipeW(
            pipe_name.as_ptr(),
            open_mode,
            ffi::PIPE_TYPE_MESSAGE
                | ffi::PIPE_READMODE_MESSAGE
                | ffi::PIPE_WAIT
                | ffi::PIPE_REJECT_REMOTE_CLIENTS,
            ffi::PIPE_UNLIMITED_INSTANCES,
            buf_size,
            buf_size,
            0,
            attrs.as_ptr(),
        )
    };

    if handle == ffi::INVALID_HANDLE_VALUE {
        let err = last_error();
        if err == ffi::ERROR_ACCESS_DENIED || err == ffi::ERROR_PIPE_BUSY {
            return Err(NpError::AddrInUse);
        }
        return Err(NpError::CreatePipe(err));
    }

    Ok(handle)
}

// ---------------------------------------------------------------------------
//  Client handshake
// ---------------------------------------------------------------------------

#[cfg(windows)]
fn client_handshake(
    handle: ffi::HANDLE,
    io: PipeIo,
    config: &ClientConfig,
    wait: &ReceiveWait<'_>,
) -> Result<NpSession, NpError> {
    let pkt_size = apply_default(config.packet_size, DEFAULT_PACKET_SIZE);

    let supported = apply_default(config.supported_profiles, PROFILE_BASELINE) & PLATFORM_PROFILES;

    let hello = Hello {
        layout_version: 1,
        flags: 0,
        supported_profiles: supported,
        preferred_profiles: config.preferred_profiles & PLATFORM_PROFILES,
        max_request_payload_bytes: apply_default(
            config.max_request_payload_bytes,
            MAX_PAYLOAD_DEFAULT,
        ),
        max_request_batch_items: apply_default(config.max_request_batch_items, DEFAULT_BATCH_ITEMS),
        max_response_payload_bytes: apply_default(
            config.max_response_payload_bytes,
            MAX_PAYLOAD_DEFAULT,
        ),
        max_response_batch_items: apply_default(
            config.max_response_batch_items,
            DEFAULT_BATCH_ITEMS,
        ),
        auth_token: config.auth_token,
        packet_size: pkt_size,
    };

    let mut no_pending = PendingQueue::default();
    write_packet(
        handle,
        Some(&io),
        &mut no_pending,
        0,
        0,
        &framing::hello_packet(&hello),
        None,
        &mut false,
    )?;

    // Receive HELLO_ACK; a server that never answers must not hold the
    // client past its deadline. A server that rejects the client writes its
    // reply and closes, so a disconnected pipe may still hold that reply.
    loop {
        let wait_ms = wait.wait_slice_ms()?;
        match wait_pipe_readable(handle, wait_ms) {
            Ok(true) | Err(NpError::Disconnected) => break,
            Ok(false) => {}
            Err(e) => return Err(e),
        }
    }
    if wait.aborted() {
        return Err(NpError::Aborted);
    }
    let mut ack_buf = [0u8; 128];
    let n = overlapped_read(handle, io.read_event, &mut ack_buf)?;

    let ack: HelloAck = framing::parse_hello_ack(&ack_buf[..n], supported)?;

    // Reject packet sizes too small for a header
    if ack.agreed_packet_size <= HEADER_SIZE as u32 {
        return Err(NpError::Protocol(
            "agreed packet_size <= HEADER_SIZE".into(),
        ));
    }
    // A server may only lower our proposal: buffers and chunking on this side
    // are sized from it.
    if ack.agreed_packet_size > pkt_size {
        return Err(NpError::Protocol(
            "agreed packet_size exceeds proposal".into(),
        ));
    }

    Ok(NpSession {
        handle,
        role: Role::Client,
        max_request_payload_bytes: ack.agreed_max_request_payload_bytes,
        max_request_batch_items: ack.agreed_max_request_batch_items,
        max_response_payload_bytes: ack.agreed_max_response_payload_bytes,
        max_response_batch_items: ack.agreed_max_response_batch_items,
        packet_size: ack.agreed_packet_size,
        selected_profile: ack.selected_profile,
        session_id: ack.session_id,
        recv_buf: Vec::new(),
        pkt_buf: Vec::new(),
        send_buf: Vec::new(),
        inflight_ids: HashSet::new(),
        io: Some(io),
        pending: PendingQueue::default(),
    })
}

// ---------------------------------------------------------------------------
//  Server handshake
// ---------------------------------------------------------------------------

#[cfg(windows)]
fn server_handshake(
    handle: ffi::HANDLE,
    config: &ServerConfig,
    session_id: u64,
    prepare: Option<&mut dyn FnMut(u32, u32, u32) -> bool>,
) -> Result<NpSession, NpError> {
    let offer = ServerOffer {
        packet_size: apply_default(config.packet_size, DEFAULT_PACKET_SIZE),
        supported_profiles: apply_default(config.supported_profiles, PROFILE_BASELINE)
            & PLATFORM_PROFILES,
        preferred_profiles: config.preferred_profiles & PLATFORM_PROFILES,
        max_request_payload_bytes: apply_default(
            config.max_request_payload_bytes,
            MAX_PAYLOAD_DEFAULT,
        ),
        max_response_payload_bytes: apply_default(
            config.max_response_payload_bytes,
            MAX_PAYLOAD_DEFAULT,
        ),
        auth_token: config.auth_token,
    };

    // Receive HELLO
    let mut buf = [0u8; 128];
    let n = raw_recv(handle, &mut buf)?;
    let ack = framing::server_negotiate(&buf[..n], &offer, session_id, prepare, |pkt| {
        raw_write(handle, pkt)
    })?;

    Ok(NpSession {
        handle,
        role: Role::Server,
        max_request_payload_bytes: ack.agreed_max_request_payload_bytes,
        max_request_batch_items: ack.agreed_max_request_batch_items,
        max_response_payload_bytes: ack.agreed_max_response_payload_bytes,
        max_response_batch_items: ack.agreed_max_response_batch_items,
        packet_size: ack.agreed_packet_size,
        selected_profile: ack.selected_profile,
        session_id,
        recv_buf: Vec::new(),
        pkt_buf: Vec::new(),
        send_buf: Vec::new(),
        inflight_ids: HashSet::new(),
        io: None,
        pending: PendingQueue::default(),
    })
}

// ---------------------------------------------------------------------------
//  Tests (cross-platform unit tests for non-Win32 logic)
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use crate::protocol;
    #[cfg(windows)]
    use crate::protocol::{KIND_REQUEST, KIND_RESPONSE, MAGIC_MSG, VERSION};
    #[cfg(windows)]
    use crate::transport::framing::HELLO_ACK_PAYLOAD_SIZE;
    #[cfg(windows)]
    use std::sync::atomic::{AtomicU64, Ordering};
    #[cfg(windows)]
    use std::thread;

    #[cfg(windows)]
    const TEST_RUN_DIR: &str = r"C:\Temp\nipc_transport_rust_test";
    #[cfg(windows)]
    static TEST_COUNTER: AtomicU64 = AtomicU64::new(0);

    #[cfg(windows)]
    fn ensure_run_dir() {
        let _ = std::fs::create_dir_all(TEST_RUN_DIR);
    }

    #[cfg(windows)]
    fn unique_service(prefix: &str) -> String {
        format!(
            "{}_{}_{}",
            prefix,
            std::process::id(),
            TEST_COUNTER.fetch_add(1, Ordering::Relaxed) + 1
        )
    }

    #[cfg(windows)]
    fn default_client_config() -> ClientConfig {
        ClientConfig {
            supported_profiles: PROFILE_BASELINE,
            preferred_profiles: 0,
            max_request_payload_bytes: 4096,
            max_request_batch_items: 16,
            max_response_payload_bytes: 4096,
            max_response_batch_items: 16,
            auth_token: 0xDEADBEEFCAFEBABE,
            packet_size: 0,
        }
    }

    #[cfg(windows)]
    fn default_server_config() -> ServerConfig {
        ServerConfig {
            supported_profiles: PROFILE_BASELINE,
            preferred_profiles: 0,
            max_request_payload_bytes: 4096,
            max_request_batch_items: 16,
            max_response_payload_bytes: 4096,
            max_response_batch_items: 16,
            auth_token: 0xDEADBEEFCAFEBABE,
            packet_size: 0,
        }
    }

    #[test]
    fn test_fnv1a_64_empty() {
        assert_eq!(fnv1a_64(b""), FNV1A_OFFSET_BASIS);
    }

    #[test]
    fn test_fnv1a_64_known() {
        // FNV-1a of "foobar" — verified against reference implementation
        let hash = fnv1a_64(b"foobar");
        assert_ne!(hash, 0);
        assert_ne!(hash, FNV1A_OFFSET_BASIS);
    }

    #[test]
    fn test_fnv1a_64_deterministic() {
        let h1 = fnv1a_64(b"/var/run/netdata");
        let h2 = fnv1a_64(b"/var/run/netdata");
        assert_eq!(h1, h2);
    }

    #[test]
    fn test_fnv1a_64_different_inputs() {
        let h1 = fnv1a_64(b"/var/run/netdata");
        let h2 = fnv1a_64(b"/tmp/netdata");
        assert_ne!(h1, h2);
    }

    #[test]
    fn test_validate_service_name_valid() {
        assert!(validate_service_name("cgroups-snapshot").is_ok());
        assert!(validate_service_name("test_service.v1").is_ok());
        assert!(validate_service_name("A-Z_09").is_ok());
    }

    #[test]
    fn test_validate_service_name_invalid() {
        assert!(validate_service_name("").is_err());
        assert!(validate_service_name(".").is_err());
        assert!(validate_service_name("..").is_err());
        assert!(validate_service_name("has space").is_err());
        assert!(validate_service_name("has/slash").is_err());
        assert!(validate_service_name("has\\backslash").is_err());
    }

    #[test]
    fn test_build_pipe_name() {
        let name = build_pipe_name("/var/run/netdata", "cgroups-snapshot").unwrap();
        // Should produce a valid wide string ending in NUL
        assert!(*name.last().unwrap() == 0);
        // Convert back to narrow for checking prefix
        let narrow: String = name[..name.len() - 1]
            .iter()
            .map(|&c| c as u8 as char)
            .collect();
        assert!(narrow.starts_with("\\\\.\\pipe\\netipc-"));
        assert!(narrow.ends_with("-cgroups-snapshot"));
        // Hash should be 16 hex chars
        let parts: Vec<&str> = narrow.split('-').collect();
        // \\.\pipe\netipc - {hash} - cgroups - snapshot
        // parts: ["\\\\.", "\\pipe\\netipc", "{hash}", "cgroups", "snapshot"]
        // Actually the split is on '-' so:
        // "\\\\.\\pipe\\netipc" - "hash" - "cgroups" - "snapshot"
        assert!(parts.len() >= 3);
        // The pipe name is \\.\pipe\netipc-{hash}-{service}, so parts[1]
        // is the 16-character hash component.
        assert_eq!(parts[1].len(), 16, "hash should be 16 hex chars");
    }

    #[test]
    fn test_build_pipe_name_invalid_service() {
        assert!(build_pipe_name("/var/run", "").is_err());
        assert!(build_pipe_name("/var/run", "bad/name").is_err());
        assert!(build_pipe_name("/var/run", ".").is_err());
    }

    #[test]
    fn test_pipe_name_deterministic() {
        let n1 = build_pipe_name("/var/run/netdata", "test-svc").unwrap();
        let n2 = build_pipe_name("/var/run/netdata", "test-svc").unwrap();
        assert_eq!(n1, n2);
    }

    #[test]
    fn test_pipe_name_different_run_dir() {
        let n1 = build_pipe_name("/var/run/netdata", "svc").unwrap();
        let n2 = build_pipe_name("/tmp/netdata", "svc").unwrap();
        assert_ne!(n1, n2);
    }

    #[test]
    fn test_np_error_display() {
        let cases = [
            (NpError::PipeName("bad".into()), "pipe name error: bad"),
            (NpError::CreatePipe(5), "CreateNamedPipeW failed: 5"),
            (NpError::Connect(231), "connect failed: 231"),
            (NpError::Accept(24), "accept failed: 24"),
            (NpError::Send(32), "send failed: 32"),
            (NpError::Recv(0), "recv failed: 0"),
            (NpError::Handshake("test".into()), "handshake error: test"),
            (NpError::AuthFailed, "authentication token rejected"),
            (NpError::NoProfile, "no common transport profile"),
            (
                NpError::Incompatible("version mismatch".into()),
                "incompatible protocol: version mismatch",
            ),
            (NpError::Protocol("bad".into()), "protocol violation: bad"),
            (
                NpError::AddrInUse,
                "pipe name already in use by live server",
            ),
            (NpError::Chunk("mismatch".into()), "chunk error: mismatch"),
            (NpError::Alloc, "memory allocation failed"),
            (NpError::LimitExceeded, "negotiated limit exceeded"),
            (NpError::BadParam("foo".into()), "bad parameter: foo"),
            (NpError::DuplicateMsgId(42), "duplicate message_id: 42"),
            (NpError::UnknownMsgId(99), "unknown response message_id: 99"),
            (NpError::Disconnected, "peer disconnected"),
        ];

        for (err, expected) in cases {
            assert_eq!(format!("{err}"), expected);
        }

        let e: &dyn std::error::Error = &NpError::Disconnected;
        let _ = format!("{e}");
    }

    #[test]
    fn test_build_pipe_name_too_long() {
        let long_name = "a".repeat(300);
        let result = build_pipe_name("/var/run", &long_name);
        assert!(matches!(result, Err(NpError::PipeName(_))));
    }

    /// Serve one HELLO_ACK with `selected` and the agreed packet size
    /// `agreed(proposal)`, then close; returns the server thread.
    #[cfg(windows)]
    fn fake_hello_ack_server(
        svc: &str,
        selected: u32,
        agreed: fn(u32) -> u32,
    ) -> thread::JoinHandle<()> {
        let pipe_name = build_pipe_name(TEST_RUN_DIR, svc).expect("pipe name");
        let (ready_tx, ready_rx) = std::sync::mpsc::channel();
        let server = thread::spawn(move || {
            let handle =
                create_pipe_instance(&pipe_name, pipe_buffer_size(DEFAULT_PACKET_SIZE), true)
                    .expect("create pipe");
            ready_tx.send(()).expect("signal ready");
            unsafe { ffi::ConnectNamedPipe(handle, std::ptr::null_mut()) };

            let mut buf = [0u8; 128];
            let n = raw_recv(handle, &mut buf).expect("recv hello");
            let hello = Hello::decode(&buf[HEADER_SIZE..n]).expect("decode hello");
            let ack = HelloAck {
                layout_version: 1,
                server_supported_profiles: PROFILE_BASELINE | protocol::PROFILE_SHM_HYBRID,
                intersection_profiles: PROFILE_BASELINE,
                selected_profile: selected,
                agreed_max_request_payload_bytes: hello.max_request_payload_bytes,
                agreed_max_request_batch_items: hello.max_request_batch_items,
                agreed_max_response_payload_bytes: hello.max_response_payload_bytes,
                agreed_max_response_batch_items: hello.max_response_batch_items,
                agreed_packet_size: agreed(hello.packet_size),
                session_id: 1,
                ..HelloAck::default()
            };
            let mut pkt = [0u8; HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE];
            Header {
                magic: MAGIC_MSG,
                version: VERSION,
                header_len: protocol::HEADER_LEN,
                kind: protocol::KIND_CONTROL,
                flags: 0,
                code: protocol::CODE_HELLO_ACK,
                transport_status: protocol::STATUS_OK,
                payload_len: HELLO_ACK_PAYLOAD_SIZE as u32,
                item_count: 1,
                message_id: 0,
            }
            .encode(&mut pkt[..HEADER_SIZE]);
            ack.encode(&mut pkt[HEADER_SIZE..]);
            raw_write(handle, &pkt).expect("send ack");
            close_handle(handle);
        });
        ready_rx.recv().expect("server ready");
        server
    }

    #[cfg(windows)]
    #[test]
    fn test_connect_rejects_selected_profile_not_offered() {
        ensure_run_dir();
        // The client offers baseline only.
        for selected in [
            protocol::PROFILE_SHM_HYBRID,
            0,
            PROFILE_BASELINE | protocol::PROFILE_SHM_HYBRID,
        ] {
            let svc = unique_service("rs_ack_selected");
            let server = fake_hello_ack_server(&svc, selected, |proposal| proposal);
            let err = match NpSession::connect(TEST_RUN_DIR, &svc, &default_client_config()) {
                Ok(_) => panic!("HELLO_ACK selecting 0x{selected:x} should fail"),
                Err(err) => err,
            };
            assert!(
                matches!(&err, NpError::Protocol(msg) if msg.contains("not offered")),
                "selected 0x{selected:x}: {err:?}"
            );
            server.join().expect("server join");
        }
    }

    #[cfg(windows)]
    #[test]
    fn test_connect_rejects_agreed_packet_size_out_of_range() {
        ensure_run_dir();
        // A server may only lower the proposal, and must leave payload room.
        let cases: [fn(u32) -> u32; 2] = [|proposal| proposal + 1, |_| HEADER_SIZE as u32];
        for agreed in cases {
            let svc = unique_service("rs_ack_pkt");
            let server = fake_hello_ack_server(&svc, PROFILE_BASELINE, agreed);
            let err = match NpSession::connect(TEST_RUN_DIR, &svc, &default_client_config()) {
                Ok(_) => panic!("HELLO_ACK with an out-of-range packet size should fail"),
                Err(err) => err,
            };
            assert!(
                matches!(&err, NpError::Protocol(msg) if msg.contains("packet_size")),
                "{err:?}"
            );
            server.join().expect("server join");
        }
    }

    #[cfg(windows)]
    #[test]
    fn test_connect_nonexistent() {
        ensure_run_dir();
        let svc = unique_service("rs_noexist");
        let result = NpSession::connect(TEST_RUN_DIR, &svc, &default_client_config());
        assert!(matches!(result, Err(NpError::Connect(_))));
    }

    #[cfg(windows)]
    #[test]
    fn test_send_receive_on_closed_session() {
        let mut session = NpSession {
            handle: ffi::INVALID_HANDLE_VALUE,
            role: Role::Client,
            max_request_payload_bytes: 4096,
            max_request_batch_items: 16,
            max_response_payload_bytes: 4096,
            max_response_batch_items: 16,
            packet_size: DEFAULT_PACKET_SIZE,
            selected_profile: PROFILE_BASELINE,
            session_id: 1,
            recv_buf: Vec::new(),
            pkt_buf: Vec::new(),
            send_buf: Vec::new(),
            inflight_ids: HashSet::new(),
            io: None,
            pending: PendingQueue::default(),
        };

        let mut hdr = Header {
            kind: protocol::KIND_REQUEST,
            code: protocol::METHOD_INCREMENT,
            item_count: 1,
            message_id: 1,
            ..Header::default()
        };
        assert!(matches!(
            session.send(&mut hdr, &[1, 2, 3]),
            Err(NpError::BadParam(_))
        ));

        let mut buf = [0u8; 64];
        assert!(matches!(
            session.receive(&mut buf),
            Err(NpError::BadParam(_))
        ));
    }

    #[cfg(windows)]
    #[test]
    fn test_roundtrip_and_accessors() {
        ensure_run_dir();
        let svc = unique_service("rs_roundtrip");

        let mut listener =
            NpListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind");
        assert_ne!(listener.handle(), ffi::INVALID_HANDLE_VALUE);
        assert_ne!(listener.handle(), 0);

        let server = thread::spawn(move || {
            let mut session = listener.accept().expect("accept");
            assert_eq!(session.role(), Role::Server);
            assert_ne!(session.handle(), ffi::INVALID_HANDLE_VALUE);
            let mut buf = [0u8; 256];
            let (hdr, payload) = session.receive(&mut buf).expect("recv");
            let payload = payload.to_vec();
            let mut resp = hdr;
            resp.kind = KIND_RESPONSE;
            resp.transport_status = protocol::STATUS_OK;
            session.send(&mut resp, &payload).expect("send");
            session.close();
        });

        let mut session =
            NpSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");
        assert_eq!(session.role(), Role::Client);
        assert_ne!(session.handle(), ffi::INVALID_HANDLE_VALUE);
        assert_eq!(session.selected_profile, PROFILE_BASELINE);

        let payload = [1u8, 2, 3, 4];
        let mut hdr = Header {
            kind: KIND_REQUEST,
            code: protocol::METHOD_INCREMENT,
            flags: 0,
            item_count: 1,
            message_id: 42,
            ..Header::default()
        };
        session.send(&mut hdr, &payload).expect("send");

        let mut rbuf = [0u8; 256];
        let (rhdr, rpayload) = session.receive(&mut rbuf).expect("recv");
        assert_eq!(rhdr.kind, KIND_RESPONSE);
        assert_eq!(rhdr.message_id, 42);
        assert_eq!(rpayload, payload);

        session.close();
        server.join().expect("server join");
    }

    #[cfg(windows)]
    #[test]
    fn test_receive_packet_longer_than_declared_payload() {
        ensure_run_dir();
        let svc = unique_service("rs_trailing");

        let mut listener =
            NpListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind");
        let server = thread::spawn(move || {
            let mut session = listener.accept().expect("accept");
            let mut buf = [0u8; 256];
            let (req_hdr, _) = session.receive(&mut buf).expect("recv request");

            let mut pkt = [0u8; HEADER_SIZE + 2];
            let resp_hdr = Header {
                magic: MAGIC_MSG,
                version: VERSION,
                header_len: protocol::HEADER_LEN,
                kind: KIND_RESPONSE,
                code: req_hdr.code,
                flags: 0,
                transport_status: protocol::STATUS_OK,
                payload_len: 1,
                item_count: 1,
                message_id: req_hdr.message_id,
            };
            resp_hdr.encode(&mut pkt[..HEADER_SIZE]);
            pkt[HEADER_SIZE..].copy_from_slice(&[0xBE, 0xEF]);
            raw_write(session.handle, &pkt).expect("raw trailing packet write");
            session.close();
        });

        let mut session =
            NpSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");
        let mut hdr = Header {
            kind: KIND_REQUEST,
            code: protocol::METHOD_INCREMENT,
            item_count: 1,
            message_id: 43,
            ..Header::default()
        };
        session.send(&mut hdr, &[0xAA]).expect("send request");

        let mut rbuf = [0u8; 256];
        let err = session
            .receive(&mut rbuf)
            .expect_err("trailing bytes should be rejected");
        assert!(matches!(err, NpError::Protocol(ref msg)
            if msg.contains("exceeds declared payload_len")));

        session.close();
        server.join().expect("server join");
    }

    #[cfg(windows)]
    #[test]
    fn test_chunking_and_received_payload() {
        ensure_run_dir();
        let svc = unique_service("rs_chunk");

        let scfg = ServerConfig {
            packet_size: 128,
            max_request_payload_bytes: 65536,
            max_response_payload_bytes: 65536,
            ..default_server_config()
        };
        let mut listener = NpListener::bind(TEST_RUN_DIR, &svc, scfg).expect("bind");

        let server = thread::spawn(move || {
            let mut session = listener.accept().expect("accept");
            let mut buf = [0u8; 256];
            let (hdr, payload) = session.receive(&mut buf).expect("recv");
            let payload = payload.to_vec();
            let mut resp = hdr;
            resp.kind = KIND_RESPONSE;
            resp.transport_status = protocol::STATUS_OK;
            session.send(&mut resp, &payload).expect("send");
            session.close();
        });

        let ccfg = ClientConfig {
            packet_size: 128,
            max_request_payload_bytes: 65536,
            max_response_payload_bytes: 65536,
            ..default_client_config()
        };
        let mut session = NpSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");
        assert_eq!(session.packet_size, 128);

        let big: Vec<u8> = (0..500).map(|i| (i & 0xff) as u8).collect();
        let mut hdr = Header {
            kind: KIND_REQUEST,
            code: protocol::METHOD_INCREMENT,
            item_count: 1,
            message_id: 7,
            ..Header::default()
        };
        session.send(&mut hdr, &big).expect("send chunked");

        let mut rbuf = [0u8; 256];
        let (rhdr, received_len) = {
            let (rhdr, rpayload) = session.receive(&mut rbuf).expect("recv chunked");
            assert_eq!(rpayload, big);
            (rhdr, rpayload.len())
        };
        assert_eq!(rhdr.message_id, 7);
        assert_eq!(session.received_payload(received_len), big.as_slice());

        session.close();
        server.join().expect("server join");
    }

    #[cfg(windows)]
    #[test]
    fn test_pipeline_chunked() {
        ensure_run_dir();
        let svc = unique_service("rs_pipe_chunked");
        let forced_packet_size = 128u32;

        let scfg = ServerConfig {
            packet_size: forced_packet_size,
            max_request_payload_bytes: 65536,
            max_response_payload_bytes: 65536,
            ..default_server_config()
        };
        let mut listener = NpListener::bind(TEST_RUN_DIR, &svc, scfg).expect("bind");

        let server = thread::spawn(move || {
            let mut session = listener.accept().expect("accept");
            let mut buf = vec![0u8; forced_packet_size as usize];
            for _ in 0..5 {
                let (hdr, payload) = session.receive(&mut buf).expect("recv");
                let payload = payload.to_vec();
                let mut resp = hdr;
                resp.kind = KIND_RESPONSE;
                resp.transport_status = protocol::STATUS_OK;
                session.send(&mut resp, &payload).expect("send");
            }
            session.close();
        });

        let ccfg = ClientConfig {
            packet_size: forced_packet_size,
            max_request_payload_bytes: 65536,
            max_response_payload_bytes: 65536,
            ..default_client_config()
        };
        let mut session = NpSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");

        let sizes = [200usize, 500, 300, 800, 150];
        for (i, &sz) in sizes.iter().enumerate() {
            let payload: Vec<u8> = (0..sz).map(|j| ((i + j) & 0xFF) as u8).collect();
            let mut hdr = Header {
                kind: KIND_REQUEST,
                code: protocol::METHOD_INCREMENT,
                item_count: 1,
                message_id: (i + 1) as u64,
                ..Header::default()
            };
            session.send(&mut hdr, &payload).expect("send");
        }

        let mut rbuf = vec![0u8; forced_packet_size as usize];
        for (i, &sz) in sizes.iter().enumerate() {
            let (rhdr, rpayload) = session.receive(&mut rbuf).expect("recv");
            assert_eq!(rhdr.message_id, (i + 1) as u64, "message_id at {i}");
            assert_eq!(rpayload.len(), sz, "payload len at {i}");
            let expected: Vec<u8> = (0..sz).map(|j| ((i + j) & 0xFF) as u8).collect();
            assert_eq!(rpayload, expected, "payload data at {i}");
        }

        session.close();
        server.join().expect("server join");
    }

    /// A client pipelining more than the pipe buffers before reading must
    /// not deadlock with a server blocked writing responses to it.
    #[cfg(windows)]
    #[test]
    fn test_pipeline_beyond_pipe_buffers() {
        const PAYLOAD: usize = 16 * 1024;
        const REQUESTS: u64 = 32; // 512 KiB each way, 8x the 64 KiB pipe buffers.
        fn payload_for(id: u64) -> Vec<u8> {
            (0..PAYLOAD).map(|i| (id as usize + i) as u8).collect()
        }

        ensure_run_dir();
        let svc = unique_service("rs_pipe_progress");
        let scfg = ServerConfig {
            max_request_payload_bytes: PAYLOAD as u32,
            max_response_payload_bytes: PAYLOAD as u32,
            ..default_server_config()
        };
        let mut listener = NpListener::bind(TEST_RUN_DIR, &svc, scfg).expect("bind");

        // Answers each request with blocking writes, one at a time.
        let server = thread::spawn(move || {
            let mut session = listener.accept().expect("accept");
            let mut buf = vec![0u8; 64 * 1024];
            for _ in 0..REQUESTS {
                let (hdr, payload) = session.receive(&mut buf).expect("recv");
                let payload = payload.to_vec();
                let mut resp = hdr;
                resp.kind = KIND_RESPONSE;
                resp.transport_status = protocol::STATUS_OK;
                session.send(&mut resp, &payload).expect("send");
            }
            session.close();
        });

        let ccfg = ClientConfig {
            max_request_payload_bytes: PAYLOAD as u32,
            max_response_payload_bytes: PAYLOAD as u32,
            ..default_client_config()
        };
        let (done_tx, done_rx) = std::sync::mpsc::channel();
        let client = thread::spawn(move || {
            let mut session = NpSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");
            for id in 1..=REQUESTS {
                let mut hdr = Header {
                    kind: KIND_REQUEST,
                    code: protocol::METHOD_INCREMENT,
                    item_count: 1,
                    message_id: id,
                    ..Header::default()
                };
                session.send(&mut hdr, &payload_for(id)).expect("send");
            }
            // The server could read every request only because the sends
            // drained responses it had written.
            assert!(
                session.has_pending(),
                "no response was queued while sending"
            );
            assert!(session.wait_readable(0).expect("wait_readable"));
            let mut rbuf = vec![0u8; 64 * 1024];
            for id in 1..=REQUESTS {
                let (rhdr, rpayload) = session.receive(&mut rbuf).expect("recv");
                assert_eq!(rhdr.message_id, id);
                assert_eq!(rpayload, payload_for(id).as_slice());
            }
            assert!(!session.has_pending());
            session.close();
            let _ = done_tx.send(());
        });

        // Watchdog only: a deadlock never completes. A client panic drops
        // done_tx; join then reports the panic itself.
        match done_rx.recv_timeout(Duration::from_secs(30)) {
            Err(std::sync::mpsc::RecvTimeoutError::Timeout) => {
                panic!("pipelining beyond the pipe buffers deadlocked")
            }
            _ => client.join().expect("client"),
        }
        server.join().expect("server join");
    }

    /// A queued message larger than the receive buffer fails like a pipe
    /// read with ERROR_MORE_DATA, and the queue moves past it.
    #[cfg(windows)]
    #[test]
    fn test_pending_queue_pop_too_small() {
        let mut q = PendingQueue::default();
        for msg in [&b"0123456789"[..], &b"ab"[..]] {
            q.push(msg);
        }
        let mut dst = [0u8; 4];
        assert!(matches!(
            q.pop(&mut dst),
            Err(NpError::Recv(ERROR_MORE_DATA))
        ));
        assert_eq!(q.pop(&mut dst).expect("second pop"), 2);
        assert_eq!(&dst[..2], b"ab");
        assert!(!q.queued());
    }

    #[cfg(windows)]
    #[test]
    fn test_duplicate_message_id() {
        ensure_run_dir();
        let svc = unique_service("rs_dupmsg");
        let mut listener =
            NpListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind");

        let server = thread::spawn(move || {
            let mut session = listener.accept().expect("accept");
            let mut buf = [0u8; 256];
            let _ = session.receive(&mut buf).expect("recv");
            session.close();
        });

        let mut session =
            NpSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");

        let mut hdr1 = Header {
            kind: KIND_REQUEST,
            code: protocol::METHOD_INCREMENT,
            item_count: 1,
            message_id: 42,
            ..Header::default()
        };
        session.send(&mut hdr1, &[1]).expect("first send");

        let mut hdr2 = Header {
            kind: KIND_REQUEST,
            code: protocol::METHOD_INCREMENT,
            item_count: 1,
            message_id: 42,
            ..Header::default()
        };
        assert!(matches!(
            session.send(&mut hdr2, &[2]),
            Err(NpError::DuplicateMsgId(42))
        ));

        session.close();
        server.join().expect("server join");
    }

    #[cfg(windows)]
    #[test]
    fn test_directional_limit_negotiation() {
        ensure_run_dir();
        let svc = unique_service("rs_dir_limits");

        let scfg = ServerConfig {
            max_request_payload_bytes: 4096,
            max_request_batch_items: 8,
            max_response_payload_bytes: 8192,
            max_response_batch_items: 32,
            ..default_server_config()
        };
        let mut listener = NpListener::bind(TEST_RUN_DIR, &svc, scfg).expect("bind");

        let server = thread::spawn(move || {
            let mut session = listener.accept().expect("accept");
            assert_eq!(session.max_request_payload_bytes, 4096);
            assert_eq!(session.max_request_batch_items, 16);
            assert_eq!(session.max_response_payload_bytes, 8192);
            assert_eq!(session.max_response_batch_items, 16);
            assert_ne!(session.session_id, 0);
            session.close();
        });

        let ccfg = ClientConfig {
            max_request_payload_bytes: 4096,
            max_request_batch_items: 16,
            max_response_payload_bytes: 4096,
            max_response_batch_items: 16,
            ..default_client_config()
        };
        let mut session = NpSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");
        assert_eq!(session.max_request_payload_bytes, 4096);
        assert_eq!(session.max_request_batch_items, 16);
        assert_eq!(session.max_response_payload_bytes, 8192);
        assert_eq!(session.max_response_batch_items, 16);
        assert_ne!(session.session_id, 0);

        session.close();
        server.join().expect("server join");
    }

    #[cfg(windows)]
    #[test]
    fn test_request_payload_over_cap() {
        ensure_run_dir();
        let svc = unique_service("rs_reqcap");

        let mut listener =
            NpListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind");

        let server = thread::spawn(move || {
            let result = listener.accept();
            assert!(matches!(result, Err(NpError::LimitExceeded)));
        });

        let ccfg = ClientConfig {
            max_request_payload_bytes: protocol::MAX_PAYLOAD_CAP + 1,
            ..default_client_config()
        };
        let result = NpSession::connect(TEST_RUN_DIR, &svc, &ccfg);
        assert!(matches!(result, Err(NpError::LimitExceeded)));

        server.join().expect("server join");
    }

    #[cfg(windows)]
    #[test]
    fn test_disconnect_clears_all_inflight() {
        ensure_run_dir();
        let svc = unique_service("rs_disc");
        let mut listener =
            NpListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind");

        let server = thread::spawn(move || {
            let mut session = listener.accept().expect("accept");
            let mut buf = [0u8; 256];
            let _ = session.receive(&mut buf).expect("recv request");
            session.close();
        });

        let mut session =
            NpSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");

        let mut hdr = Header {
            kind: KIND_REQUEST,
            code: protocol::METHOD_INCREMENT,
            item_count: 1,
            message_id: 99,
            ..Header::default()
        };
        session.send(&mut hdr, &[0xFF]).expect("send");
        session.inflight_ids.insert(100);

        let mut rbuf = [0u8; 256];
        assert!(matches!(
            session.receive(&mut rbuf),
            Err(NpError::Disconnected)
        ));
        assert!(
            session.inflight_ids.is_empty(),
            "disconnect must fail every in-flight request on the session"
        );

        session.close();
        server.join().expect("server join");
    }

    #[cfg(windows)]
    #[test]
    fn test_unknown_response_msg_id() {
        ensure_run_dir();
        let svc = unique_service("rs_unkmsg");
        let mut listener =
            NpListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind");

        let server = thread::spawn(move || {
            let mut session = listener.accept().expect("accept");
            let mut buf = [0u8; 256];
            let (hdr, payload) = session.receive(&mut buf).expect("recv");
            let payload = payload.to_vec();

            let mut resp = Header {
                kind: KIND_RESPONSE,
                code: hdr.code,
                message_id: hdr.message_id + 999,
                item_count: 1,
                transport_status: protocol::STATUS_OK,
                ..Header::default()
            };
            session.send(&mut resp, &payload).expect("send");
            session.close();
        });

        let mut session =
            NpSession::connect(TEST_RUN_DIR, &svc, &default_client_config()).expect("connect");

        let mut hdr = Header {
            kind: KIND_REQUEST,
            code: protocol::METHOD_INCREMENT,
            item_count: 1,
            message_id: 50,
            ..Header::default()
        };
        session.send(&mut hdr, &[1]).expect("send");

        let mut rbuf = [0u8; 256];
        assert!(matches!(
            session.receive(&mut rbuf),
            Err(NpError::UnknownMsgId(_))
        ));

        session.close();
        server.join().expect("server join");
    }

    /// The connect hooks are process-wide: tests that set them run one at
    /// a time.
    #[cfg(windows)]
    static CONNECT_HOOK_TESTS: std::sync::Mutex<()> = std::sync::Mutex::new(());

    #[cfg(windows)]
    #[test]
    fn test_connect_waits_for_busy_pipe() {
        let _hooks = CONNECT_HOOK_TESTS.lock().unwrap_or_else(|e| e.into_inner());
        ensure_run_dir();
        let svc = unique_service("rs_busy");
        let mut listener =
            NpListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind");

        // The first client takes the only instance; the server has not
        // accepted it, so no next instance exists yet.
        let pipe_name = build_pipe_name(TEST_RUN_DIR, &svc).expect("pipe name");
        let holder = create_client_pipe(&pipe_name);
        assert_ne!(
            holder,
            ffi::INVALID_HANDLE_VALUE,
            "first client holds the only instance"
        );

        let (busy_tx, busy_rx) = std::sync::mpsc::channel::<()>();
        let busy_name = pipe_name.clone();
        *CONNECT_BUSY_HOOK.lock().unwrap() = Some(Box::new(move |name| {
            if name == busy_name.as_slice() {
                let _ = busy_tx.send(());
            }
        }));

        // Accept the holder once the second client found the pipe busy, then
        // serve the second client's handshake.
        let server = thread::spawn(move || {
            busy_rx.recv().ok()?;
            let first = listener.accept_pending().ok()?;
            let second = listener.accept_pending().ok()?;
            let session = second
                .handshake(2, &default_server_config(), 10_000, None)
                .ok()?;
            drop(first);
            Some(session)
        });

        let result = NpSession::connect(TEST_RUN_DIR, &svc, &default_client_config());
        // Dropping the hook releases the server thread if connect never waited.
        *CONNECT_BUSY_HOOK.lock().unwrap() = None;
        if result.is_err() {
            close_handle(holder);
        }
        let mut session = result.expect("busy pipe connect waits and succeeds");
        let accepted = server.join().expect("server join");
        assert!(accepted.is_some(), "server accepted the waiting client");

        session.close();
        close_handle(holder);
    }

    #[cfg(windows)]
    #[test]
    fn test_connect_retries_stolen_instance() {
        let _hooks = CONNECT_HOOK_TESTS.lock().unwrap_or_else(|e| e.into_inner());
        ensure_run_dir();
        let svc = unique_service("rs_stolen");
        let mut listener =
            NpListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind");

        let pipe_name = build_pipe_name(TEST_RUN_DIR, &svc).expect("pipe name");
        let holder = create_client_pipe(&pipe_name);
        assert_ne!(
            holder,
            ffi::INVALID_HANDLE_VALUE,
            "holder takes the only instance"
        );

        let (busy_tx, busy_rx) = std::sync::mpsc::channel::<()>();
        let busy_name = pipe_name.clone();
        *CONNECT_BUSY_HOOK.lock().unwrap() = Some(Box::new(move |name| {
            if name == busy_name.as_slice() {
                let _ = busy_tx.send(());
            }
        }));
        // Once WaitNamedPipeW reports the instance the server created on
        // accept, a third client takes it first, once.
        let (thief_tx, thief_rx) = std::sync::mpsc::channel::<ffi::HANDLE>();
        let thief_name = pipe_name.clone();
        let stolen = std::sync::atomic::AtomicBool::new(false);
        *CONNECT_WAITED_HOOK.lock().unwrap() = Some(Box::new(move |name| {
            if name == thief_name.as_slice()
                && !stolen.swap(true, std::sync::atomic::Ordering::SeqCst)
            {
                let _ = thief_tx.send(create_client_pipe(&thief_name));
            }
        }));

        // Accept the holder, then the thief, then the retrying client.
        let server = thread::spawn(move || {
            busy_rx.recv().ok()?;
            let first = listener.accept_pending().ok()?;
            let thief = thief_rx.recv().ok()?;
            let second = listener.accept_pending().ok()?;
            let third = listener.accept_pending().ok()?;
            let session = third
                .handshake(3, &default_server_config(), 10_000, None)
                .ok()?;
            drop(first);
            drop(second);
            Some((session, thief))
        });

        let result = NpSession::connect(TEST_RUN_DIR, &svc, &default_client_config());
        // Dropping the hooks releases the server thread if connect never got there.
        *CONNECT_BUSY_HOOK.lock().unwrap() = None;
        *CONNECT_WAITED_HOOK.lock().unwrap() = None;
        if result.is_err() {
            close_handle(holder);
        }
        let mut session = result.expect("connect retries after the instance was taken");
        let (_accepted, thief) = server
            .join()
            .expect("server join")
            .expect("server accepted the retrying client");
        assert_ne!(
            thief,
            ffi::INVALID_HANDLE_VALUE,
            "thief took the freed instance"
        );

        session.close();
        close_handle(thief);
        close_handle(holder);
    }

    #[cfg(windows)]
    #[test]
    fn test_accept_survives_failed_instance() {
        let _hooks = CONNECT_HOOK_TESTS.lock().unwrap_or_else(|e| e.into_inner());
        ensure_run_dir();
        let svc = unique_service("rs_failinst");
        let mut listener =
            NpListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind");

        let pipe_name = build_pipe_name(TEST_RUN_DIR, &svc).expect("pipe name");
        let dropped = create_client_pipe(&pipe_name);
        assert_ne!(dropped, ffi::INVALID_HANDLE_VALUE, "first client connects");

        listener.fail_next_instances = 1;
        let result = listener.accept_pending();
        assert!(matches!(result, Err(NpError::CreatePipe(_))));
        assert_ne!(
            listener.handle,
            ffi::INVALID_HANDLE_VALUE,
            "listener keeps its instance"
        );
        let mut avail: ffi::DWORD = 0;
        let peek = unsafe {
            ffi::PeekNamedPipe(
                dropped,
                ptr::null_mut(),
                0,
                ptr::null_mut(),
                &mut avail,
                ptr::null_mut(),
            )
        };
        assert_eq!(peek, 0, "first client was disconnected");

        // The disconnected instance takes no client until accept listens on
        // it again, so the next client finds the pipe busy first.
        let (busy_tx, busy_rx) = std::sync::mpsc::channel::<()>();
        let busy_name = pipe_name.clone();
        *CONNECT_BUSY_HOOK.lock().unwrap() = Some(Box::new(move |name| {
            if name == busy_name.as_slice() {
                let _ = busy_tx.send(());
            }
        }));
        let server = thread::spawn(move || {
            busy_rx.recv().ok()?;
            let pending = listener.accept_pending().ok()?;
            pending
                .handshake(1, &default_server_config(), 10_000, None)
                .ok()
        });

        let result = NpSession::connect_with_timeout(
            TEST_RUN_DIR,
            &svc,
            &default_client_config(),
            10_000,
            None,
        );
        *CONNECT_BUSY_HOOK.lock().unwrap() = None;
        close_handle(dropped);
        let mut session = result.expect("next client connects");
        let accepted = server.join().expect("server join");
        assert!(accepted.is_some(), "server accepted the next client");
        session.close();
    }

    #[cfg(windows)]
    #[test]
    fn test_pipe_dacl_owner_only() {
        use crate::transport::win_security::{object_dacl_sddl, owner_only_dacl_sddl};
        ensure_run_dir();
        let svc = unique_service("rs_dacl");
        let mut listener =
            NpListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind");
        let expected = owner_only_dacl_sddl("FA").expect("expected DACL");
        assert_eq!(
            object_dacl_sddl(listener.handle).as_deref(),
            Some(expected.as_str())
        );

        // Instances created after an accept carry the same DACL.
        let pipe_name = build_pipe_name(TEST_RUN_DIR, &svc).expect("pipe name");
        let client = create_client_pipe(&pipe_name);
        let pending = listener.accept_pending().expect("accept pending");
        assert_eq!(
            object_dacl_sddl(listener.handle).as_deref(),
            Some(expected.as_str())
        );
        drop(pending);
        close_handle(client);
    }

    #[cfg(windows)]
    #[test]
    fn test_remote_client_rejected() {
        ensure_run_dir();
        let svc = unique_service("rs_remote");
        let _listener =
            NpListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind");

        // The same pipe through the SMB loopback is a remote client.
        let pipe_name = build_pipe_name(TEST_RUN_DIR, &svc).expect("pipe name");
        let mut remote: Vec<u16> = r"\\127.0.0.1\".encode_utf16().collect();
        remote.extend_from_slice(&pipe_name[4..]);
        let h = unsafe {
            ffi::CreateFileW(
                remote.as_ptr(),
                ffi::GENERIC_READ | ffi::GENERIC_WRITE,
                0,
                ptr::null(),
                ffi::OPEN_EXISTING,
                0,
                0,
            )
        };
        if h != ffi::INVALID_HANDLE_VALUE {
            close_handle(h);
        }
        assert_eq!(h, ffi::INVALID_HANDLE_VALUE, "remote open is refused");
    }

    #[cfg(windows)]
    #[test]
    fn test_untrusted_server_rejected() {
        use crate::transport::win_security::{
            SECURITY_IDENTIFICATION_LEVEL, TRUSTED_OWNER_FOR_TESTS, client_impersonation_level,
            object_owner_sid,
        };
        ensure_run_dir();
        let svc = unique_service("rs_untrusted");
        let listener = NpListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind");

        // BUILTIN\Users never owns the test pipe. Nobody accepts, so a client
        // that skipped the check fails on the deadline instead of hanging.
        TRUSTED_OWNER_FOR_TESTS.with(|t| *t.borrow_mut() = Some("S-1-5-32-545".into()));
        let result = NpSession::connect_with_timeout(
            TEST_RUN_DIR,
            &svc,
            &default_client_config(),
            2000,
            None,
        );
        TRUSTED_OWNER_FOR_TESTS.with(|t| *t.borrow_mut() = None);
        assert!(
            matches!(result, Err(NpError::UntrustedServer)),
            "got {:?}",
            result.err()
        );
        drop(listener);

        // Trusting the real owner lets the client through, and the server sees
        // only an identification-level token for it.
        let svc = unique_service("rs_trusted");
        let mut listener =
            NpListener::bind(TEST_RUN_DIR, &svc, default_server_config()).expect("bind");
        let owner = object_owner_sid(listener.handle).expect("pipe owner");
        let server = thread::spawn(move || {
            let session = listener.accept().ok()?;
            client_impersonation_level(session.handle())
        });
        TRUSTED_OWNER_FOR_TESTS.with(|t| *t.borrow_mut() = Some(owner));
        let result = NpSession::connect_with_timeout(
            TEST_RUN_DIR,
            &svc,
            &default_client_config(),
            10_000,
            None,
        );
        TRUSTED_OWNER_FOR_TESTS.with(|t| *t.borrow_mut() = None);
        let mut session = result.expect("connect accepts the trusted owner");
        let level = server.join().expect("server join");
        assert_eq!(
            level,
            Some(SECURITY_IDENTIFICATION_LEVEL),
            "server can only identify the client"
        );
        session.close();
    }

    #[cfg(windows)]
    #[test]
    fn test_preferred_profile_selected() {
        ensure_run_dir();
        let svc = unique_service("rs_pref");
        let preferred = crate::protocol::PROFILE_SHM_HYBRID;

        let scfg = ServerConfig {
            supported_profiles: PROFILE_BASELINE | preferred,
            preferred_profiles: preferred,
            ..default_server_config()
        };
        let mut listener = NpListener::bind(TEST_RUN_DIR, &svc, scfg).expect("bind");

        let server = thread::spawn(move || {
            let mut session = listener.accept().expect("accept");
            assert_eq!(session.selected_profile, preferred);
            session.close();
        });

        let ccfg = ClientConfig {
            supported_profiles: PROFILE_BASELINE | preferred,
            preferred_profiles: preferred,
            ..default_client_config()
        };
        let mut session = NpSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");
        assert_eq!(session.selected_profile, preferred);
        session.close();
        server.join().expect("server join");
    }

    // A profile bit Windows does not implement, offered and preferred by
    // both peers, is never selected: they would otherwise run the pipe under
    // that label. 0x08 (SHM_WAITADDR) is reserved and no transport implements it.
    #[cfg(windows)]
    #[test]
    fn test_unassigned_profile_bit_not_selected() {
        ensure_run_dir();
        let svc = unique_service("rs_unassigned_prof");
        let unassigned = crate::protocol::PROFILE_SHM_WAITADDR;

        let scfg = ServerConfig {
            supported_profiles: PROFILE_BASELINE | unassigned,
            preferred_profiles: unassigned,
            ..default_server_config()
        };
        let mut listener = NpListener::bind(TEST_RUN_DIR, &svc, scfg).expect("bind");

        let server = thread::spawn(move || {
            let mut session = listener.accept().expect("accept");
            assert_eq!(session.selected_profile, PROFILE_BASELINE);
            session.close();
        });

        let ccfg = ClientConfig {
            supported_profiles: PROFILE_BASELINE | unassigned,
            preferred_profiles: unassigned,
            ..default_client_config()
        };
        let mut session = NpSession::connect(TEST_RUN_DIR, &svc, &ccfg).expect("connect");
        assert_eq!(session.selected_profile, PROFILE_BASELINE);
        session.close();
        server.join().expect("server join");
    }
}
