//! L1 POSIX UDS transport.
//!
//! Connection lifecycle, handshake with profile/limit negotiation,
//! and send/receive with transparent chunking over AF_UNIX SEQPACKET sockets
//! (a stream socket carrying the same packets on macOS and FreeBSD, see
//! `posix_stream.rs`). Wire-compatible with the C implementation in
//! netipc_uds.c.

use super::framing::{self, Framing, FramingError, ServerOffer, apply_default};
use crate::protocol::{
    self, HEADER_SIZE, Header, Hello, HelloAck, MAX_PAYLOAD_DEFAULT, PROFILE_BASELINE,
};
use std::collections::HashSet;
use std::ffi::CString;
use std::io;
use std::os::unix::io::RawFd;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::time::{Duration, Instant};

// ---------------------------------------------------------------------------
//  Constants
// ---------------------------------------------------------------------------

const DEFAULT_BACKLOG: i32 = 16;
const DEFAULT_BATCH_ITEMS: u32 = 1;
#[cfg_attr(any(target_os = "macos", target_os = "freebsd"), allow(dead_code))]
const DEFAULT_PACKET_SIZE_FALLBACK: u32 = 65536;
const PACKET_SIZE_MAX: u32 = 262_144;
/// Bytes Linux keeps from SO_SNDBUF for a SEQPACKET datagram: it accepts
/// SO_SNDBUF - 32.
#[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
const SNDBUF_OVERHEAD: u32 = 32;

/// macOS has no AF_UNIX SOCK_SEQPACKET, and FreeBSD 15 SEQPACKET keeps record
/// boundaries only where the sender sets MSG_EOR and can write part of a
/// record. Both use a stream socket carrying the same packets
/// (docs/level1-posix-uds.md, "Stream baseline").
#[cfg(any(target_os = "macos", target_os = "freebsd"))]
pub(crate) const SOCK_TYPE: libc::c_int = libc::SOCK_STREAM;
#[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
pub(crate) const SOCK_TYPE: libc::c_int = libc::SOCK_SEQPACKET;

const SEND_FLAGS: libc::c_int = libc::MSG_NOSIGNAL;

#[path = "posix_stream.rs"]
#[cfg_attr(not(any(target_os = "macos", target_os = "freebsd")), allow(dead_code))]
mod stream;

/// Profiles this platform can provide; unassigned bits are never offered or
/// accepted. POSIX SHM needs a futex-style wait (Linux futex, FreeBSD
/// `_umtx_op`), so other POSIX systems offer and accept the baseline
/// profile only.
#[cfg(any(target_os = "linux", target_os = "freebsd"))]
const PLATFORM_PROFILES: u32 =
    PROFILE_BASELINE | protocol::PROFILE_SHM_HYBRID | protocol::PROFILE_SHM_FUTEX;
#[cfg(not(any(target_os = "linux", target_os = "freebsd")))]
const PLATFORM_PROFILES: u32 = PROFILE_BASELINE;

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
    /// Receive timed out before a complete message arrived.
    Timeout,
    /// Receive was aborted by the caller.
    Aborted,
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
            UdsError::Timeout => write!(f, "receive timed out"),
            UdsError::Aborted => write!(f, "receive aborted"),
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

impl FramingError for UdsError {
    fn protocol(msg: String) -> Self {
        UdsError::Protocol(msg)
    }
    fn chunk(msg: String) -> Self {
        UdsError::Chunk(msg)
    }
    fn incompatible(msg: String) -> Self {
        UdsError::Incompatible(msg)
    }
    fn handshake(msg: String) -> Self {
        UdsError::Handshake(msg)
    }
    fn bad_param(msg: String) -> Self {
        UdsError::BadParam(msg)
    }
    fn limit_exceeded() -> Self {
        UdsError::LimitExceeded
    }
    fn auth_failed() -> Self {
        UdsError::AuthFailed
    }
    fn no_profile() -> Self {
        UdsError::NoProfile
    }
    fn duplicate_msg_id(id: u64) -> Self {
        UdsError::DuplicateMsgId(id)
    }
    fn unknown_msg_id(id: u64) -> Self {
        UdsError::UnknownMsgId(id)
    }
    fn fails_session(&self) -> bool {
        matches!(self, UdsError::Send(_))
    }
    fn is_recv_failure(&self) -> bool {
        matches!(self, UdsError::Recv(_))
    }
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
    /// 0 = transport limit (SEQPACKET: SO_SNDBUF - 32; stream: 256 KiB);
    /// always capped at 256 KiB.
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
    /// 0 = transport limit (SEQPACKET: SO_SNDBUF - 32; stream: 256 KiB);
    /// always capped at 256 KiB.
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

    // Inbound packets read while a pipelined send waited for buffer space
    pending: Pending,

    // Stream baseline: a receive failed after reading part of a message, so
    // the stream is off a packet boundary and every later receive fails.
    #[cfg_attr(not(any(target_os = "macos", target_os = "freebsd")), allow(dead_code))]
    recv_failed: bool,
}

impl UdsSession {
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

    /// Get the raw fd for poll/epoll integration. A pipelined send can read
    /// responses into the session's pending queue, where the fd no longer
    /// shows them: check [`Self::has_pending`] before waiting on the fd.
    pub fn fd(&self) -> RawFd {
        self.fd
    }

    /// True when the next receive can return data read earlier by a
    /// pipelined send, without the fd becoming readable.
    pub fn has_pending(&self) -> bool {
        !self.recv_failed && !self.pending.is_empty()
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
        Self::connect_with_timeout(run_dir, service_name, config, 0, None)
    }

    /// `connect()` bounded by `timeout_ms`: connect and the wait for
    /// HELLO_ACK together end with `UdsError::Timeout` after `timeout_ms`
    /// (0 = no deadline), and with `UdsError::Aborted` once `abort` is set.
    pub fn connect_with_timeout(
        run_dir: &str,
        service_name: &str,
        config: &ClientConfig,
        timeout_ms: u32,
        abort: Option<&AtomicBool>,
    ) -> Result<Self, UdsError> {
        let wait = ReceiveWait::new(timeout_ms, abort);
        let path = build_socket_path(run_dir, service_name)?;

        let fd = uds_socket_cloexec();
        if fd < 0 {
            return Err(UdsError::Socket(errno()));
        }

        let result = connect_and_handshake(fd, &path, config, &wait);
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
        self.send_with_control(hdr, payload, 0, None)
    }

    /// `send` with an optional deadline and abort flag for a client session.
    ///
    /// `timeout_ms == 0` waits without a deadline. While the socket lacks
    /// buffer space the send reads the peer's bytes as a pipelined send
    /// does. If the deadline passes or `abort` is set before the message is
    /// sent, returns `UdsError::Timeout` or `UdsError::Aborted` and shuts
    /// the session down, since part of the message may be on the wire.
    pub fn send_with_control(
        &mut self,
        hdr: &mut Header,
        payload: &[u8],
        timeout_ms: u32,
        abort: Option<&AtomicBool>,
    ) -> Result<(), UdsError> {
        if self.fd < 0 {
            return Err(UdsError::BadParam("session closed".into()));
        }

        let framing = self.framing(true);
        let fd = self.fd;

        // A client send with a deadline or abort, or with another request in
        // flight (a pipelining client can deadlock with a server blocked
        // sending to it), waits in the progress loop; anything else keeps the
        // plain blocking send.
        let controlled = timeout_ms != 0 || abort.is_some();
        let inflight = self.inflight_ids.len()
            + usize::from(framing.client && hdr.kind == crate::protocol::KIND_REQUEST);
        if !(framing.client && (inflight > 1 || controlled)) {
            return framing::send_message(framing, &mut self.inflight_ids, hdr, payload, |h, p| {
                send_packet(fd, h, p)
            });
        }

        // A bounded send of an untracked message can run with nothing in
        // flight; it still gets one response's room.
        let bound = pending_bound(
            self.max_response_payload_bytes,
            self.packet_size,
            inflight.max(1),
        );
        let read_len = self.packet_size as usize;
        let pending = &mut self.pending;
        // After a failed stream receive the inbound bytes belong to an
        // abandoned message: never queue them.
        let recv_failed = self.recv_failed;
        let wait = controlled.then(|| ReceiveWait::new(timeout_ms, abort));
        let mut failed = false;
        let result =
            framing::send_message(framing, &mut self.inflight_ids, hdr, payload, |h, p| {
                let r = send_with_progress(
                    fd,
                    h,
                    p,
                    pending,
                    read_len,
                    bound,
                    recv_failed,
                    wait.as_ref(),
                );
                if matches!(
                    r,
                    Err(UdsError::Protocol(_) | UdsError::Timeout | UdsError::Aborted)
                ) {
                    failed = true;
                }
                r
            });
        if failed {
            // Inbound bytes were dropped, or the message stopped part way:
            // the session cannot continue.
            unsafe { libc::shutdown(fd, libc::SHUT_RDWR) };
            self.inflight_ids.clear();
        }
        result
    }

    /// Receive one logical message. Blocks until a complete message arrives.
    ///
    /// On success, returns (header, payload_view). The payload view is valid
    /// until the next receive call on this session.
    pub fn receive<'a>(&'a mut self, buf: &'a mut [u8]) -> Result<(Header, &'a [u8]), UdsError> {
        self.receive_with_control(buf, 0, None)
    }

    /// Receive one logical message with an optional deadline and abort flag.
    ///
    /// `timeout_ms == 0` waits indefinitely. When `abort` is set before a
    /// complete message arrives, the call returns `UdsError::Aborted`.
    pub fn receive_with_control<'a>(
        &'a mut self,
        buf: &'a mut [u8],
        timeout_ms: u32,
        abort: Option<&AtomicBool>,
    ) -> Result<(Header, &'a [u8]), UdsError> {
        if self.fd < 0 {
            return Err(UdsError::BadParam("session closed".into()));
        }
        // Rejected before any byte is read, so the session stays usable.
        if buf.len() < HEADER_SIZE {
            return Err(UdsError::BadParam("receive buffer too small".into()));
        }

        let controlled = timeout_ms != 0 || abort.is_some();
        #[cfg_attr(any(target_os = "macos", target_os = "freebsd"), allow(unused_mut))]
        let mut wait = ReceiveWait::new(timeout_ms, abort);

        let framing = self.framing(false);
        let fd = self.fd;

        #[cfg(any(target_os = "macos", target_os = "freebsd"))]
        {
            // A failure before the first byte (a timeout or abort, also one
            // that fires after the wait below) leaves the stream on a packet
            // boundary. A failure after it does not: the rest of the message
            // is still queued, so every later receive fails. The
            // session records that instead of shutting the read side, whose
            // effect differs by OS (FreeBSD 15 fails the peer's sends,
            // FreeBSD 14 still returns queued bytes). Sends stay valid; they
            // always write whole messages.
            if self.recv_failed {
                return Err(UdsError::Recv(0));
            }
            let wait = controlled.then_some(&wait);
            if let Some(wait) = wait
                && self.pending.is_empty()
            {
                wait_fd_readable(fd, wait)?;
            }
            let pending = &mut self.pending;
            let mut first = true;
            let mut consumed = false;
            let result = framing::receive_message(
                framing,
                &mut self.inflight_ids,
                buf,
                &mut self.recv_buf,
                &mut self.pkt_buf,
                |b| {
                    let r = stream::recv_session_packet(
                        fd,
                        pending,
                        b,
                        first,
                        framing.packet_size,
                        framing.max_payload,
                        framing.max_items,
                        wait,
                        &mut consumed,
                    );
                    first = false;
                    r
                },
            );
            if result.is_err() && consumed {
                self.recv_failed = true;
                // Queued bytes are part of the abandoned stream too, and no
                // response for a request in flight can arrive any more.
                self.pending = Pending::default();
                self.inflight_ids.clear();
            }
            result
        }

        #[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
        framing::receive_message(
            framing,
            &mut self.inflight_ids,
            buf,
            &mut self.recv_buf,
            &mut self.pkt_buf,
            |b| {
                if self.pending.is_empty() {
                    raw_recv_control(fd, b, controlled, &mut wait)
                } else {
                    Ok(self.pending.pop_packet(b))
                }
            },
        )
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
    /// `{service_name}.lock`, flock'ed for the listener's lifetime. Fields
    /// drop after `Drop::drop` removed the socket file, so the endpoint is
    /// released only once its socket file is gone.
    _lock: EndpointLock,
}

/// Test-only hook run inside `UdsListener::bind` between bind() and
/// listen() for the given socket path.
#[cfg(test)]
pub(crate) type ListenHook = (String, Box<dyn Fn() + Send>);
#[cfg(test)]
pub(crate) static LISTEN_HOOK: std::sync::Mutex<Option<ListenHook>> = std::sync::Mutex::new(None);

/// Runs just before a new endpoint lock file is linked to its final name.
#[cfg(test)]
pub(crate) type LockPublishHook = Box<dyn Fn(&str) + Send>;
#[cfg(test)]
pub(crate) static LOCK_PUBLISH_HOOK: std::sync::Mutex<Option<LockPublishHook>> =
    std::sync::Mutex::new(None);

/// A file created under a umask without owner bits stays unopenable if the
/// process dies before fchmod(), so a missing lock gets its name only once
/// it is 0600. Best effort: the caller's creating open covers every failure,
/// such as a filesystem without hard links, and a losing link means another
/// starter published the file first.
fn publish_endpoint_lock(lock_path: &str) {
    use std::os::unix::fs::{OpenOptionsExt, PermissionsExt};

    static SEQ: AtomicU64 = AtomicU64::new(0);
    let tmp = format!(
        "{lock_path}.{}.{}.tmp",
        std::process::id(),
        SEQ.fetch_add(1, Ordering::Relaxed)
    );
    let Ok(file) = std::fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(0o600)
        .custom_flags(libc::O_CLOEXEC | libc::O_NOFOLLOW)
        .open(&tmp)
    else {
        return;
    };
    let restricted = file
        .set_permissions(std::fs::Permissions::from_mode(0o600))
        .is_ok();
    drop(file);
    if restricted {
        #[cfg(test)]
        if let Some(hook) = LOCK_PUBLISH_HOOK.lock().unwrap().as_ref() {
            hook(&tmp);
        }
        let _ = std::fs::hard_link(&tmp, lock_path);
    }
    let _ = std::fs::remove_file(&tmp);
}

/// Take the endpoint lock `{run_dir}/{service_name}.lock`. A server's
/// socket file exists from bind() until listen(), and connects in that
/// window are refused just as for a dead server's leftover file, so a
/// connect probe alone cannot tell a starting server from a stale one. The
/// lock can: its holder owns the endpoint, and the kernel drops it when that
/// process dies. The file stays on disk; removing it would let a starter
/// that already opened it lock an orphaned inode.
fn acquire_endpoint_lock(run_dir: &str, service_name: &str) -> Result<EndpointLock, UdsError> {
    use std::os::unix::fs::OpenOptionsExt;
    use std::os::unix::io::AsRawFd;

    use std::os::unix::fs::PermissionsExt;

    let lock_path = format!("{run_dir}/{service_name}.lock");
    // A file left with mode bits missing, by an older build or a fallback
    // create under a umask without owner read or write, may not reopen
    // read-write, so a narrower open follows (flock() works on any access
    // mode) and fchmod() restores 0600 for the next starter. fchmod() fails
    // only for a file another account owns, which the lock still works on.
    let open_mode = |read: bool, write: bool, create: bool| {
        std::fs::OpenOptions::new()
            .read(read)
            .write(write)
            .create(create)
            .truncate(false)
            .mode(0o600)
            .custom_flags(libc::O_CLOEXEC | libc::O_NOFOLLOW)
            .open(&lock_path)
    };
    let denied = |e: &io::Error| e.raw_os_error() == Some(libc::EACCES);
    let file = open_mode(true, true, false)
        .or_else(|e| {
            if e.raw_os_error() == Some(libc::ENOENT) {
                publish_endpoint_lock(&lock_path);
                open_mode(true, true, true)
            } else {
                Err(e)
            }
        })
        .or_else(|e| {
            if denied(&e) {
                open_mode(false, true, false)
            } else {
                Err(e)
            }
        })
        .or_else(|e| {
            if denied(&e) {
                open_mode(true, false, false)
            } else {
                Err(e)
            }
        })
        .map_err(|e| UdsError::Socket(e.raw_os_error().unwrap_or(0)))?;
    let _ = file.set_permissions(std::fs::Permissions::from_mode(0o600));
    loop {
        if unsafe { libc::flock(file.as_raw_fd(), libc::LOCK_EX | libc::LOCK_NB) } == 0 {
            return Ok(EndpointLock(file));
        }
        match errno() {
            libc::EINTR => continue,
            libc::EWOULDBLOCK => return Err(UdsError::AddrInUse),
            e => return Err(UdsError::Socket(e)),
        }
    }
}

/// The held endpoint lock. Dropping it unlocks before closing: a child
/// forked by another thread shares the open file description until it
/// execs, and its copy would keep the lock held.
struct EndpointLock(std::fs::File);

impl Drop for EndpointLock {
    fn drop(&mut self) {
        use std::os::unix::io::AsRawFd;
        unsafe { libc::flock(self.0.as_raw_fd(), libc::LOCK_UN) };
    }
}

impl UdsListener {
    /// Create a listener on `{run_dir}/{service_name}.sock`.
    /// First takes an exclusive flock on `{run_dir}/{service_name}.lock`,
    /// held until the listener drops; a held lock means another server owns
    /// the endpoint (`AddrInUse`). With the lock held, an existing socket
    /// file no live server accepts on is stale and is replaced.
    pub fn bind(run_dir: &str, service_name: &str, config: ServerConfig) -> Result<Self, UdsError> {
        let path = build_socket_path(run_dir, service_name)?;
        let lock = acquire_endpoint_lock(run_dir, service_name)?;

        // The probe still guards against a live server that takes no lock
        // (an older library version).
        match check_and_recover_stale(&path) {
            StaleResult::LiveServer => return Err(UdsError::AddrInUse),
            StaleResult::Stale | StaleResult::NotExist => { /* proceed */ }
        }

        let fd = uds_socket_cloexec();
        if fd < 0 {
            return Err(UdsError::Socket(errno()));
        }

        if let Err(e) = bind_owner_only(fd, run_dir, service_name, &path) {
            unsafe {
                libc::close(fd);
            }
            return Err(e);
        }

        #[cfg(test)]
        {
            // Run the hook once, outside the lock: other binds go on.
            let hook = {
                let mut slot = LISTEN_HOOK.lock().unwrap();
                match slot.as_ref() {
                    Some((hook_path, _)) if *hook_path == path => slot.take(),
                    _ => None,
                }
            };
            if let Some((_, hook)) = hook {
                hook();
            }
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
            _lock: lock,
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
    /// and session ID. The handshake waits at most
    /// `SERVER_HANDSHAKE_TIMEOUT_MS` for HELLO.
    pub fn accept_with_config(
        &self,
        session_id: u64,
        config: ServerConfig,
    ) -> Result<UdsSession, UdsError> {
        self.accept_pending()?.handshake(
            session_id,
            &config,
            protocol::SERVER_HANDSHAKE_TIMEOUT_MS,
            None,
        )
    }

    /// Accept one connection without handshaking, so the caller can run the
    /// handshake on another thread. Blocks until a client connects.
    pub fn accept_pending(&self) -> Result<PendingConnection, UdsError> {
        // A signal that interrupts the wait is retried: it must not end the
        // server's accept loop.
        let fd = retry_eintr(
            #[cfg(test)]
            &eintr_inject::ACCEPT,
            || accept_cloexec(self.fd) as isize,
        )
        .map_err(UdsError::Accept)?;
        Ok(PendingConnection { fd: fd as RawFd })
    }
}

/// An accepted connection whose handshake has not run yet. Dropping it
/// closes the connection.
#[derive(Debug)]
pub struct PendingConnection {
    fd: RawFd,
}

impl PendingConnection {
    /// Complete the server side of the handshake. Waits at most `timeout_ms`
    /// for HELLO (`UdsError::Timeout`; 0 waits without a deadline) and returns
    /// `UdsError::Aborted` once `abort` is set.
    pub fn handshake(
        self,
        session_id: u64,
        config: &ServerConfig,
        timeout_ms: u32,
        abort: Option<&AtomicBool>,
    ) -> Result<UdsSession, UdsError> {
        self.handshake_inner(session_id, config, timeout_ms, abort, None)
    }

    /// [`handshake`](Self::handshake) with a prepare step. Once a profile is
    /// selected and the payload limits are agreed, and before HELLO_ACK is
    /// sent, `prepare(profile, max_request_payload_bytes,
    /// max_response_payload_bytes)` sets up that profile's resources (an SHM
    /// region). Returning false drops the profile and the handshake selects
    /// again; with no profile left the client is rejected as unsupported.
    pub fn handshake_prepared(
        self,
        session_id: u64,
        config: &ServerConfig,
        timeout_ms: u32,
        abort: Option<&AtomicBool>,
        prepare: &mut dyn FnMut(u32, u32, u32) -> bool,
    ) -> Result<UdsSession, UdsError> {
        self.handshake_inner(session_id, config, timeout_ms, abort, Some(prepare))
    }

    fn handshake_inner(
        self,
        session_id: u64,
        config: &ServerConfig,
        timeout_ms: u32,
        abort: Option<&AtomicBool>,
        prepare: Option<&mut dyn FnMut(u32, u32, u32) -> bool>,
    ) -> Result<UdsSession, UdsError> {
        // One deadline covers the wait for HELLO and, on a stream, its bytes.
        let wait = ReceiveWait::new(timeout_ms, abort);
        wait_fd_readable(self.fd, &wait)?;
        let session = server_handshake(self.fd, config, session_id, prepare, &wait)?;
        // The session owns the descriptor now.
        std::mem::forget(self);
        Ok(session)
    }
}

impl Drop for PendingConnection {
    fn drop(&mut self) {
        unsafe {
            libc::close(self.fd);
        }
    }
}

impl UdsListener {
    /// Stop accepting: close the listening socket and remove the socket
    /// file, keeping the endpoint lock until the listener drops, so no other
    /// server takes the endpoint while this one still serves its sessions.
    /// Idempotent.
    pub fn stop_listening(&mut self) {
        if self.fd >= 0 {
            unsafe {
                libc::close(self.fd);
            }
            self.fd = -1;
        }
        if !self.path.as_os_str().is_empty() {
            let _ = std::fs::remove_file(&self.path);
            self.path = PathBuf::new();
        }
    }
}

impl Drop for UdsListener {
    fn drop(&mut self) {
        self.stop_listening();
    }
}

// ---------------------------------------------------------------------------
//  Internal helpers
// ---------------------------------------------------------------------------

// SO_NOSIGPIPE is macOS's documented way to keep a write to a closed peer
// from raising SIGPIPE in the embedding process; sends also pass
// MSG_NOSIGNAL.
#[cfg(target_os = "macos")]
fn set_nosigpipe(fd: i32) {
    let on: libc::c_int = 1;
    unsafe {
        libc::setsockopt(
            fd,
            libc::SOL_SOCKET,
            libc::SO_NOSIGPIPE,
            &on as *const _ as *const libc::c_void,
            std::mem::size_of::<libc::c_int>() as libc::socklen_t,
        )
    };
}

// Sockets must not leak into children a consumer forks and execs: an
// inherited session socket keeps the peer from ever seeing EOF, which is
// the dead-peer signal. Platforms without SOCK_CLOEXEC set the flag right
// after creation.
#[cfg(any(target_os = "linux", target_os = "android", target_os = "freebsd"))]
fn uds_socket_cloexec() -> i32 {
    unsafe { libc::socket(libc::AF_UNIX, SOCK_TYPE | libc::SOCK_CLOEXEC, 0) }
}

#[cfg(not(any(target_os = "linux", target_os = "android", target_os = "freebsd")))]
fn uds_socket_cloexec() -> i32 {
    let fd = unsafe { libc::socket(libc::AF_UNIX, SOCK_TYPE, 0) };
    if fd >= 0 {
        unsafe { libc::fcntl(fd, libc::F_SETFD, libc::FD_CLOEXEC) };
        #[cfg(target_os = "macos")]
        set_nosigpipe(fd);
    }
    fd
}

#[cfg(any(target_os = "linux", target_os = "android", target_os = "freebsd"))]
fn accept_cloexec(listen_fd: i32) -> i32 {
    unsafe {
        libc::accept4(
            listen_fd,
            std::ptr::null_mut(),
            std::ptr::null_mut(),
            libc::SOCK_CLOEXEC,
        )
    }
}

#[cfg(not(any(target_os = "linux", target_os = "android", target_os = "freebsd")))]
fn accept_cloexec(listen_fd: i32) -> i32 {
    let fd = unsafe { libc::accept(listen_fd, std::ptr::null_mut(), std::ptr::null_mut()) };
    if fd >= 0 {
        unsafe { libc::fcntl(fd, libc::F_SETFD, libc::FD_CLOEXEC) };
        #[cfg(target_os = "macos")]
        set_nosigpipe(fd);
    }
    fd
}

/// Bind as owner-only (0600, narrowed by the umask). Linux and FreeBSD give
/// the socket file the mode of the unbound socket, so fchmod() before bind()
/// sets it without a post-bind window and without touching the
/// process-global umask, which other threads of the embedding process use.
/// connect() needs write permission on the socket file, so a umask without
/// owner write fails the listen instead of leaving an endpoint its own user
/// cannot reach.
#[cfg(not(target_os = "macos"))]
fn bind_owner_only(
    fd: RawFd,
    _run_dir: &str,
    _service_name: &str,
    path: &str,
) -> Result<(), UdsError> {
    use std::os::unix::fs::PermissionsExt;

    if unsafe { libc::fchmod(fd, libc::S_IRUSR | libc::S_IWUSR) } < 0 {
        return Err(UdsError::Socket(errno()));
    }
    bind_unix(fd, path)?;
    let err = match std::fs::symlink_metadata(path) {
        Ok(meta) if meta.permissions().mode() & 0o200 != 0 => return Ok(()),
        Ok(_) => libc::EACCES,
        Err(e) => e.raw_os_error().unwrap_or(libc::EIO),
    };
    let _ = std::fs::remove_file(path);
    Err(UdsError::Socket(err))
}

/// macOS rejects fchmod() on an unbound socket and creates the socket file
/// 0777 & ~umask. The socket is bound inside a private 0700 directory
/// `{service_name}.d`, narrowed to owner-only there, then renamed into
/// place, so it is never reachable with a wider mode and the process-global
/// umask is untouched. The caller holds the endpoint lock, so a leftover
/// directory belongs to a crashed server and is removed first. fchmodat()
/// and open() resolve the directory by path without following a symlink,
/// and the owner check on the opened descriptor refuses a directory another
/// account swapped in. After that check every step but bind() goes through
/// the descriptor, so a later swap makes the listen fail instead of placing
/// a socket someone else can reach.
#[cfg(target_os = "macos")]
fn bind_owner_only(
    fd: RawFd,
    run_dir: &str,
    service_name: &str,
    path: &str,
) -> Result<(), UdsError> {
    use std::os::unix::fs::{DirBuilderExt, MetadataExt, OpenOptionsExt, PermissionsExt};
    use std::os::unix::io::AsRawFd;

    let io_err = |e: io::Error| UdsError::Socket(e.raw_os_error().unwrap_or(libc::EIO));
    let dir = format!("{run_dir}/{service_name}.d");
    let tmp = format!("{dir}/s");

    // Anything but a real directory is unlinked without following it.
    match std::fs::symlink_metadata(&dir) {
        Ok(meta) if meta.is_dir() => {
            match std::fs::remove_file(&tmp) {
                Ok(()) => {}
                Err(e) if e.kind() == io::ErrorKind::NotFound => {}
                Err(e) => return Err(io_err(e)),
            }
            std::fs::remove_dir(&dir).map_err(io_err)?;
        }
        Ok(_) => std::fs::remove_file(&dir).map_err(io_err)?,
        Err(e) if e.kind() == io::ErrorKind::NotFound => {}
        Err(e) => return Err(io_err(e)),
    }

    std::fs::DirBuilder::new()
        .mode(0o700)
        .create(&dir)
        .map_err(io_err)?;
    let result = (|| {
        // fchmodat() ignores the umask, so a umask without owner bits cannot
        // make the directory unopenable or unusable.
        let c_dir = CString::new(dir.as_str()).map_err(|_| UdsError::PathTooLong)?;
        if unsafe {
            libc::fchmodat(
                libc::AT_FDCWD,
                c_dir.as_ptr(),
                0o700,
                libc::AT_SYMLINK_NOFOLLOW,
            )
        } < 0
        {
            return Err(UdsError::Socket(errno()));
        }
        let dir_file = std::fs::OpenOptions::new()
            .read(true)
            .custom_flags(libc::O_DIRECTORY | libc::O_NOFOLLOW | libc::O_CLOEXEC)
            .open(&dir)
            .map_err(io_err)?;
        if dir_file.metadata().map_err(io_err)?.uid() != unsafe { libc::geteuid() } {
            return Err(UdsError::Socket(libc::EPERM));
        }
        // The descriptor is the directory every later step uses; keep it
        // 0700 even if another one of ours replaced the path.
        dir_file
            .set_permissions(std::fs::Permissions::from_mode(0o700))
            .map_err(io_err)?;
        bind_unix(fd, &tmp)?;
        let dfd = dir_file.as_raw_fd();
        let name = c"s";
        let placed = (|| {
            // The bound file carries 0777 & ~umask. It is set to exactly
            // 0600: a umask without owner write would leave a socket its own
            // user cannot connect to, and only the owner can reach the
            // private directory.
            let mode = libc::S_IRUSR | libc::S_IWUSR;
            if unsafe { libc::fchmodat(dfd, name.as_ptr(), mode, 0) } < 0 {
                return Err(UdsError::Socket(errno()));
            }
            let c_path = CString::new(path).map_err(|_| UdsError::PathTooLong)?;
            if unsafe { libc::renameat(dfd, name.as_ptr(), libc::AT_FDCWD, c_path.as_ptr()) } < 0 {
                return Err(UdsError::Socket(errno()));
            }
            Ok(())
        })();
        if placed.is_err() {
            unsafe { libc::unlinkat(dfd, name.as_ptr(), 0) };
        }
        placed
    })();
    let _ = std::fs::remove_dir(&dir);
    result
}

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
                )));
            }
        }
    }
    Ok(())
}

/// Build `{run_dir}/{service_name}.sock` and validate length.
fn build_socket_path(run_dir: &str, service_name: &str) -> Result<String, UdsError> {
    // An empty run_dir would put the socket at "/<name>".
    if run_dir.is_empty() {
        return Err(UdsError::BadParam("empty run_dir".into()));
    }
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

/// Packet size this side may propose: `configured` (0 = no preference)
/// capped by what the kernel accepts as one datagram.
///
/// Linux rejects a datagram larger than SO_SNDBUF minus 32 bytes of
/// accounting overhead with EMSGSIZE; very large buffers also hit ENOBUFS
/// near 1 MiB, hence the fixed cap.
#[cfg(any(target_os = "macos", target_os = "freebsd"))]
fn packet_size_limit(_fd: RawFd, configured: u32) -> u32 {
    // A stream has no per-message size cap; the fixed cap keeps the first
    // packet's buffer bounded and the chunking code shared.
    if configured != 0 && configured < PACKET_SIZE_MAX {
        configured
    } else {
        PACKET_SIZE_MAX
    }
}

#[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
fn packet_size_limit(fd: RawFd, configured: u32) -> u32 {
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

    let limit = if rc < 0 || val <= SNDBUF_OVERHEAD as libc::c_int {
        DEFAULT_PACKET_SIZE_FALLBACK
    } else {
        val as u32 - SNDBUF_OVERHEAD
    }
    .min(PACKET_SIZE_MAX);

    if configured != 0 && configured < limit {
        configured
    } else {
        limit
    }
}

// ---------------------------------------------------------------------------
//  Low-level I/O
// ---------------------------------------------------------------------------

/// Send one packet: one SEQPACKET message, or all its bytes on a stream.
fn send_packet(fd: RawFd, hdr: &[u8], payload: &[u8]) -> Result<(), UdsError> {
    #[cfg(any(target_os = "macos", target_os = "freebsd"))]
    return stream::send_all(fd, hdr, payload);
    #[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
    raw_send_iov(fd, hdr, payload)
}

/// Send one contiguous handshake packet.
fn send_control(fd: RawFd, pkt: &[u8]) -> Result<(), UdsError> {
    #[cfg(any(target_os = "macos", target_os = "freebsd"))]
    return stream::send_all(fd, pkt, &[]);
    #[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
    raw_send(fd, pkt)
}

/// Inbound bytes a pipelined client send read while it waited for buffer
/// space; receive consumes them before reading the socket. SEQPACKET stores
/// each datagram as a 4-byte length record followed by its bytes; the
/// stream stores raw bytes.
#[derive(Default)]
pub(crate) struct Pending {
    buf: Vec<u8>,
    head: usize,
}

#[cfg(any(target_os = "macos", target_os = "freebsd"))]
const PENDING_RECORD_LEN: usize = 0;
#[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
const PENDING_RECORD_LEN: usize = 4;

impl Pending {
    pub(crate) fn is_empty(&self) -> bool {
        self.head == self.buf.len()
    }

    fn consumed(&mut self) {
        if self.head == self.buf.len() {
            self.buf.clear();
            self.head = 0;
        }
    }

    /// Reads without blocking until the socket has nothing more to read, EOF
    /// or an error (both set `eof`). Queued bytes beyond `bound` are a
    /// protocol failure. Each datagram is stored whole, so its later
    /// receive sees what a direct receive sees: one longer than `read_len`,
    /// a protocol violation, is rejected there instead of being stored cut
    /// short.
    fn drain(
        &mut self,
        fd: RawFd,
        read_len: usize,
        bound: usize,
        eof: &mut bool,
    ) -> Result<(), UdsError> {
        if self.head > 0 {
            self.buf.drain(..self.head);
            self.head = 0;
        }
        loop {
            #[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
            let read_len = {
                // MSG_TRUNC reports the full length on Linux 3.4 and later;
                // older kernels report at most the probe length.
                let mut probe = 0u8;
                let full = unsafe {
                    libc::recv(
                        fd,
                        &mut probe as *mut u8 as *mut libc::c_void,
                        1,
                        libc::MSG_DONTWAIT | libc::MSG_PEEK | libc::MSG_TRUNC,
                    )
                };
                if full <= 0 {
                    match errno() {
                        _ if full == 0 => *eof = true,
                        libc::EINTR => continue,
                        e if e == libc::EAGAIN || e == libc::EWOULDBLOCK => {}
                        // A socket error: the retried send reports it.
                        _ => *eof = true,
                    }
                    return Ok(());
                }
                let full = full as usize;
                if full > bound {
                    return Err(UdsError::Protocol(
                        "inbound bytes exceed the in-flight responses".into(),
                    ));
                }
                read_len.max(full)
            };
            let start = self.buf.len();
            self.buf.resize(start + PENDING_RECORD_LEN + read_len, 0);
            let dst = &mut self.buf[start + PENDING_RECORD_LEN..];
            let n = unsafe {
                libc::recv(
                    fd,
                    dst.as_mut_ptr() as *mut libc::c_void,
                    dst.len(),
                    libc::MSG_DONTWAIT,
                )
            };
            if n <= 0 {
                self.buf.truncate(start);
                match errno() {
                    _ if n == 0 => *eof = true,
                    libc::EINTR => continue,
                    e if e == libc::EAGAIN || e == libc::EWOULDBLOCK => {}
                    // A socket error: the retried send reports it.
                    _ => *eof = true,
                }
                return Ok(());
            }
            let n = n as usize;
            #[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
            self.buf[start..start + PENDING_RECORD_LEN].copy_from_slice(&(n as u32).to_ne_bytes());
            self.buf.truncate(start + PENDING_RECORD_LEN + n);
            if self.buf.len() > bound {
                return Err(UdsError::Protocol(
                    "inbound bytes exceed the in-flight responses".into(),
                ));
            }
        }
    }

    /// SEQPACKET: pops one queued datagram into `dst`, truncating it to
    /// `dst.len()` as recv() does. Returns the bytes copied.
    #[cfg_attr(any(target_os = "macos", target_os = "freebsd"), allow(dead_code))]
    fn pop_packet(&mut self, dst: &mut [u8]) -> usize {
        let h = self.head;
        let rec = u32::from_ne_bytes(self.buf[h..h + 4].try_into().unwrap()) as usize;
        let n = rec.min(dst.len());
        dst[..n].copy_from_slice(&self.buf[h + 4..h + 4 + n]);
        self.head = h + 4 + rec;
        self.consumed();
        n
    }

    /// Stream: copies up to `dst.len()` queued bytes. Returns the count.
    #[cfg_attr(not(any(target_os = "macos", target_os = "freebsd")), allow(dead_code))]
    pub(crate) fn take(&mut self, dst: &mut [u8]) -> usize {
        let n = dst.len().min(self.buf.len() - self.head);
        dst[..n].copy_from_slice(&self.buf[self.head..self.head + n]);
        self.head += n;
        self.consumed();
        n
    }
}

/// Wire bytes `inflight` responses can occupy: the pending queue bound.
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

/// `send_with_progress_loop`; macOS ignores MSG_DONTWAIT on a stream send
/// that lacks buffer space and blocks, so on the stream baseline (macOS,
/// FreeBSD) the descriptor is made non-blocking for the send instead.
#[allow(clippy::too_many_arguments)]
fn send_with_progress(
    fd: RawFd,
    hdr: &[u8],
    payload: &[u8],
    pending: &mut Pending,
    read_len: usize,
    bound: usize,
    recv_failed: bool,
    wait: Option<&ReceiveWait<'_>>,
) -> Result<(), UdsError> {
    #[cfg(any(target_os = "macos", target_os = "freebsd"))]
    {
        let flags = unsafe { libc::fcntl(fd, libc::F_GETFL) };
        if flags < 0 || unsafe { libc::fcntl(fd, libc::F_SETFL, flags | libc::O_NONBLOCK) } < 0 {
            return Err(UdsError::Send(errno()));
        }
        let result = send_with_progress_loop(
            fd,
            hdr,
            payload,
            pending,
            read_len,
            bound,
            recv_failed,
            wait,
        );
        // A descriptor left non-blocking would break later blocking calls;
        // a Send error also fails every in-flight request.
        if unsafe { libc::fcntl(fd, libc::F_SETFL, flags) } < 0 {
            let e = errno();
            unsafe { libc::shutdown(fd, libc::SHUT_RDWR) };
            return Err(UdsError::Send(e));
        }
        result
    }
    #[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
    send_with_progress_loop(
        fd,
        hdr,
        payload,
        pending,
        read_len,
        bound,
        recv_failed,
        wait,
    )
}

/// Sends one packet without blocking; while the socket is full, reads what
/// the peer sent into `pending`, so both peers keep making progress.
/// Without a wait it waits as long as a blocking send would; with one it
/// ends at the deadline or abort.
#[allow(clippy::too_many_arguments)]
fn send_with_progress_loop(
    fd: RawFd,
    hdr: &[u8],
    payload: &[u8],
    pending: &mut Pending,
    read_len: usize,
    bound: usize,
    recv_failed: bool,
    wait: Option<&ReceiveWait<'_>>,
) -> Result<(), UdsError> {
    // A SEQPACKET send moves the whole packet or nothing, so only the
    // stream advances these.
    #[cfg_attr(
        not(any(target_os = "macos", target_os = "freebsd")),
        allow(unused_mut)
    )]
    let mut parts = [hdr, payload];
    #[cfg_attr(
        not(any(target_os = "macos", target_os = "freebsd")),
        allow(unused_mut)
    )]
    let mut first = 0;
    let mut eof = recv_failed;
    loop {
        let mut iov = [libc::iovec {
            iov_base: std::ptr::null_mut(),
            iov_len: 0,
        }; 2];
        let mut iovcnt = 0;
        for part in &parts[first..] {
            if !part.is_empty() {
                iov[iovcnt] = libc::iovec {
                    iov_base: part.as_ptr() as *mut libc::c_void,
                    iov_len: part.len(),
                };
                iovcnt += 1;
            }
        }
        // Zero-initialize: musl's 64-bit msghdr has private padding fields.
        let mut msg: libc::msghdr = unsafe { std::mem::zeroed() };
        msg.msg_iov = iov.as_mut_ptr();
        msg.msg_iovlen = iovcnt as _;

        let n = unsafe { libc::sendmsg(fd, &msg, SEND_FLAGS | libc::MSG_DONTWAIT) };
        if n >= 0 {
            #[cfg(any(target_os = "macos", target_os = "freebsd"))]
            {
                if n == 0 {
                    return Err(UdsError::Send(0));
                }
                let mut left = n as usize;
                while first < parts.len() && left >= parts[first].len() {
                    left -= parts[first].len();
                    first += 1;
                }
                if first == parts.len() {
                    return Ok(());
                }
                parts[first] = &parts[first][left..];
                continue;
            }
            #[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
            {
                return if n as usize == hdr.len() + payload.len() {
                    Ok(())
                } else {
                    Err(UdsError::Send(errno()))
                };
            }
        }
        match errno() {
            libc::EINTR => continue,
            e if e == libc::EAGAIN || e == libc::EWOULDBLOCK => {}
            e => return Err(UdsError::Send(e)),
        }

        // After EOF only writability matters; the retried send reports a
        // closed peer.
        let timeout = match wait {
            Some(wait) => wait.poll_timeout_ms()?,
            None => -1,
        };
        let mut pfd = libc::pollfd {
            fd,
            events: libc::POLLOUT | if eof { 0 } else { libc::POLLIN },
            revents: 0,
        };
        let rc = unsafe { libc::poll(&mut pfd, 1, timeout) };
        if rc < 0 {
            let e = errno();
            if e == libc::EINTR {
                continue;
            }
            return Err(UdsError::Send(e));
        }
        if rc == 0 {
            // poll_timeout_ms() reports the deadline and the abort flag.
            continue;
        }
        if !eof && pfd.revents & (libc::POLLIN | libc::POLLHUP | libc::POLLERR) != 0 {
            pending.drain(fd, read_len, bound, &mut eof)?;
        }
    }
}

/// Send header + payload as one SEQPACKET message using sendmsg.
#[cfg_attr(any(target_os = "macos", target_os = "freebsd"), allow(dead_code))]
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

    // Zero-initialize: musl's 64-bit msghdr has private padding fields, so a
    // struct literal does not compile there.
    let mut msg: libc::msghdr = unsafe { std::mem::zeroed() };
    msg.msg_iov = iov.as_mut_ptr();
    msg.msg_iovlen = iovcnt;

    let n = retry_eintr(
        #[cfg(test)]
        &eintr_inject::SENDMSG,
        || unsafe { libc::sendmsg(fd, &msg, SEND_FLAGS) },
    );
    match n {
        Ok(n) if n as usize == total => Ok(()),
        Ok(_) => Err(UdsError::Send(errno())),
        Err(e) => Err(UdsError::Send(e)),
    }
}

/// Run a socket call, retrying EINTR. SOCK_SEQPACKET sends and receives
/// whole messages or nothing, so an interrupted call moved no data.
/// Returns the call's result, or the errno of a failed call.
fn retry_eintr(
    #[cfg(test)] inject: &'static std::thread::LocalKey<std::cell::Cell<u32>>,
    mut call: impl FnMut() -> isize,
) -> Result<isize, i32> {
    loop {
        #[cfg(test)]
        let injected = eintr_inject::take(inject);
        #[cfg(not(test))]
        let injected = false;
        let n = if injected { -1 } else { call() };
        if n >= 0 {
            return Ok(n);
        }
        let e = if injected { libc::EINTR } else { errno() };
        if e != libc::EINTR {
            return Err(e);
        }
    }
}

/// Test-only EINTR injection: the next N socket calls of a kind made on the
/// current thread fail with EINTR before reaching the kernel.
#[cfg(test)]
pub(crate) mod eintr_inject {
    use std::cell::Cell;
    use std::thread::LocalKey;

    thread_local! {
        pub(crate) static SEND: Cell<u32> = const { Cell::new(0) };
        pub(crate) static SENDMSG: Cell<u32> = const { Cell::new(0) };
        pub(crate) static RECV: Cell<u32> = const { Cell::new(0) };
        pub(crate) static ACCEPT: Cell<u32> = const { Cell::new(0) };
    }

    pub(crate) fn take(counter: &'static LocalKey<Cell<u32>>) -> bool {
        counter.with(|c| {
            let left = c.get();
            if left == 0 {
                return false;
            }
            c.set(left - 1);
            true
        })
    }

    /// Injections on this thread not consumed yet.
    pub(crate) fn pending() -> u32 {
        SEND.with(Cell::get)
            + SENDMSG.with(Cell::get)
            + RECV.with(Cell::get)
            + ACCEPT.with(Cell::get)
    }
}

/// Send a contiguous buffer as one SEQPACKET message.
#[cfg_attr(any(target_os = "macos", target_os = "freebsd"), allow(dead_code))]
fn raw_send(fd: RawFd, data: &[u8]) -> Result<(), UdsError> {
    let n = retry_eintr(
        #[cfg(test)]
        &eintr_inject::SEND,
        || unsafe {
            libc::send(
                fd,
                data.as_ptr() as *const libc::c_void,
                data.len(),
                SEND_FLAGS,
            )
        },
    );
    match n {
        Ok(n) if n as usize == data.len() => Ok(()),
        Ok(_) => Err(UdsError::Send(errno())),
        Err(e) => Err(UdsError::Send(e)),
    }
}

/// Receive one SEQPACKET message. Returns bytes received.
fn raw_recv(fd: RawFd, buf: &mut [u8]) -> Result<usize, UdsError> {
    let n = retry_eintr(
        #[cfg(test)]
        &eintr_inject::RECV,
        || unsafe { libc::recv(fd, buf.as_mut_ptr() as *mut libc::c_void, buf.len(), 0) },
    );
    match n {
        Ok(0) => Err(UdsError::Recv(0)),
        Ok(n) => Ok(n as usize),
        Err(e) => Err(UdsError::Recv(e)),
    }
}

struct ReceiveWait<'a> {
    deadline: Option<Instant>,
    abort: Option<&'a AtomicBool>,
}

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

    fn poll_timeout_ms(&self) -> Result<i32, UdsError> {
        if self.aborted() {
            return Err(UdsError::Aborted);
        }

        let mut wait_ms: Option<u64> = if let Some(deadline) = self.deadline {
            let now = Instant::now();
            if now >= deadline {
                return Err(UdsError::Timeout);
            }
            Some(
                deadline
                    .duration_since(now)
                    .as_millis()
                    .clamp(1, i32::MAX as u128) as u64,
            )
        } else {
            None
        };

        if self.abort.is_some() {
            wait_ms = Some(wait_ms.map_or(100, |ms| ms.min(100)));
        }

        Ok(wait_ms.map_or(-1, |ms| ms as i32))
    }
}

fn wait_fd_readable(fd: RawFd, wait: &ReceiveWait<'_>) -> Result<(), UdsError> {
    loop {
        let timeout = wait.poll_timeout_ms()?;
        let mut pfd = libc::pollfd {
            fd,
            events: libc::POLLIN,
            revents: 0,
        };
        let rc = unsafe { libc::poll(&mut pfd, 1, timeout) };
        if rc < 0 {
            let err = errno();
            if err == libc::EINTR {
                continue;
            }
            return Err(UdsError::Recv(err));
        }
        if rc == 0 {
            // An empty slice only means the abort flag is due for a check;
            // poll_timeout_ms() reports the real deadline.
            continue;
        }
        if wait.aborted() {
            return Err(UdsError::Aborted);
        }
        return Ok(());
    }
}

#[cfg_attr(any(target_os = "macos", target_os = "freebsd"), allow(dead_code))]
fn raw_recv_with_wait(
    fd: RawFd,
    buf: &mut [u8],
    wait: &mut ReceiveWait<'_>,
) -> Result<usize, UdsError> {
    wait_fd_readable(fd, wait)?;
    raw_recv(fd, buf)
}

#[cfg_attr(any(target_os = "macos", target_os = "freebsd"), allow(dead_code))]
fn raw_recv_control(
    fd: RawFd,
    buf: &mut [u8],
    controlled: bool,
    wait: &mut ReceiveWait<'_>,
) -> Result<usize, UdsError> {
    if controlled {
        raw_recv_with_wait(fd, buf, wait)
    } else {
        raw_recv(fd, buf)
    }
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
    if path_bytes.len() > addr.sun_path.len() {
        return Err(UdsError::PathTooLong);
    }
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
    let probe = uds_socket_cloexec();
    if probe < 0 {
        // Cannot probe liveness (fd exhaustion) — keep the endpoint; the
        // subsequent bind() fails instead of risking a live socket.
        return StaleResult::LiveServer;
    }

    let result = match connect_unix(probe, path) {
        Ok(()) => {
            // Connected => live server
            StaleResult::LiveServer
        }
        Err(UdsError::Connect(e)) if e == libc::ENOENT => StaleResult::NotExist,
        Err(_) => {
            // Nothing accepted the connection: a dead server's socket, or a
            // foreign file squatting on the endpoint path. Reclaim it.
            match std::fs::remove_file(path) {
                Ok(()) => StaleResult::Stale,
                Err(err) if err.kind() == io::ErrorKind::NotFound => StaleResult::NotExist,
                // Unlinking a directory fails with EISDIR on Linux and
                // EPERM on FreeBSD and macOS.
                Err(err)
                    if matches!(err.raw_os_error(), Some(libc::EISDIR) | Some(libc::EPERM)) =>
                {
                    match std::fs::remove_dir(path) {
                        Ok(()) => StaleResult::Stale,
                        Err(_) => StaleResult::LiveServer,
                    }
                }
                Err(_) => StaleResult::LiveServer,
            }
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

/// Longest single connect wait, so an abort ends a connect promptly.
const CONNECT_SLICE_MS: u64 = 100;

fn set_send_timeout(fd: RawFd, ms: u64) {
    let tv = libc::timeval {
        tv_sec: (ms / 1000) as _,
        tv_usec: ((ms % 1000) * 1000) as _,
    };
    unsafe {
        libc::setsockopt(
            fd,
            libc::SOL_SOCKET,
            libc::SO_SNDTIMEO,
            &tv as *const libc::timeval as *const libc::c_void,
            std::mem::size_of::<libc::timeval>() as libc::socklen_t,
        );
    }
}

/// A blocking AF_UNIX connect waits while the listener's backlog is full.
/// On Linux SO_SNDTIMEO bounds that wait and the connect fails with EAGAIN
/// when it expires; waiting in slices lets an abort end it. Other systems
/// refuse a full backlog at once.
fn connect_unix_bounded(fd: RawFd, path: &str, wait: &ReceiveWait<'_>) -> Result<(), UdsError> {
    let result = loop {
        if wait.aborted() {
            break Err(UdsError::Aborted);
        }
        let mut slice = CONNECT_SLICE_MS;
        if let Some(deadline) = wait.deadline {
            let now = Instant::now();
            if now >= deadline {
                break Err(UdsError::Timeout);
            }
            let left = deadline.duration_since(now).as_millis().max(1) as u64;
            slice = slice.min(left);
        }
        set_send_timeout(fd, slice);
        match connect_unix(fd, path) {
            Ok(()) => break Ok(()),
            Err(UdsError::Connect(e)) if e == libc::EISCONN => break Ok(()),
            Err(UdsError::Connect(e))
                if e == libc::EAGAIN
                    || e == libc::EWOULDBLOCK
                    || e == libc::EINTR
                    || e == libc::EALREADY => {}
            Err(e) => break Err(e),
        }
    };
    // Session sends block without a timeout.
    set_send_timeout(fd, 0);
    result
}

fn connect_and_handshake(
    fd: RawFd,
    path: &str,
    config: &ClientConfig,
    wait: &ReceiveWait<'_>,
) -> Result<UdsSession, UdsError> {
    connect_unix_bounded(fd, path, wait)?;

    let pkt_size = packet_size_limit(fd, config.packet_size);

    let supported = apply_default(config.supported_profiles, PROFILE_BASELINE) & PLATFORM_PROFILES;

    // Build HELLO
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

    // Send HELLO
    send_control(fd, &framing::hello_packet(&hello))?;

    // Receive HELLO_ACK; a server that never answers must not hold the
    // client past its deadline.
    let mut buf = [0u8; 128];
    #[cfg(any(target_os = "macos", target_os = "freebsd"))]
    let n = stream::recv_packet(fd, &mut buf, 0, Some(wait))?;
    #[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
    let n = {
        wait_fd_readable(fd, wait)?;
        raw_recv(fd, &mut buf)?
    };
    let ack: HelloAck = framing::parse_hello_ack(&buf[..n], supported)?;

    // Sanity: reject a packet_size too small for chunking arithmetic
    if ack.agreed_packet_size <= HEADER_SIZE as u32 {
        return Err(UdsError::Protocol("agreed packet_size too small".into()));
    }
    // A server may only lower our proposal; a larger size could exceed what
    // this side's kernel accepts as one datagram.
    if ack.agreed_packet_size > pkt_size {
        return Err(UdsError::Protocol(
            "agreed packet_size exceeds proposal".into(),
        ));
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
        pending: Pending::default(),
        recv_failed: false,
    })
}

// ---------------------------------------------------------------------------
//  Handshake: server side
// ---------------------------------------------------------------------------

fn server_handshake(
    fd: RawFd,
    config: &ServerConfig,
    session_id: u64,
    prepare: Option<&mut dyn FnMut(u32, u32, u32) -> bool>,
    #[cfg_attr(
        not(any(target_os = "macos", target_os = "freebsd")),
        allow(unused_variables)
    )]
    wait: &ReceiveWait<'_>,
) -> Result<UdsSession, UdsError> {
    let offer = ServerOffer {
        packet_size: packet_size_limit(fd, config.packet_size),
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

    // Receive HELLO. The caller waited for it; a datagram arrives whole,
    // a stream packet within the same deadline.
    let mut buf = [0u8; 128];
    #[cfg(any(target_os = "macos", target_os = "freebsd"))]
    let n = stream::recv_packet(fd, &mut buf, 0, Some(wait))?;
    #[cfg(not(any(target_os = "macos", target_os = "freebsd")))]
    let n = raw_recv(fd, &mut buf)?;
    let ack = framing::server_negotiate(&buf[..n], &offer, session_id, prepare, |pkt| {
        send_control(fd, pkt)
    })?;

    Ok(UdsSession {
        fd,
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
        inflight_ids: HashSet::new(),
        pending: Pending::default(),
        recv_failed: false,
    })
}

// ---------------------------------------------------------------------------
//  Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
#[path = "posix_tests.rs"]
mod tests;
