//! L1 POSIX UDS SEQPACKET transport.
//!
//! Connection lifecycle, handshake with profile/limit negotiation,
//! and send/receive with transparent chunking over AF_UNIX SEQPACKET sockets.
//! Wire-compatible with the C implementation in netipc_uds.c.

use crate::protocol::{
    self, align8, ChunkHeader, Header, Hello, HelloAck, FLAG_BATCH, HEADER_SIZE, KIND_REQUEST,
    KIND_RESPONSE, MAGIC_CHUNK, MAGIC_MSG, MAX_PAYLOAD_CAP, MAX_PAYLOAD_DEFAULT, PROFILE_BASELINE,
    VERSION,
};
use std::collections::HashSet;
use std::ffi::CString;
use std::io;
use std::os::unix::io::RawFd;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};

// ---------------------------------------------------------------------------
//  Constants
// ---------------------------------------------------------------------------

const DEFAULT_BACKLOG: i32 = 16;
const DEFAULT_BATCH_ITEMS: u32 = 1;
const DEFAULT_PACKET_SIZE_FALLBACK: u32 = 65536;
const HELLO_PAYLOAD_SIZE: usize = 44;
const HELLO_ACK_PAYLOAD_SIZE: usize = 48;

// ---------------------------------------------------------------------------
//  Errors
// ---------------------------------------------------------------------------

/// Transport-level errors.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum UdsError {
    /// Socket path exceeds sun_path limit.
    PathTooLong,
    /// socket()/bind()/listen() syscall failed.
    Socket(i32),
    /// connect() failed.
    Connect(i32),
    /// accept() failed.
    Accept(i32),
    /// send()/sendmsg() failed.
    Send(i32),
    /// recv() failed or peer disconnected.
    Recv(i32),
    /// Handshake protocol error.
    Handshake(String),
    /// Authentication token rejected.
    AuthFailed,
    /// No common profile between client and server.
    NoProfile,
    /// Protocol or layout version mismatch.
    Incompatible(String),
    /// Wire protocol violation.
    Protocol(String),
    /// A live server already owns this socket path.
    AddrInUse,
    /// Chunk header mismatch during reassembly.
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
}

impl std::fmt::Display for UdsError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            UdsError::PathTooLong => write!(f, "socket path exceeds sun_path limit"),
            UdsError::Socket(e) => write!(f, "socket syscall failed: errno {e}"),
            UdsError::Connect(e) => write!(f, "connect failed: errno {e}"),
            UdsError::Accept(e) => write!(f, "accept failed: errno {e}"),
            UdsError::Send(e) => write!(f, "send failed: errno {e}"),
            UdsError::Recv(e) => write!(f, "recv failed: errno {e}"),
            UdsError::Handshake(s) => write!(f, "handshake error: {s}"),
            UdsError::AuthFailed => write!(f, "authentication token rejected"),
            UdsError::NoProfile => write!(f, "no common transport profile"),
            UdsError::Incompatible(s) => write!(f, "incompatible protocol: {s}"),
            UdsError::Protocol(s) => write!(f, "protocol violation: {s}"),
            UdsError::AddrInUse => write!(f, "address already in use by live server"),
            UdsError::Chunk(s) => write!(f, "chunk error: {s}"),
            UdsError::Alloc => write!(f, "memory allocation failed"),
            UdsError::LimitExceeded => write!(f, "negotiated limit exceeded"),
            UdsError::BadParam(s) => write!(f, "bad parameter: {s}"),
            UdsError::DuplicateMsgId(id) => write!(f, "duplicate message_id: {id}"),
            UdsError::UnknownMsgId(id) => write!(f, "unknown response message_id: {id}"),
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

impl std::error::Error for UdsError {}

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
    /// 0 = auto-detect from SO_SNDBUF.
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
    /// 0 = auto-detect from SO_SNDBUF.
    pub packet_size: u32,
    /// listen() backlog, 0 = default (16).
    pub backlog: i32,
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
            backlog: 0,
        }
    }
}

// ---------------------------------------------------------------------------
//  Session
// ---------------------------------------------------------------------------

/// A connected UDS SEQPACKET session (client or server side).
pub struct UdsSession {
    fd: RawFd,
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

    // In-flight message_id set (client-side only)
    inflight_ids: HashSet<u64>,
}

impl UdsSession {
    fn fail_all_inflight(&mut self) {
        if self.role == Role::Client {
            self.inflight_ids.clear();
        }
    }

    /// Get the raw fd for poll/epoll integration.
    pub fn fd(&self) -> RawFd {
        self.fd
    }

    /// Get the session role.
    pub fn role(&self) -> Role {
        self.role
    }

    /// Return the most recently reassembled payload stored in the internal
    /// receive buffer.
    pub fn received_payload(&self, len: usize) -> &[u8] {
        &self.recv_buf[..len]
    }

    /// Connect to a server at `{run_dir}/{service_name}.sock`.
    /// Performs the full handshake. Blocks until connected + handshake done.
    pub fn connect(
        run_dir: &str,
        service_name: &str,
        config: &ClientConfig,
    ) -> Result<Self, UdsError> {
        let path = build_socket_path(run_dir, service_name)?;

        let fd = unsafe { libc::socket(libc::AF_UNIX, libc::SOCK_SEQPACKET, 0) };
        if fd < 0 {
            return Err(UdsError::Socket(errno()));
        }

        let result = connect_and_handshake(fd, &path, config);
        if result.is_err() {
            unsafe {
                libc::close(fd);
            }
        }
        result
    }

    /// Send one logical message. `hdr` is the 32-byte outer header (caller
    /// fills kind, code, flags, item_count, message_id; this function sets
    /// magic/version/header_len/payload_len).
    ///
    /// If the total message (32 + payload_len) exceeds packet_size, the
    /// message is chunked transparently.
    pub fn send(&mut self, hdr: &mut Header, payload: &[u8]) -> Result<(), UdsError> {
        if self.fd < 0 {
            return Err(UdsError::BadParam("session closed".into()));
        }

        // Validate payload against negotiated directional limits before transmitting.
        // The u32 cast below requires payload.len() <= u32::MAX.
        let (max_payload, max_items) = if self.role == Role::Client {
            (self.max_request_payload_bytes, self.max_request_batch_items)
        } else {
            (
                self.max_response_payload_bytes,
                self.max_response_batch_items,
            )
        };
        if payload.len() > max_payload as usize || payload.len() > u32::MAX as usize {
            return Err(UdsError::LimitExceeded);
        }
        if hdr.item_count > max_items {
            return Err(UdsError::LimitExceeded);
        }

        // Client-side: track in-flight message_ids for requests
        if self.role == Role::Client && hdr.kind == KIND_REQUEST {
            if !self.inflight_ids.insert(hdr.message_id) {
                return Err(UdsError::DuplicateMsgId(hdr.message_id));
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
                    UdsError::Send(_) => self.fail_all_inflight(),
                    _ => {
                        self.inflight_ids.remove(&msg_id);
                    }
                }
            }
        }

        result
    }

    /// Inner send logic, separated so the caller can rollback on failure.
    fn send_inner(&mut self, hdr: &mut Header, payload: &[u8]) -> Result<(), UdsError> {
        let total_msg = HEADER_SIZE + payload.len();

        // Single packet?
        if total_msg <= self.packet_size as usize {
            let mut hdr_buf = [0u8; HEADER_SIZE];
            hdr.encode(&mut hdr_buf);
            return raw_send_iov(self.fd, &hdr_buf, payload);
        }

        // Chunked send
        let chunk_payload_budget = self.packet_size as usize - HEADER_SIZE;
        if chunk_payload_budget == 0 {
            return Err(UdsError::BadParam("packet_size too small".into()));
        }

        let first_chunk_payload = payload.len().min(chunk_payload_budget);
        let remaining_after_first = payload.len() - first_chunk_payload;

        let continuation_chunks = if remaining_after_first > 0 {
            (remaining_after_first + chunk_payload_budget - 1) / chunk_payload_budget
        } else {
            0
        };
        let chunk_count = 1 + continuation_chunks as u32;

        // Send first chunk: outer header + first part of payload
        let mut hdr_buf = [0u8; HEADER_SIZE];
        hdr.encode(&mut hdr_buf);
        raw_send_iov(self.fd, &hdr_buf, &payload[..first_chunk_payload])?;

        // Send continuation chunks
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
            raw_send_iov(self.fd, &chk_buf, &payload[offset..offset + this_chunk])?;

            offset += this_chunk;
        }

        Ok(())
    }

    /// Receive one logical message. Blocks until a complete message arrives.
    ///
    /// On success, returns (header, payload_view). The payload view is valid
    /// until the next receive call on this session.
    pub fn receive<'a>(&'a mut self, buf: &'a mut [u8]) -> Result<(Header, &'a [u8]), UdsError> {
        if self.fd < 0 {
            return Err(UdsError::BadParam("session closed".into()));
        }

        // Read first packet
        let n = match raw_recv(self.fd, buf) {
            Ok(n) => n,
            Err(err) => {
                self.fail_all_inflight();
                return Err(err);
            }
        };

        if n < HEADER_SIZE {
            return Err(UdsError::Protocol("packet too short for header".into()));
        }

        let hdr = Header::decode(&buf[..n])
            .map_err(|e| UdsError::Protocol(format!("header decode: {e}")))?;

        // Validate payload_len against negotiated directional limit.
        // Server receives requests; client receives responses.
        let max_payload = if self.role == Role::Server {
            self.max_request_payload_bytes
        } else {
            self.max_response_payload_bytes
        };
        if hdr.payload_len > max_payload {
            return Err(UdsError::LimitExceeded);
        }

        // Validate item_count against negotiated directional batch limit.
        let max_batch = if self.role == Role::Server {
            self.max_request_batch_items
        } else {
            self.max_response_batch_items
        };
        if hdr.item_count > max_batch {
            return Err(UdsError::LimitExceeded);
        }

        // Client-side: validate response message_id is in-flight
        if self.role == Role::Client && hdr.kind == KIND_RESPONSE {
            if !self.inflight_ids.remove(&hdr.message_id) {
                return Err(UdsError::UnknownMsgId(hdr.message_id));
            }
        }

        let total_msg = HEADER_SIZE + hdr.payload_len as usize;

        // Non-chunked: entire message in one packet
        if n >= total_msg {
            let payload = &buf[HEADER_SIZE..HEADER_SIZE + hdr.payload_len as usize];

            // Validate batch directory
            if hdr.flags & FLAG_BATCH != 0 && hdr.item_count > 1 {
                let dir_bytes = hdr.item_count as usize * 8;
                let dir_aligned = align8(dir_bytes);
                if payload.len() < dir_aligned {
                    return Err(UdsError::Protocol("batch directory exceeds payload".into()));
                }
                let packed_area_len = (payload.len() - dir_aligned) as u32;
                protocol::batch_dir_validate(
                    &payload[..dir_bytes],
                    hdr.item_count,
                    packed_area_len,
                )
                .map_err(|e| UdsError::Protocol(format!("batch directory: {e:?}")))?;
            }

            return Ok((hdr, payload));
        }

        // Chunked: first packet has partial payload
        let first_payload_bytes = n - HEADER_SIZE;

        // Grow recv_buf to hold full payload
        let needed = hdr.payload_len as usize;
        if self.recv_buf.len() < needed {
            self.recv_buf.resize(needed, 0);
        }

        // Copy first chunk's payload
        self.recv_buf[..first_payload_bytes]
            .copy_from_slice(&buf[HEADER_SIZE..HEADER_SIZE + first_payload_bytes]);

        let mut assembled = first_payload_bytes;
        let chunk_payload_budget = self.packet_size as usize - HEADER_SIZE;

        // Expected chunk count
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

        let mut ci = 1u32;
        while assembled < hdr.payload_len as usize {
            let cn = match raw_recv(self.fd, &mut self.pkt_buf) {
                Ok(n) => n,
                Err(err) => {
                    self.fail_all_inflight();
                    return Err(err);
                }
            };

            if cn < HEADER_SIZE {
                return Err(UdsError::Chunk("continuation too short".into()));
            }

            let chk = ChunkHeader::decode(&self.pkt_buf[..cn])
                .map_err(|e| UdsError::Chunk(format!("chunk header: {e}")))?;

            // Validate chunk header
            if chk.message_id != hdr.message_id {
                return Err(UdsError::Chunk("message_id mismatch".into()));
            }
            if chk.chunk_index != ci {
                return Err(UdsError::Chunk(format!(
                    "chunk_index mismatch: expected {ci}, got {}",
                    chk.chunk_index
                )));
            }
            if chk.chunk_count != expected_chunk_count {
                return Err(UdsError::Chunk("chunk_count mismatch".into()));
            }
            if chk.total_message_len != total_msg as u32 {
                return Err(UdsError::Chunk("total_message_len mismatch".into()));
            }

            let chunk_data = cn - HEADER_SIZE;
            if chunk_data != chk.chunk_payload_len as usize {
                return Err(UdsError::Chunk("chunk_payload_len mismatch".into()));
            }
            if assembled + chunk_data > hdr.payload_len as usize {
                return Err(UdsError::Chunk("chunk exceeds payload_len".into()));
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
                return Err(UdsError::Protocol("batch directory exceeds payload".into()));
            }
            let packed_area_len = (payload.len() - dir_aligned) as u32;
            protocol::batch_dir_validate(&payload[..dir_bytes], hdr.item_count, packed_area_len)
                .map_err(|e| UdsError::Protocol(format!("batch directory: {e:?}")))?;
        }

        Ok((hdr, payload))
    }
}

impl Drop for UdsSession {
    fn drop(&mut self) {
        if self.fd >= 0 {
            unsafe {
                libc::close(self.fd);
            }
            self.fd = -1;
        }
    }
}

// ---------------------------------------------------------------------------
//  Listener
// ---------------------------------------------------------------------------

/// A listening UDS SEQPACKET endpoint.
pub struct UdsListener {
    fd: RawFd,
    config: ServerConfig,
    path: PathBuf,
    next_session_id: AtomicU64,
}

impl UdsListener {
    /// Create a listener on `{run_dir}/{service_name}.sock`.
    /// Performs stale endpoint recovery.
    pub fn bind(run_dir: &str, service_name: &str, config: ServerConfig) -> Result<Self, UdsError> {
        let path = build_socket_path(run_dir, service_name)?;

        // Stale recovery
        match check_and_recover_stale(&path) {
            StaleResult::LiveServer => return Err(UdsError::AddrInUse),
            StaleResult::Stale | StaleResult::NotExist => { /* proceed */ }
        }

        let fd = unsafe { libc::socket(libc::AF_UNIX, libc::SOCK_SEQPACKET, 0) };
        if fd < 0 {
            return Err(UdsError::Socket(errno()));
        }

        // Bind
        if let Err(e) = bind_unix(fd, &path) {
            unsafe {
                libc::close(fd);
            }
            return Err(e);
        }

        let backlog = if config.backlog > 0 {
            config.backlog
        } else {
            DEFAULT_BACKLOG
        };

        if unsafe { libc::listen(fd, backlog) } < 0 {
            let e = errno();
            unsafe {
                libc::close(fd);
            }
            let _ = std::fs::remove_file(&path);
            return Err(UdsError::Socket(e));
        }

        Ok(UdsListener {
            fd,
            config,
            path: PathBuf::from(&path),
            next_session_id: AtomicU64::new(1),
        })
    }

    /// Get the raw fd for poll/epoll integration.
    pub fn fd(&self) -> RawFd {
        self.fd
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

    /// Accept one client. Performs the full handshake.
    /// Blocks until a client connects and the handshake completes.
    pub fn accept(&self) -> Result<UdsSession, UdsError> {
        let session_id = self.next_session_id.fetch_add(1, Ordering::Relaxed);
        self.accept_with_config(session_id, self.config.clone())
    }

    /// Accept one client using a caller-provided per-session server config
    /// and session ID.
    pub fn accept_with_config(
        &self,
        session_id: u64,
        config: ServerConfig,
    ) -> Result<UdsSession, UdsError> {
        let client_fd =
            unsafe { libc::accept(self.fd, std::ptr::null_mut(), std::ptr::null_mut()) };
        if client_fd < 0 {
            return Err(UdsError::Accept(errno()));
        }

        match server_handshake(client_fd, &config, session_id) {
            Ok(session) => Ok(session),
            Err(e) => {
                unsafe {
                    libc::close(client_fd);
                }
                Err(e)
            }
        }
    }
}

impl Drop for UdsListener {
    fn drop(&mut self) {
        if self.fd >= 0 {
            unsafe {
                libc::close(self.fd);
            }
            self.fd = -1;
        }
        if self.path.exists() {
            let _ = std::fs::remove_file(&self.path);
        }
    }
}

// ---------------------------------------------------------------------------
//  Internal helpers
// ---------------------------------------------------------------------------

fn errno() -> i32 {
    io::Error::last_os_error().raw_os_error().unwrap_or(0)
}

/// Validate service_name: only [a-zA-Z0-9._-], non-empty, not "." or "..".
fn validate_service_name(name: &str) -> Result<(), UdsError> {
    if name.is_empty() {
        return Err(UdsError::BadParam("empty service name".into()));
    }
    if name == "." || name == ".." {
        return Err(UdsError::BadParam(
            "service name cannot be '.' or '..'".into(),
        ));
    }
    for c in name.bytes() {
        match c {
            b'a'..=b'z' | b'A'..=b'Z' | b'0'..=b'9' | b'.' | b'_' | b'-' => {}
            _ => {
                return Err(UdsError::BadParam(format!(
                    "service name contains invalid character: {:?}",
                    c as char
                )))
            }
        }
    }
    Ok(())
}

/// Build `{run_dir}/{service_name}.sock` and validate length.
fn build_socket_path(run_dir: &str, service_name: &str) -> Result<String, UdsError> {
    validate_service_name(service_name)?;

    let path = format!("{run_dir}/{service_name}.sock");

    // sun_path is typically 108 bytes on Linux, 104 on macOS
    let max_sun_path =
        std::mem::size_of::<libc::sockaddr_un>() - std::mem::size_of::<libc::sa_family_t>();

    if path.len() >= max_sun_path {
        return Err(UdsError::PathTooLong);
    }
    Ok(path)
}

/// Get SO_SNDBUF as packet size.
fn detect_packet_size(fd: RawFd) -> u32 {
    let mut val: libc::c_int = 0;
    let mut len: libc::socklen_t = std::mem::size_of::<libc::c_int>() as libc::socklen_t;

    let rc = unsafe {
        libc::getsockopt(
            fd,
            libc::SOL_SOCKET,
            libc::SO_SNDBUF,
            &mut val as *mut _ as *mut libc::c_void,
            &mut len,
        )
    };

    if rc < 0 || val <= 0 {
        DEFAULT_PACKET_SIZE_FALLBACK
    } else {
        val as u32
    }
}

/// Highest set bit in a bitmask (0 if empty).
fn highest_bit(mask: u32) -> u32 {
    if mask == 0 {
        return 0;
    }
    1u32 << (31 - mask.leading_zeros() as u32)
}

fn apply_default(val: u32, def: u32) -> u32 {
    if val == 0 {
        def
    } else {
        val
    }
}

// ---------------------------------------------------------------------------
//  Low-level I/O
// ---------------------------------------------------------------------------

/// Send header + payload as one SEQPACKET message using sendmsg.
fn raw_send_iov(fd: RawFd, hdr: &[u8], payload: &[u8]) -> Result<(), UdsError> {
    let total = hdr.len() + payload.len();

    let mut iov = [
        libc::iovec {
            iov_base: hdr.as_ptr() as *mut libc::c_void,
            iov_len: hdr.len(),
        },
        libc::iovec {
            iov_base: payload.as_ptr() as *mut libc::c_void,
            iov_len: payload.len(),
        },
    ];

    let iovcnt = if payload.is_empty() { 1 } else { 2 };

    let msg = libc::msghdr {
        msg_name: std::ptr::null_mut(),
        msg_namelen: 0,
        msg_iov: iov.as_mut_ptr(),
        msg_iovlen: iovcnt,
        msg_control: std::ptr::null_mut(),
        msg_controllen: 0,
        msg_flags: 0,
    };

    let n = unsafe { libc::sendmsg(fd, &msg, libc::MSG_NOSIGNAL) };
    if n < 0 || n as usize != total {
        return Err(UdsError::Send(errno()));
    }
    Ok(())
}

/// Send a contiguous buffer as one SEQPACKET message.
fn raw_send(fd: RawFd, data: &[u8]) -> Result<(), UdsError> {
    let n = unsafe {
        libc::send(
            fd,
            data.as_ptr() as *const libc::c_void,
            data.len(),
            libc::MSG_NOSIGNAL,
        )
    };
    if n < 0 || n as usize != data.len() {
        return Err(UdsError::Send(errno()));
    }
    Ok(())
}

/// Receive one SEQPACKET message. Returns bytes received.
fn raw_recv(fd: RawFd, buf: &mut [u8]) -> Result<usize, UdsError> {
    let n = unsafe { libc::recv(fd, buf.as_mut_ptr() as *mut libc::c_void, buf.len(), 0) };
    if n <= 0 {
        return Err(UdsError::Recv(if n == 0 { 0 } else { errno() }));
    }
    Ok(n as usize)
}

// ---------------------------------------------------------------------------
//  Socket helpers
// ---------------------------------------------------------------------------

/// Bind a Unix socket to the given path.
fn bind_unix(fd: RawFd, path: &str) -> Result<(), UdsError> {
    let c_path = CString::new(path).map_err(|_| UdsError::PathTooLong)?;

    let mut addr: libc::sockaddr_un = unsafe { std::mem::zeroed() };
    addr.sun_family = libc::AF_UNIX as libc::sa_family_t;

    let path_bytes = c_path.as_bytes_with_nul();
    let sun_path_ptr = addr.sun_path.as_mut_ptr() as *mut u8;
    unsafe {
        std::ptr::copy_nonoverlapping(path_bytes.as_ptr(), sun_path_ptr, path_bytes.len());
    }

    let rc = unsafe {
        libc::bind(
            fd,
            &addr as *const libc::sockaddr_un as *const libc::sockaddr,
            std::mem::size_of::<libc::sockaddr_un>() as libc::socklen_t,
        )
    };
    if rc < 0 {
        return Err(UdsError::Socket(errno()));
    }
    Ok(())
}

/// Connect to a Unix socket at the given path.
fn connect_unix(fd: RawFd, path: &str) -> Result<(), UdsError> {
    let c_path = CString::new(path).map_err(|_| UdsError::PathTooLong)?;

    let mut addr: libc::sockaddr_un = unsafe { std::mem::zeroed() };
    addr.sun_family = libc::AF_UNIX as libc::sa_family_t;

    let path_bytes = c_path.as_bytes_with_nul();
    let sun_path_ptr = addr.sun_path.as_mut_ptr() as *mut u8;
    unsafe {
        std::ptr::copy_nonoverlapping(path_bytes.as_ptr(), sun_path_ptr, path_bytes.len());
    }

    let rc = unsafe {
        libc::connect(
            fd,
            &addr as *const libc::sockaddr_un as *const libc::sockaddr,
            std::mem::size_of::<libc::sockaddr_un>() as libc::socklen_t,
        )
    };
    if rc < 0 {
        return Err(UdsError::Connect(errno()));
    }
    Ok(())
}

// ---------------------------------------------------------------------------
//  Stale endpoint recovery
// ---------------------------------------------------------------------------

enum StaleResult {
    NotExist,
    Stale,
    LiveServer,
}

fn check_and_recover_stale(path: &str) -> StaleResult {
    if !Path::new(path).exists() {
        return StaleResult::NotExist;
    }

    // Try connecting to see if a live server is there
    let probe = unsafe { libc::socket(libc::AF_UNIX, libc::SOCK_SEQPACKET, 0) };
    if probe < 0 {
        return StaleResult::NotExist;
    }

    let result = match connect_unix(probe, path) {
        Ok(()) => {
            // Connected => live server
            StaleResult::LiveServer
        }
        Err(UdsError::Connect(e)) if e == libc::ECONNREFUSED || e == libc::ENOENT => {
            // Connection refused or no such socket => stale, unlink
            let _ = std::fs::remove_file(path);
            StaleResult::Stale
        }
        Err(_) => {
            // Other errors (EACCES, etc.) — can't determine ownership,
            // treat as live to prevent overwriting
            StaleResult::LiveServer
        }
    };

    unsafe {
        libc::close(probe);
    }
    result
}

// ---------------------------------------------------------------------------
//  Handshake: client side
// ---------------------------------------------------------------------------

fn connect_and_handshake(
    fd: RawFd,
    path: &str,
    config: &ClientConfig,
) -> Result<UdsSession, UdsError> {
    connect_unix(fd, path)?;

    let pkt_size = if config.packet_size == 0 {
        detect_packet_size(fd)
    } else {
        config.packet_size
    };

    let supported = if config.supported_profiles == 0 {
        PROFILE_BASELINE
    } else {
        config.supported_profiles
    };

    // Build HELLO
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

    // Build outer CONTROL header
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

    // Send HELLO
    raw_send(fd, &pkt)?;

    // Receive HELLO_ACK
    let mut buf = [0u8; 128];
    let n = raw_recv(fd, &mut buf)?;

    // Decode outer header
    let ack_hdr = match Header::decode(&buf[..n]) {
        Ok(hdr) => hdr,
        Err(crate::protocol::NipcError::BadVersion) => {
            return Err(UdsError::Incompatible("ack header version mismatch".into()))
        }
        Err(e) => return Err(UdsError::Protocol(format!("ack header: {e}"))),
    };

    if ack_hdr.kind != protocol::KIND_CONTROL || ack_hdr.code != protocol::CODE_HELLO_ACK {
        return Err(UdsError::Protocol("expected HELLO_ACK".into()));
    }

    // Check transport_status for rejection
    if ack_hdr.transport_status == protocol::STATUS_AUTH_FAILED {
        return Err(UdsError::AuthFailed);
    }
    if ack_hdr.transport_status == protocol::STATUS_UNSUPPORTED {
        return Err(UdsError::NoProfile);
    }
    if ack_hdr.transport_status == protocol::STATUS_INCOMPATIBLE {
        return Err(UdsError::Incompatible(
            "ack transport_status incompatible".into(),
        ));
    }
    if ack_hdr.transport_status == protocol::STATUS_LIMIT_EXCEEDED {
        return Err(UdsError::LimitExceeded);
    }
    if ack_hdr.transport_status != protocol::STATUS_OK {
        return Err(UdsError::Handshake(format!(
            "transport_status={}",
            ack_hdr.transport_status
        )));
    }

    // Decode hello-ack payload
    if n < HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE {
        return Err(UdsError::Protocol("ack payload truncated".into()));
    }
    let ack = match HelloAck::decode(&buf[HEADER_SIZE..n]) {
        Ok(ack) => ack,
        Err(crate::protocol::NipcError::BadLayout)
            if hello_ack_layout_incompatible(&buf[HEADER_SIZE..n]) =>
        {
            return Err(UdsError::Incompatible(
                "ack payload layout version mismatch".into(),
            ))
        }
        Err(e) => return Err(UdsError::Protocol(format!("ack payload: {e}"))),
    };

    // Sanity: reject a packet_size too small for chunking arithmetic
    if ack.agreed_packet_size <= HEADER_SIZE as u32 {
        return Err(UdsError::Protocol("agreed packet_size too small".into()));
    }

    Ok(UdsSession {
        fd,
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
        inflight_ids: HashSet::new(),
    })
}

// ---------------------------------------------------------------------------
//  Handshake: server side
// ---------------------------------------------------------------------------

fn server_handshake(
    fd: RawFd,
    config: &ServerConfig,
    session_id: u64,
) -> Result<UdsSession, UdsError> {
    let server_pkt_size = if config.packet_size == 0 {
        detect_packet_size(fd)
    } else {
        config.packet_size
    };

    let s_resp_pay = apply_default(config.max_response_payload_bytes, MAX_PAYLOAD_DEFAULT);
    let s_profiles = if config.supported_profiles == 0 {
        PROFILE_BASELINE
    } else {
        config.supported_profiles
    };
    let s_preferred = config.preferred_profiles;

    let send_rejection = |status: u16| -> Result<(), UdsError> {
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
        let _ = raw_send(fd, &pkt);
        Ok(())
    };

    // Receive HELLO
    let mut buf = [0u8; 128];
    let n = raw_recv(fd, &mut buf)?;

    let hdr = match Header::decode(&buf[..n]) {
        Ok(hdr) => hdr,
        Err(crate::protocol::NipcError::BadVersion)
            if header_version_incompatible(&buf[..n], protocol::CODE_HELLO) =>
        {
            send_rejection(protocol::STATUS_INCOMPATIBLE)?;
            return Err(UdsError::Incompatible(
                "hello header version mismatch".into(),
            ));
        }
        Err(e) => return Err(UdsError::Protocol(format!("hello header: {e}"))),
    };

    if hdr.kind != protocol::KIND_CONTROL || hdr.code != protocol::CODE_HELLO {
        return Err(UdsError::Protocol("expected HELLO".into()));
    }

    let hello = match Hello::decode(&buf[HEADER_SIZE..n]) {
        Ok(hello) => hello,
        Err(crate::protocol::NipcError::BadLayout)
            if hello_layout_incompatible(&buf[HEADER_SIZE..n]) =>
        {
            send_rejection(protocol::STATUS_INCOMPATIBLE)?;
            return Err(UdsError::Incompatible(
                "hello payload layout version mismatch".into(),
            ));
        }
        Err(e) => return Err(UdsError::Protocol(format!("hello payload: {e}"))),
    };

    // Compute intersection
    let intersection = hello.supported_profiles & s_profiles;

    // Check intersection
    if intersection == 0 {
        send_rejection(protocol::STATUS_UNSUPPORTED)?;
        return Err(UdsError::NoProfile);
    }

    // Check auth
    if hello.auth_token != config.auth_token {
        send_rejection(protocol::STATUS_AUTH_FAILED)?;
        return Err(UdsError::AuthFailed);
    }

    // Select profile
    let preferred_intersection = intersection & hello.preferred_profiles & s_preferred;
    let selected = if preferred_intersection != 0 {
        highest_bit(preferred_intersection)
    } else {
        highest_bit(intersection)
    };

    if hello.max_request_payload_bytes > MAX_PAYLOAD_CAP {
        send_rejection(protocol::STATUS_LIMIT_EXCEEDED)?;
        return Err(UdsError::LimitExceeded);
    }

    // Negotiate limits:
    // - request payload and batch size are client-proposed and echoed
    // - response payload is server-authoritative
    // - response batch size is symmetric with request batch size
    let agreed_req_pay = hello.max_request_payload_bytes;
    let agreed_req_bat = hello.max_request_batch_items;
    let agreed_resp_pay = s_resp_pay;
    let agreed_resp_bat = agreed_req_bat;
    let agreed_pkt = hello.packet_size.min(server_pkt_size);

    // packet_size must be large enough for a usable message packet
    if agreed_pkt <= HEADER_SIZE as u32 {
        send_rejection(protocol::STATUS_INCOMPATIBLE)?;
        return Err(UdsError::Incompatible(
            "packet size too small for negotiated session".into(),
        ));
    }

    // Send HELLO_ACK (success)
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
    raw_send(fd, &pkt)?;

    Ok(UdsSession {
        fd,
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
        inflight_ids: HashSet::new(),
    })
}

// ---------------------------------------------------------------------------
//  Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
#[path = "posix_tests.rs"]
mod tests;
