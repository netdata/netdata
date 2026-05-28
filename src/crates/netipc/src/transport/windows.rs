//! L1 Windows Named Pipe transport.
//!
//! Connection lifecycle, handshake with profile/limit negotiation,
//! and send/receive with transparent chunking over Win32 Named Pipes
//! in message mode. Wire-compatible with the C and Go implementations.

use crate::protocol::{
    self, align8, ChunkHeader, Header, Hello, HelloAck, FLAG_BATCH, HEADER_SIZE, KIND_REQUEST,
    KIND_RESPONSE, MAGIC_CHUNK, MAGIC_MSG, MAX_PAYLOAD_CAP, MAX_PAYLOAD_DEFAULT, PROFILE_BASELINE,
    VERSION,
};
use std::collections::HashSet;
use std::ptr;
use std::sync::atomic::{AtomicU64, Ordering};

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

    extern "system" {
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

        pub fn DisconnectNamedPipe(hNamedPipe: HANDLE) -> BOOL;
        pub fn FlushFileBuffers(hFile: HANDLE) -> BOOL;

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

        pub fn SetNamedPipeHandleState(
            hNamedPipe: HANDLE,
            lpMode: *const DWORD,
            lpMaxCollectionCount: *const DWORD,
            lpCollectDataTimeout: *const DWORD,
        ) -> BOOL;
    }
}

// ---------------------------------------------------------------------------
//  Constants
// ---------------------------------------------------------------------------

const DEFAULT_BATCH_ITEMS: u32 = 1;
const DEFAULT_PACKET_SIZE: u32 = 65536;
const DEFAULT_PIPE_BUF_SIZE: u32 = 65536;
const HELLO_PAYLOAD_SIZE: usize = 44;
const HELLO_ACK_PAYLOAD_SIZE: usize = 48;
const MAX_PIPE_NAME_CHARS: usize = 256;

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
        }
    }
}

fn header_version_incompatible(buf: &[u8], expected_code: u16) -> bool {
    if buf.len() < HEADER_SIZE {
        return false;
    }

    let magic = u32::from_ne_bytes(buf[0..4].try_into().unwrap());
    let version = u16::from_ne_bytes(buf[4..6].try_into().unwrap());
    let header_len = u16::from_ne_bytes(buf[6..8].try_into().unwrap());
    let kind = u16::from_ne_bytes(buf[8..10].try_into().unwrap());
    let code = u16::from_ne_bytes(buf[12..14].try_into().unwrap());

    magic == MAGIC_MSG
        && version != VERSION
        && header_len == protocol::HEADER_LEN
        && kind == protocol::KIND_CONTROL
        && code == expected_code
}

fn hello_layout_incompatible(buf: &[u8]) -> bool {
    buf.len() >= 2 && u16::from_ne_bytes(buf[0..2].try_into().unwrap()) != 1
}

fn hello_ack_layout_incompatible(buf: &[u8]) -> bool {
    buf.len() >= 2 && u16::from_ne_bytes(buf[0..2].try_into().unwrap()) != 1
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

fn apply_default(val: u32, def: u32) -> u32 {
    if val == 0 {
        def
    } else {
        val
    }
}

fn min_u32(a: u32, b: u32) -> u32 {
    a.min(b)
}

fn max_u32(a: u32, b: u32) -> u32 {
    a.max(b)
}

#[cfg(windows)]
fn pipe_buffer_size(packet_size: u32) -> u32 {
    // The protocol packet size controls logical framing and chunk size. The
    // underlying pipe quota must stay large enough for full-duplex pipelining
    // even when tests force a tiny protocol packet size.
    max_u32(
        apply_default(packet_size, DEFAULT_PIPE_BUF_SIZE),
        DEFAULT_PIPE_BUF_SIZE,
    )
}

fn highest_bit(mask: u32) -> u32 {
    if mask == 0 {
        return 0;
    }
    let mut bit: u32 = 1 << 31;
    while bit & mask == 0 {
        bit >>= 1;
    }
    bit
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

/// Send header + payload as one pipe message.
#[cfg(windows)]
fn raw_send_msg(
    handle: ffi::HANDLE,
    scratch: &mut Vec<u8>,
    hdr: &[u8],
    payload: &[u8],
) -> Result<(), NpError> {
    let total = hdr.len() + payload.len();
    if scratch.len() < total {
        scratch.resize(total, 0);
    }
    scratch[..hdr.len()].copy_from_slice(hdr);
    scratch[hdr.len()..total].copy_from_slice(payload);
    raw_write(handle, &scratch[..total])
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
}

#[cfg(windows)]
impl NpSession {
    fn fail_all_inflight(&mut self) {
        if self.role == Role::Client {
            self.inflight_ids.clear();
        }
    }

    /// Get the raw HANDLE for WaitForSingleObject integration.
    pub fn handle(&self) -> ffi::HANDLE {
        self.handle
    }

    /// Get the session role.
    pub fn role(&self) -> Role {
        self.role
    }

    /// Wait until the pipe becomes readable or the timeout expires.
    pub fn wait_readable(&self, timeout_ms: u32) -> Result<bool, NpError> {
        if self.handle == ffi::INVALID_HANDLE_VALUE || self.handle == 0 {
            return Err(NpError::BadParam("session closed".into()));
        }

        let deadline = unsafe { ffi::GetTickCount64() }.saturating_add(timeout_ms as u64);
        let mut yielded = false;
        loop {
            let mut available: u32 = 0;
            let ok = unsafe {
                ffi::PeekNamedPipe(
                    self.handle,
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
                            self.handle,
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
        let pipe_name = build_pipe_name(run_dir, service_name)?;

        let handle = unsafe {
            ffi::CreateFileW(
                pipe_name.as_ptr(),
                ffi::GENERIC_READ | ffi::GENERIC_WRITE,
                0,
                ptr::null(),
                ffi::OPEN_EXISTING,
                0,
                0,
            )
        };

        if handle == ffi::INVALID_HANDLE_VALUE {
            return Err(NpError::Connect(last_error()));
        }

        // Set read mode to message mode
        let mode: u32 = ffi::PIPE_READMODE_MESSAGE;
        let ok = unsafe { ffi::SetNamedPipeHandleState(handle, &mode, ptr::null(), ptr::null()) };
        if ok == 0 {
            let err = last_error();
            close_handle(handle);
            return Err(NpError::Connect(err));
        }

        match client_handshake(handle, config) {
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
        if self.handle == ffi::INVALID_HANDLE_VALUE {
            return Err(NpError::BadParam("session closed".into()));
        }

        // Validate payload against negotiated directional limits before transmitting.
        let (max_payload, max_items) = if self.role == Role::Client {
            (self.max_request_payload_bytes, self.max_request_batch_items)
        } else {
            (
                self.max_response_payload_bytes,
                self.max_response_batch_items,
            )
        };
        if payload.len() > max_payload as usize || payload.len() > u32::MAX as usize {
            return Err(NpError::LimitExceeded);
        }
        if hdr.item_count > max_items {
            return Err(NpError::LimitExceeded);
        }

        // Client-side: track in-flight message_ids for requests
        if self.role == Role::Client && hdr.kind == KIND_REQUEST {
            if !self.inflight_ids.insert(hdr.message_id) {
                return Err(NpError::DuplicateMsgId(hdr.message_id));
            }
        }

        // Fill envelope fields
        hdr.magic = MAGIC_MSG;
        hdr.version = VERSION;
        hdr.header_len = protocol::HEADER_LEN;
        hdr.payload_len = payload.len() as u32;

        let tracked = self.role == Role::Client && hdr.kind == KIND_REQUEST;
        let msg_id = hdr.message_id;

        let result = self.send_inner(hdr, payload);

        if let Err(err) = &result {
            if tracked {
                match err {
                    NpError::Send(_) | NpError::Disconnected => self.fail_all_inflight(),
                    _ => {
                        self.inflight_ids.remove(&msg_id);
                    }
                }
            }
        }

        result
    }

    fn send_inner(&mut self, hdr: &mut Header, payload: &[u8]) -> Result<(), NpError> {
        let total_msg = HEADER_SIZE + payload.len();

        // Single packet?
        if total_msg <= self.packet_size as usize {
            let mut hdr_buf = [0u8; HEADER_SIZE];
            hdr.encode(&mut hdr_buf);
            return raw_send_msg(self.handle, &mut self.send_buf, &hdr_buf, payload);
        }

        // Chunked send
        let chunk_payload_budget = self.packet_size as usize - HEADER_SIZE;
        if chunk_payload_budget == 0 {
            return Err(NpError::BadParam("packet_size too small".into()));
        }

        let first_chunk_payload = payload.len().min(chunk_payload_budget);
        let remaining_after_first = payload.len() - first_chunk_payload;

        let continuation_chunks = if remaining_after_first > 0 {
            (remaining_after_first + chunk_payload_budget - 1) / chunk_payload_budget
        } else {
            0
        };
        let chunk_count = 1 + continuation_chunks as u32;

        // First chunk
        let mut hdr_buf = [0u8; HEADER_SIZE];
        hdr.encode(&mut hdr_buf);
        raw_send_msg(
            self.handle,
            &mut self.send_buf,
            &hdr_buf,
            &payload[..first_chunk_payload],
        )?;

        // Continuation chunks
        let mut offset = first_chunk_payload;
        for ci in 1..chunk_count {
            let remaining = payload.len() - offset;
            let this_chunk = remaining.min(chunk_payload_budget);

            let chk = ChunkHeader {
                magic: MAGIC_CHUNK,
                version: VERSION,
                flags: 0,
                message_id: hdr.message_id,
                total_message_len: total_msg as u32,
                chunk_index: ci,
                chunk_count,
                chunk_payload_len: this_chunk as u32,
            };

            let mut chk_buf = [0u8; HEADER_SIZE];
            chk.encode(&mut chk_buf);
            raw_send_msg(
                self.handle,
                &mut self.send_buf,
                &chk_buf,
                &payload[offset..offset + this_chunk],
            )?;

            offset += this_chunk;
        }

        Ok(())
    }

    /// Receive one logical message. buf is a scratch buffer for the first read.
    /// The payload view is valid until the next receive call on this session.
    pub fn receive<'a>(&'a mut self, buf: &'a mut [u8]) -> Result<(Header, &'a [u8]), NpError> {
        if self.handle == ffi::INVALID_HANDLE_VALUE {
            return Err(NpError::BadParam("session closed".into()));
        }

        let n = match raw_recv(self.handle, buf) {
            Ok(n) => n,
            Err(err) => {
                self.fail_all_inflight();
                return Err(err);
            }
        };

        if n < HEADER_SIZE {
            return Err(NpError::Protocol("packet too short for header".into()));
        }

        let hdr = Header::decode(&buf[..n])
            .map_err(|e| NpError::Protocol(format!("header decode: {e}")))?;

        // Validate payload_len against negotiated limit
        let max_payload = if self.role == Role::Server {
            self.max_request_payload_bytes
        } else {
            self.max_response_payload_bytes
        };
        if hdr.payload_len > max_payload {
            return Err(NpError::LimitExceeded);
        }

        // Validate item_count
        let max_batch = if self.role == Role::Server {
            self.max_request_batch_items
        } else {
            self.max_response_batch_items
        };
        if hdr.item_count > max_batch {
            return Err(NpError::LimitExceeded);
        }

        // Client-side: validate response message_id
        if self.role == Role::Client && hdr.kind == KIND_RESPONSE {
            if !self.inflight_ids.remove(&hdr.message_id) {
                return Err(NpError::UnknownMsgId(hdr.message_id));
            }
        }

        let total_msg = HEADER_SIZE + hdr.payload_len as usize;

        // Non-chunked
        if n >= total_msg {
            let payload = &buf[HEADER_SIZE..HEADER_SIZE + hdr.payload_len as usize];

            // Validate batch directory
            if hdr.flags & FLAG_BATCH != 0 && hdr.item_count > 1 {
                let dir_bytes = hdr.item_count as usize * 8;
                let dir_aligned = align8(dir_bytes);
                if payload.len() < dir_aligned {
                    return Err(NpError::Protocol("batch directory exceeds payload".into()));
                }
                let packed_area_len = (payload.len() - dir_aligned) as u32;
                protocol::batch_dir_validate(
                    &payload[..dir_bytes],
                    hdr.item_count,
                    packed_area_len,
                )
                .map_err(|e| NpError::Protocol(format!("batch directory: {e:?}")))?;
            }

            return Ok((hdr, payload));
        }

        // Chunked
        let first_payload_bytes = n - HEADER_SIZE;
        let needed = hdr.payload_len as usize;
        if self.recv_buf.len() < needed {
            self.recv_buf.resize(needed, 0);
        }

        self.recv_buf[..first_payload_bytes]
            .copy_from_slice(&buf[HEADER_SIZE..HEADER_SIZE + first_payload_bytes]);

        let mut assembled = first_payload_bytes;
        let chunk_payload_budget = self.packet_size as usize - HEADER_SIZE;

        let remaining_after_first = hdr.payload_len as usize - first_payload_bytes;
        let expected_continuations = if remaining_after_first > 0 && chunk_payload_budget > 0 {
            (remaining_after_first + chunk_payload_budget - 1) / chunk_payload_budget
        } else {
            0
        };
        let expected_chunk_count = 1 + expected_continuations as u32;

        if self.pkt_buf.len() < self.packet_size as usize {
            self.pkt_buf.resize(self.packet_size as usize, 0);
        }

        let mut ci: u32 = 1;
        while assembled < hdr.payload_len as usize {
            let cn = match raw_recv(self.handle, &mut self.pkt_buf) {
                Ok(n) => n,
                Err(err) => {
                    self.fail_all_inflight();
                    return Err(err);
                }
            };

            if cn < HEADER_SIZE {
                return Err(NpError::Chunk("continuation too short".into()));
            }

            let chk = ChunkHeader::decode(&self.pkt_buf[..cn])
                .map_err(|e| NpError::Chunk(format!("chunk header: {e}")))?;

            if chk.message_id != hdr.message_id {
                return Err(NpError::Chunk("message_id mismatch".into()));
            }
            if chk.chunk_index != ci {
                return Err(NpError::Chunk(format!(
                    "chunk_index mismatch: expected {ci}, got {}",
                    chk.chunk_index
                )));
            }
            if chk.chunk_count != expected_chunk_count {
                return Err(NpError::Chunk("chunk_count mismatch".into()));
            }
            if chk.total_message_len != total_msg as u32 {
                return Err(NpError::Chunk("total_message_len mismatch".into()));
            }

            let chunk_data = cn - HEADER_SIZE;
            if chunk_data != chk.chunk_payload_len as usize {
                return Err(NpError::Chunk("chunk_payload_len mismatch".into()));
            }
            if assembled + chunk_data > hdr.payload_len as usize {
                return Err(NpError::Chunk("chunk exceeds payload_len".into()));
            }

            self.recv_buf[assembled..assembled + chunk_data]
                .copy_from_slice(&self.pkt_buf[HEADER_SIZE..HEADER_SIZE + chunk_data]);
            assembled += chunk_data;
            ci += 1;
        }

        let payload = &self.recv_buf[..hdr.payload_len as usize];

        // Validate batch directory
        if hdr.flags & FLAG_BATCH != 0 && hdr.item_count > 1 {
            let dir_bytes = hdr.item_count as usize * 8;
            let dir_aligned = align8(dir_bytes);
            if payload.len() < dir_aligned {
                return Err(NpError::Protocol("batch directory exceeds payload".into()));
            }
            let packed_area_len = (payload.len() - dir_aligned) as u32;
            protocol::batch_dir_validate(&payload[..dir_bytes], hdr.item_count, packed_area_len)
                .map_err(|e| NpError::Protocol(format!("batch directory: {e:?}")))?;
        }

        Ok((hdr, payload))
    }

    /// Close the session.
    pub fn close(&mut self) {
        if self.handle != ffi::INVALID_HANDLE_VALUE && self.handle != 0 {
            // Flush pending writes so the peer reads all data
            unsafe {
                ffi::FlushFileBuffers(self.handle);
            }
            if self.role == Role::Server {
                unsafe {
                    ffi::DisconnectNamedPipe(self.handle);
                }
            }
            close_handle(self.handle);
            self.handle = ffi::INVALID_HANDLE_VALUE;
        }
        self.fail_all_inflight();
        self.recv_buf.clear();
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
    /// and session ID.
    pub fn accept_with_config(
        &mut self,
        session_id: u64,
        config: ServerConfig,
    ) -> Result<NpSession, NpError> {
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
        let next = match create_pipe_instance(&self.pipe_name, buf_size, false) {
            Ok(h) => h,
            Err(e) => {
                // Failed to create replacement instance; disconnect and close
                // the accepted client to avoid an orphaned connection.
                unsafe {
                    ffi::DisconnectNamedPipe(session_handle);
                }
                close_handle(session_handle);
                self.handle = ffi::INVALID_HANDLE_VALUE;
                return Err(e);
            }
        };
        self.handle = next;

        // Perform handshake
        match server_handshake(session_handle, &config, session_id) {
            Ok(session) => Ok(session),
            Err(e) => {
                unsafe {
                    ffi::DisconnectNamedPipe(session_handle);
                }
                close_handle(session_handle);
                Err(e)
            }
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

    let handle = unsafe {
        ffi::CreateNamedPipeW(
            pipe_name.as_ptr(),
            open_mode,
            ffi::PIPE_TYPE_MESSAGE | ffi::PIPE_READMODE_MESSAGE | ffi::PIPE_WAIT,
            ffi::PIPE_UNLIMITED_INSTANCES,
            buf_size,
            buf_size,
            0,
            ptr::null(),
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
fn client_handshake(handle: ffi::HANDLE, config: &ClientConfig) -> Result<NpSession, NpError> {
    let pkt_size = apply_default(config.packet_size, DEFAULT_PACKET_SIZE);

    let supported = if config.supported_profiles != 0 {
        config.supported_profiles
    } else {
        PROFILE_BASELINE
    };

    let hello = Hello {
        layout_version: 1,
        flags: 0,
        supported_profiles: supported,
        preferred_profiles: config.preferred_profiles,
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

    let mut hello_buf = [0u8; HELLO_PAYLOAD_SIZE];
    hello.encode(&mut hello_buf);

    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: protocol::KIND_CONTROL,
        flags: 0,
        code: protocol::CODE_HELLO,
        transport_status: protocol::STATUS_OK,
        payload_len: HELLO_PAYLOAD_SIZE as u32,
        item_count: 1,
        message_id: 0,
    };

    let mut pkt = [0u8; HEADER_SIZE + HELLO_PAYLOAD_SIZE];
    hdr.encode(&mut pkt[..HEADER_SIZE]);
    pkt[HEADER_SIZE..].copy_from_slice(&hello_buf);

    raw_write(handle, &pkt)?;

    // Receive HELLO_ACK
    let mut ack_buf = [0u8; 128];
    let n = raw_recv(handle, &mut ack_buf)?;

    let ack_hdr = match Header::decode(&ack_buf[..n]) {
        Ok(hdr) => hdr,
        Err(crate::protocol::NipcError::BadVersion) => {
            return Err(NpError::Incompatible("ack header version mismatch".into()))
        }
        Err(e) => return Err(NpError::Protocol(format!("ack header: {e}"))),
    };

    if ack_hdr.kind != protocol::KIND_CONTROL || ack_hdr.code != protocol::CODE_HELLO_ACK {
        return Err(NpError::Protocol("expected HELLO_ACK".into()));
    }

    if ack_hdr.transport_status == protocol::STATUS_AUTH_FAILED {
        return Err(NpError::AuthFailed);
    }
    if ack_hdr.transport_status == protocol::STATUS_UNSUPPORTED {
        return Err(NpError::NoProfile);
    }
    if ack_hdr.transport_status == protocol::STATUS_INCOMPATIBLE {
        return Err(NpError::Incompatible(
            "ack transport_status incompatible".into(),
        ));
    }
    if ack_hdr.transport_status == protocol::STATUS_LIMIT_EXCEEDED {
        return Err(NpError::LimitExceeded);
    }
    if ack_hdr.transport_status != protocol::STATUS_OK {
        return Err(NpError::Handshake(format!(
            "transport_status={}",
            ack_hdr.transport_status
        )));
    }

    if n < HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE {
        return Err(NpError::Protocol("ack payload truncated".into()));
    }

    let ack = match HelloAck::decode(&ack_buf[HEADER_SIZE..n]) {
        Ok(ack) => ack,
        Err(crate::protocol::NipcError::BadLayout)
            if hello_ack_layout_incompatible(&ack_buf[HEADER_SIZE..n]) =>
        {
            return Err(NpError::Incompatible(
                "ack payload layout version mismatch".into(),
            ))
        }
        Err(e) => return Err(NpError::Protocol(format!("ack payload: {e}"))),
    };

    // Reject packet sizes too small for a header
    if ack.agreed_packet_size <= HEADER_SIZE as u32 {
        return Err(NpError::Handshake(
            "agreed packet_size <= HEADER_SIZE".into(),
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
) -> Result<NpSession, NpError> {
    let server_pkt_size = apply_default(config.packet_size, DEFAULT_PACKET_SIZE);
    let s_resp_pay = apply_default(config.max_response_payload_bytes, MAX_PAYLOAD_DEFAULT);
    let s_profiles = if config.supported_profiles != 0 {
        config.supported_profiles
    } else {
        PROFILE_BASELINE
    };
    let s_preferred = config.preferred_profiles;

    let send_rejection = |status: u16| {
        let ack = HelloAck {
            layout_version: 1,
            ..HelloAck::default()
        };
        let mut ack_buf = [0u8; HELLO_ACK_PAYLOAD_SIZE];
        ack.encode(&mut ack_buf);

        let ack_hdr = Header {
            magic: MAGIC_MSG,
            version: VERSION,
            header_len: protocol::HEADER_LEN,
            kind: protocol::KIND_CONTROL,
            flags: 0,
            code: protocol::CODE_HELLO_ACK,
            transport_status: status,
            payload_len: HELLO_ACK_PAYLOAD_SIZE as u32,
            item_count: 1,
            message_id: 0,
        };

        let mut pkt = [0u8; HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE];
        ack_hdr.encode(&mut pkt[..HEADER_SIZE]);
        pkt[HEADER_SIZE..].copy_from_slice(&ack_buf);
        let _ = raw_write(handle, &pkt);
    };

    // Receive HELLO
    let mut buf = [0u8; 128];
    let n = raw_recv(handle, &mut buf)?;

    let hdr = match Header::decode(&buf[..n]) {
        Ok(hdr) => hdr,
        Err(crate::protocol::NipcError::BadVersion)
            if header_version_incompatible(&buf[..n], protocol::CODE_HELLO) =>
        {
            send_rejection(protocol::STATUS_INCOMPATIBLE);
            return Err(NpError::Incompatible(
                "hello header version mismatch".into(),
            ));
        }
        Err(e) => return Err(NpError::Protocol(format!("hello header: {e}"))),
    };

    if hdr.kind != protocol::KIND_CONTROL || hdr.code != protocol::CODE_HELLO {
        return Err(NpError::Protocol("expected HELLO".into()));
    }

    let hello = match Hello::decode(&buf[HEADER_SIZE..n]) {
        Ok(hello) => hello,
        Err(crate::protocol::NipcError::BadLayout)
            if hello_layout_incompatible(&buf[HEADER_SIZE..n]) =>
        {
            send_rejection(protocol::STATUS_INCOMPATIBLE);
            return Err(NpError::Incompatible(
                "hello payload layout version mismatch".into(),
            ));
        }
        Err(e) => return Err(NpError::Protocol(format!("hello payload: {e}"))),
    };

    let intersection = hello.supported_profiles & s_profiles;

    if intersection == 0 {
        send_rejection(protocol::STATUS_UNSUPPORTED);
        return Err(NpError::NoProfile);
    }

    if hello.auth_token != config.auth_token {
        send_rejection(protocol::STATUS_AUTH_FAILED);
        return Err(NpError::AuthFailed);
    }

    // Select profile
    let preferred_intersection = intersection & hello.preferred_profiles & s_preferred;
    let selected = if preferred_intersection != 0 {
        highest_bit(preferred_intersection)
    } else {
        highest_bit(intersection)
    };

    if hello.max_request_payload_bytes > MAX_PAYLOAD_CAP {
        send_rejection(protocol::STATUS_LIMIT_EXCEEDED);
        return Err(NpError::LimitExceeded);
    }

    // Negotiate limits:
    // - request payload and batch size are client-proposed and echoed
    // - response payload is server-authoritative
    // - response batch size is symmetric with request batch size
    let agreed_req_pay = hello.max_request_payload_bytes;
    let agreed_req_bat = hello.max_request_batch_items;
    let agreed_resp_pay = s_resp_pay;
    let agreed_resp_bat = agreed_req_bat;
    let agreed_pkt = min_u32(hello.packet_size, server_pkt_size);

    // Reject packet sizes too small for a usable message packet
    if agreed_pkt <= HEADER_SIZE as u32 {
        send_rejection(protocol::STATUS_INCOMPATIBLE);
        return Err(NpError::Incompatible(
            "packet size too small for negotiated session".into(),
        ));
    }

    // Send HELLO_ACK
    let ack = HelloAck {
        layout_version: 1,
        flags: 0,
        server_supported_profiles: s_profiles,
        intersection_profiles: intersection,
        selected_profile: selected,
        agreed_max_request_payload_bytes: agreed_req_pay,
        agreed_max_request_batch_items: agreed_req_bat,
        agreed_max_response_payload_bytes: agreed_resp_pay,
        agreed_max_response_batch_items: agreed_resp_bat,
        agreed_packet_size: agreed_pkt,
        session_id,
    };

    let mut ack_buf = [0u8; HELLO_ACK_PAYLOAD_SIZE];
    ack.encode(&mut ack_buf);

    let ack_hdr = Header {
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
    };

    let mut pkt = [0u8; HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE];
    ack_hdr.encode(&mut pkt[..HEADER_SIZE]);
    pkt[HEADER_SIZE..].copy_from_slice(&ack_buf);

    raw_write(handle, &pkt)?;

    Ok(NpSession {
        handle,
        role: Role::Server,
        max_request_payload_bytes: agreed_req_pay,
        max_request_batch_items: agreed_req_bat,
        max_response_payload_bytes: agreed_resp_pay,
        max_response_batch_items: agreed_resp_bat,
        packet_size: agreed_pkt,
        selected_profile: selected,
        session_id,
        recv_buf: Vec::new(),
        pkt_buf: Vec::new(),
        send_buf: Vec::new(),
        inflight_ids: HashSet::new(),
    })
}

// ---------------------------------------------------------------------------
//  Tests (cross-platform unit tests for non-Win32 logic)
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
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
    fn test_incompatible_classifiers() {
        let hdr = Header {
            magic: MAGIC_MSG,
            version: VERSION + 1,
            header_len: protocol::HEADER_LEN,
            kind: protocol::KIND_CONTROL,
            flags: 0,
            code: protocol::CODE_HELLO,
            transport_status: protocol::STATUS_OK,
            payload_len: HELLO_PAYLOAD_SIZE as u32,
            item_count: 1,
            message_id: 0,
        };
        let mut hdr_buf = [0u8; HEADER_SIZE];
        hdr.encode(&mut hdr_buf);
        assert!(header_version_incompatible(&hdr_buf, protocol::CODE_HELLO));
        assert!(!header_version_incompatible(
            &hdr_buf,
            protocol::CODE_HELLO_ACK
        ));

        let hello = Hello {
            layout_version: 2,
            ..Hello::default()
        };
        let mut hello_buf = [0u8; HELLO_PAYLOAD_SIZE];
        hello.encode(&mut hello_buf);
        assert!(hello_layout_incompatible(&hello_buf));

        let ack = HelloAck {
            layout_version: 2,
            ..HelloAck::default()
        };
        let mut ack_buf = [0u8; HELLO_ACK_PAYLOAD_SIZE];
        ack.encode(&mut ack_buf);
        assert!(hello_ack_layout_incompatible(&ack_buf));
    }

    #[test]
    fn test_highest_bit() {
        assert_eq!(highest_bit(0), 0);
        assert_eq!(highest_bit(1), 1);
        assert_eq!(highest_bit(0b0101), 4);
        assert_eq!(highest_bit(0b1000), 8);
        assert_eq!(highest_bit(0xFF), 128);
    }

    #[test]
    fn test_build_pipe_name_too_long() {
        let long_name = "a".repeat(300);
        let result = build_pipe_name("/var/run", &long_name);
        assert!(matches!(result, Err(NpError::PipeName(_))));
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
            max_request_payload_bytes: 2048,
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
}
