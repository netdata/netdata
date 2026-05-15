//! L1 POSIX SHM transport (Linux only).
//!
//! Shared memory data plane with spin+futex synchronization.
//! The SHM region carries the same outer protocol envelope as the UDS
//! transport. Higher levels see no difference.
//!
//! Wire-compatible with the C implementation in netipc_shm.c.

use std::path::{Path, PathBuf};
use std::ptr;

// ---------------------------------------------------------------------------
//  Constants
// ---------------------------------------------------------------------------

/// Magic value: "NSHM" as u32 LE.
pub const REGION_MAGIC: u32 = 0x4e53484d;
pub const REGION_VERSION: u16 = 3;
pub const REGION_ALIGNMENT: u32 = 64;
pub const HEADER_LEN: u16 = 64;
pub const DEFAULT_SPIN_TRIES: u32 = 128;

// Byte offsets of atomic fields in the region header.
const OFF_REQ_SEQ: usize = 32;
const OFF_RESP_SEQ: usize = 40;
const OFF_REQ_LEN: usize = 48;
const OFF_RESP_LEN: usize = 52;
const OFF_REQ_SIGNAL: usize = 56;
const OFF_RESP_SIGNAL: usize = 60;

// futex operations
const FUTEX_WAIT: i32 = 0;
const FUTEX_WAKE: i32 = 1;

// ---------------------------------------------------------------------------
//  Errors
// ---------------------------------------------------------------------------

/// SHM transport errors.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ShmError {
    /// SHM path exceeds limit.
    PathTooLong,
    /// open/shm_open failed.
    Open(i32),
    /// ftruncate failed.
    Truncate(i32),
    /// mmap failed.
    Mmap(i32),
    /// Header magic mismatch.
    BadMagic,
    /// Header version mismatch.
    BadVersion,
    /// header_len mismatch or corrupt.
    BadHeader,
    /// File too small / capacity mismatch.
    BadSize,
    /// Live server owns the region.
    AddrInUse,
    /// Server hasn't finished setup (retry).
    NotReady,
    /// Message exceeds area capacity.
    MsgTooLarge,
    /// Futex wait timed out.
    Timeout,
    /// Invalid argument.
    BadParam(String),
    /// Owner process has exited.
    PeerDead,
}

impl std::fmt::Display for ShmError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ShmError::PathTooLong => write!(f, "SHM path exceeds limit"),
            ShmError::Open(e) => write!(f, "open failed: errno {e}"),
            ShmError::Truncate(e) => write!(f, "ftruncate failed: errno {e}"),
            ShmError::Mmap(e) => write!(f, "mmap failed: errno {e}"),
            ShmError::BadMagic => write!(f, "SHM header magic mismatch"),
            ShmError::BadVersion => write!(f, "SHM header version mismatch"),
            ShmError::BadHeader => write!(f, "SHM header_len mismatch"),
            ShmError::BadSize => write!(f, "SHM file too small for declared areas"),
            ShmError::AddrInUse => write!(f, "SHM region owned by live server"),
            ShmError::NotReady => write!(f, "SHM server not ready"),
            ShmError::MsgTooLarge => write!(f, "message exceeds SHM area capacity"),
            ShmError::Timeout => write!(f, "SHM futex wait timed out"),
            ShmError::BadParam(s) => write!(f, "bad parameter: {s}"),
            ShmError::PeerDead => write!(f, "SHM owner process has exited"),
        }
    }
}

impl std::error::Error for ShmError {}

// ---------------------------------------------------------------------------
//  Role
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ShmRole {
    Server = 1,
    Client = 2,
}

// ---------------------------------------------------------------------------
//  Region header (64 bytes at offset 0)
// ---------------------------------------------------------------------------

/// On-disk layout. Not used directly for atomic accesses; we use
/// raw pointer arithmetic like the C implementation.
#[repr(C)]
struct RegionHeader {
    magic: u32,             //  0
    version: u16,           //  4
    header_len: u16,        //  6
    owner_pid: i32,         //  8
    owner_generation: u32,  // 12
    request_offset: u32,    // 16
    request_capacity: u32,  // 20
    response_offset: u32,   // 24
    response_capacity: u32, // 28
    req_seq: u64,           // 32
    resp_seq: u64,          // 40
    req_len: u32,           // 48
    resp_len: u32,          // 52
    req_signal: u32,        // 56
    resp_signal: u32,       // 60
}

const _: () = assert!(std::mem::size_of::<RegionHeader>() == 64);

// ---------------------------------------------------------------------------
//  SHM context
// ---------------------------------------------------------------------------

/// A handle to a shared memory region (server or client side).
pub struct ShmContext {
    role: ShmRole,
    fd: i32,
    base: *mut u8,
    region_size: usize,

    // Cached from header
    request_offset: u32,
    request_capacity: u32,
    response_offset: u32,
    response_capacity: u32,

    // Sequence tracking
    local_req_seq: u64,
    local_resp_seq: u64,

    spin_tries: u32,
    owner_generation: u32,
    path: PathBuf,
}

// ShmContext is Send: the mmap pointer is process-global shared memory,
// and the context is used by one thread at a time (single in-flight).
unsafe impl Send for ShmContext {}

impl ShmContext {
    /// Returns the role.
    pub fn role(&self) -> ShmRole {
        self.role
    }

    /// Returns the raw file descriptor.
    pub fn fd(&self) -> i32 {
        self.fd
    }

    /// Check if the region's owner process is still alive.
    pub fn owner_alive(&self) -> bool {
        if self.base.is_null() || self.region_size < HEADER_LEN as usize {
            return false;
        }
        // Read the PID and generation fields by computing their byte offsets
        // directly from self.base, avoiding any pointer dereference through
        // a reference type. This is safer than casting to *const RegionHeader
        // and dereferencing because we never materialize a borrow of the
        // header struct — we just read raw bytes at known offsets.
        //
        // SAFETY: self.base is a non-null mmap'd region of at least HEADER_LEN
        // bytes (checked above). The field offsets below are within HEADER_LEN.
        // owner_pid is at offset 8, owner_generation at offset 12
        let pid: i32 = unsafe {
            let p = self.base.add(8) as *const i32;
            ptr::read_volatile(p)
        };
        if !pid_alive(pid) {
            return false;
        }
        if self.owner_generation != 0 {
            let cur_gen: u32 = unsafe {
                let p = self.base.add(12) as *const u32;
                ptr::read_volatile(p)
            };
            if cur_gen != self.owner_generation {
                return false;
            }
        }
        true
    }

    /// Create a SHM region (server side).
    ///
    /// Creates `{run_dir}/{service_name}-{session_id:016x}.ipcshm` with O_EXCL.
    /// Pre-checks for stale files and unlinks them before creating.
    pub fn server_create(
        run_dir: &str,
        service_name: &str,
        session_id: u64,
        req_capacity: u32,
        resp_capacity: u32,
    ) -> Result<Self, ShmError> {
        let path = build_shm_path(run_dir, service_name, session_id)?;

        // Round capacities to alignment (fails if rounding would overflow u32)
        let req_cap = align64(req_capacity)
            .ok_or_else(|| ShmError::BadParam("request capacity overflow".into()))?;
        let resp_cap = align64(resp_capacity)
            .ok_or_else(|| ShmError::BadParam("response capacity overflow".into()))?;

        let req_off = align64(HEADER_LEN as u32)
            .ok_or_else(|| ShmError::BadParam("header offset overflow".into()))?;
        let resp_off = req_off
            .checked_add(req_cap)
            .and_then(align64)
            .ok_or_else(|| ShmError::BadParam("region offset overflow".into()))?;
        let region_size = resp_off
            .checked_add(resp_cap)
            .ok_or_else(|| ShmError::BadParam("region size overflow".into()))?
            as usize;

        // Try O_EXCL create first (fast path, no stale check needed).
        let c_path = path_to_cstring(&path)?;
        let mut fd = unsafe {
            libc::open(
                c_path.as_ptr(),
                libc::O_RDWR | libc::O_CREAT | libc::O_EXCL,
                0o600,
            )
        };

        // If O_EXCL failed (file exists), do stale recovery and retry.
        if fd < 0 && unsafe { *libc::__errno_location() } == libc::EEXIST {
            let stale = check_shm_stale(&path);
            if stale == StaleResult::LiveServer {
                return Err(ShmError::AddrInUse);
            }
            // Stale file was (hopefully) unlinked, retry create
            fd = unsafe {
                libc::open(
                    c_path.as_ptr(),
                    libc::O_RDWR | libc::O_CREAT | libc::O_EXCL,
                    0o600,
                )
            };
            // If retry still fails with EEXIST, the stale check couldn't
            // unlink (e.g., EACCES) — treat as address-in-use rather than
            // leaking EEXIST up the stack.
            if fd < 0 && unsafe { *libc::__errno_location() } == libc::EEXIST {
                return Err(ShmError::AddrInUse);
            }
        }
        if fd < 0 {
            return Err(ShmError::Open(errno()));
        }

        if unsafe { libc::ftruncate(fd, region_size as libc::off_t) } < 0 {
            let e = errno();
            unsafe {
                libc::close(fd);
                libc::unlink(c_path.as_ptr());
            }
            return Err(ShmError::Truncate(e));
        }

        let base = unsafe {
            libc::mmap(
                ptr::null_mut(),
                region_size,
                libc::PROT_READ | libc::PROT_WRITE,
                libc::MAP_SHARED,
                fd,
                0,
            )
        };
        if base == libc::MAP_FAILED {
            let e = errno();
            unsafe {
                libc::close(fd);
                libc::unlink(c_path.as_ptr());
            }
            return Err(ShmError::Mmap(e));
        }

        let base = base as *mut u8;

        // Zero the region
        unsafe { ptr::write_bytes(base, 0, region_size) };

        // Use a time-based generation to detect PID reuse across restarts.
        let generation = {
            let mut ts: libc::timespec = unsafe { std::mem::zeroed() };
            unsafe { libc::clock_gettime(libc::CLOCK_MONOTONIC, &mut ts) };
            (ts.tv_sec as u32) ^ ((ts.tv_nsec >> 10) as u32)
        };

        // Write header
        let hdr = base as *mut RegionHeader;
        unsafe {
            (*hdr).magic = REGION_MAGIC;
            (*hdr).version = REGION_VERSION;
            (*hdr).header_len = HEADER_LEN;
            (*hdr).owner_pid = libc::getpid();
            (*hdr).owner_generation = generation;
            (*hdr).request_offset = req_off;
            (*hdr).request_capacity = req_cap;
            (*hdr).response_offset = resp_off;
            (*hdr).response_capacity = resp_cap;
        }

        // Release fence so clients see header writes
        std::sync::atomic::fence(std::sync::atomic::Ordering::Release);

        Ok(ShmContext {
            role: ShmRole::Server,
            fd,
            base,
            region_size,
            request_offset: req_off,
            request_capacity: req_cap,
            response_offset: resp_off,
            response_capacity: resp_cap,
            local_req_seq: 0,
            local_resp_seq: 0,
            spin_tries: DEFAULT_SPIN_TRIES,
            owner_generation: generation,
            path,
        })
    }

    /// Attach to an existing SHM region (client side).
    pub fn client_attach(
        run_dir: &str,
        service_name: &str,
        session_id: u64,
    ) -> Result<Self, ShmError> {
        let path = build_shm_path(run_dir, service_name, session_id)?;
        let c_path = path_to_cstring(&path)?;

        let fd = unsafe { libc::open(c_path.as_ptr(), libc::O_RDWR) };
        if fd < 0 {
            return Err(ShmError::Open(errno()));
        }

        // Check file size
        let mut st: libc::stat = unsafe { std::mem::zeroed() };
        if unsafe { libc::fstat(fd, &mut st) } < 0 {
            unsafe { libc::close(fd) };
            return Err(ShmError::Open(errno()));
        }

        let file_size = st.st_size as usize;
        if file_size < HEADER_LEN as usize {
            unsafe { libc::close(fd) };
            return Err(ShmError::NotReady);
        }

        let base = unsafe {
            libc::mmap(
                ptr::null_mut(),
                file_size,
                libc::PROT_READ | libc::PROT_WRITE,
                libc::MAP_SHARED,
                fd,
                0,
            )
        };
        if base == libc::MAP_FAILED {
            unsafe { libc::close(fd) };
            return Err(ShmError::Mmap(errno()));
        }

        let base = base as *mut u8;

        // Acquire fence to see server's header writes
        std::sync::atomic::fence(std::sync::atomic::Ordering::Acquire);

        let hdr = base as *const RegionHeader;
        let (magic, version, hdr_len) =
            unsafe { ((*hdr).magic, (*hdr).version, (*hdr).header_len) };

        if magic != REGION_MAGIC {
            unsafe {
                libc::munmap(base as *mut libc::c_void, file_size);
                libc::close(fd);
            }
            return Err(ShmError::BadMagic);
        }
        if version != REGION_VERSION {
            unsafe {
                libc::munmap(base as *mut libc::c_void, file_size);
                libc::close(fd);
            }
            return Err(ShmError::BadVersion);
        }
        if hdr_len != HEADER_LEN {
            unsafe {
                libc::munmap(base as *mut libc::c_void, file_size);
                libc::close(fd);
            }
            return Err(ShmError::BadHeader);
        }

        let (req_off, req_cap, resp_off, resp_cap) = unsafe {
            (
                (*hdr).request_offset,
                (*hdr).request_capacity,
                (*hdr).response_offset,
                (*hdr).response_capacity,
            )
        };

        let header_end = align64(HEADER_LEN as u32).unwrap_or(HEADER_LEN as u32);
        if req_off < header_end || req_cap == 0 || resp_off < header_end || resp_cap == 0 {
            unsafe {
                libc::munmap(base as *mut libc::c_void, file_size);
                libc::close(fd);
            }
            return Err(ShmError::NotReady);
        }

        if req_off % REGION_ALIGNMENT != 0
            || req_cap % REGION_ALIGNMENT != 0
            || resp_off % REGION_ALIGNMENT != 0
            || resp_cap % REGION_ALIGNMENT != 0
            || req_off
                .checked_add(req_cap)
                .and_then(align64)
                .map_or(true, |min| resp_off < min)
        {
            unsafe {
                libc::munmap(base as *mut libc::c_void, file_size);
                libc::close(fd);
            }
            return Err(ShmError::BadSize);
        }

        // Validate region size
        let req_end = req_off as usize + req_cap as usize;
        let resp_end = resp_off as usize + resp_cap as usize;
        let needed = req_end.max(resp_end);
        if file_size < needed {
            unsafe {
                libc::munmap(base as *mut libc::c_void, file_size);
                libc::close(fd);
            }
            return Err(ShmError::BadSize);
        }

        // Read current sequence numbers
        let cur_req_seq = atomic_load_u64(base, OFF_REQ_SEQ);
        let cur_resp_seq = atomic_load_u64(base, OFF_RESP_SEQ);
        let generation = unsafe { (*hdr).owner_generation };

        Ok(ShmContext {
            role: ShmRole::Client,
            fd,
            base,
            region_size: file_size,
            request_offset: req_off,
            request_capacity: req_cap,
            response_offset: resp_off,
            response_capacity: resp_cap,
            local_req_seq: cur_req_seq,
            local_resp_seq: cur_resp_seq,
            spin_tries: DEFAULT_SPIN_TRIES,
            owner_generation: generation,
            path,
        })
    }

    /// Publish a message (client sends request, server sends response).
    ///
    /// The message must include the 32-byte outer header + payload,
    /// exactly as sent over UDS.
    pub fn send(&mut self, msg: &[u8]) -> Result<(), ShmError> {
        if self.base.is_null() || msg.is_empty() {
            return Err(ShmError::BadParam("null context or empty message".into()));
        }

        let (area_offset, area_capacity, seq_off, len_off, sig_off) = match self.role {
            ShmRole::Client => (
                self.request_offset,
                self.request_capacity,
                OFF_REQ_SEQ,
                OFF_REQ_LEN,
                OFF_REQ_SIGNAL,
            ),
            ShmRole::Server => (
                self.response_offset,
                self.response_capacity,
                OFF_RESP_SEQ,
                OFF_RESP_LEN,
                OFF_RESP_SIGNAL,
            ),
        };

        if msg.len() > area_capacity as usize {
            return Err(ShmError::MsgTooLarge);
        }

        // 1. Write message data into the area
        unsafe {
            ptr::copy_nonoverlapping(msg.as_ptr(), self.base.add(area_offset as usize), msg.len());
        }

        // 2. Store message length (release)
        atomic_store_u32(self.base, len_off, msg.len() as u32);

        // 3. Increment sequence number (release) to publish
        atomic_add_u64(self.base, seq_off, 1);

        // 4. Wake the peer via futex
        atomic_add_u32(self.base, sig_off, 1);
        futex_wake(unsafe { self.base.add(sig_off) as *mut u32 }, 1);

        // Track locally
        match self.role {
            ShmRole::Client => self.local_req_seq += 1,
            ShmRole::Server => self.local_resp_seq += 1,
        }

        Ok(())
    }

    /// Receive a message into the caller-provided buffer.
    ///
    /// On success, returns the number of bytes written to `buf`.
    /// Returns `MsgTooLarge` if the message exceeds `buf.len()`.
    pub fn receive(&mut self, buf: &mut [u8], timeout_ms: u32) -> Result<usize, ShmError> {
        if self.base.is_null() {
            return Err(ShmError::BadParam("null context".into()));
        }
        if buf.is_empty() {
            return Err(ShmError::BadParam("empty buffer".into()));
        }

        let (area_offset, area_capacity, seq_off, len_off, sig_off, expected_seq) = match self.role
        {
            ShmRole::Server => (
                self.request_offset,
                self.request_capacity,
                OFF_REQ_SEQ,
                OFF_REQ_LEN,
                OFF_REQ_SIGNAL,
                self.local_req_seq + 1,
            ),
            ShmRole::Client => (
                self.response_offset,
                self.response_capacity,
                OFF_RESP_SEQ,
                OFF_RESP_LEN,
                OFF_RESP_SIGNAL,
                self.local_resp_seq + 1,
            ),
        };

        let max_copy = buf.len().min(area_capacity as usize);

        // Phase 1: spin. Copy immediately on observing the advance.
        let mut observed = false;
        let mut mlen = 0usize;
        for _ in 0..self.spin_tries {
            let cur = atomic_load_u64(self.base, seq_off);
            if cur >= expected_seq {
                mlen = atomic_load_u32(self.base, len_off) as usize;
                if mlen > 0 && mlen <= max_copy {
                    unsafe {
                        ptr::copy_nonoverlapping(
                            self.base.add(area_offset as usize),
                            buf.as_mut_ptr(),
                            mlen,
                        );
                    }
                }
                observed = true;
                break;
            }
            cpu_relax();
        }

        // Phase 2: futex wait with deadline-based retry loop.
        //
        // Handles spurious wakeups (EAGAIN when signal word changed
        // between read and syscall, or EINTR from signal delivery).
        // Computes a wall-clock deadline so total wait never exceeds
        // timeout_ms regardless of retries.
        if !observed {
            let deadline_ns: u64 = if timeout_ms > 0 {
                let mut ts = libc::timespec {
                    tv_sec: 0,
                    tv_nsec: 0,
                };
                unsafe { libc::clock_gettime(libc::CLOCK_MONOTONIC, &mut ts) };
                ts.tv_sec as u64 * 1_000_000_000 + ts.tv_nsec as u64 + timeout_ms as u64 * 1_000_000
            } else {
                0
            };

            loop {
                let sig_val = atomic_load_u32(self.base, sig_off);

                let cur = atomic_load_u64(self.base, seq_off);
                if cur >= expected_seq {
                    break; // response arrived
                }

                // Compute remaining timeout for this futex_wait call
                let timeout = if deadline_ns > 0 {
                    let mut now_ts = libc::timespec {
                        tv_sec: 0,
                        tv_nsec: 0,
                    };
                    unsafe { libc::clock_gettime(libc::CLOCK_MONOTONIC, &mut now_ts) };
                    let now_val = now_ts.tv_sec as u64 * 1_000_000_000 + now_ts.tv_nsec as u64;
                    if now_val >= deadline_ns {
                        return Err(ShmError::Timeout);
                    }
                    let remain = deadline_ns - now_val;
                    Some(libc::timespec {
                        tv_sec: (remain / 1_000_000_000) as libc::time_t,
                        tv_nsec: (remain % 1_000_000_000) as libc::c_long,
                    })
                } else {
                    None
                };

                let ret = futex_wait(
                    unsafe { self.base.add(sig_off) as *mut u32 },
                    sig_val,
                    timeout.as_ref(),
                );

                if ret < 0 && errno() == libc::ETIMEDOUT {
                    return Err(ShmError::Timeout);
                }

                // EAGAIN (value changed) or EINTR (signal): re-check seq
            }

            // Copy immediately after observing the sequence advance
            mlen = atomic_load_u32(self.base, len_off) as usize;
            if mlen > 0 && mlen <= max_copy {
                unsafe {
                    ptr::copy_nonoverlapping(
                        self.base.add(area_offset as usize),
                        buf.as_mut_ptr(),
                        mlen,
                    );
                }
            }
        }

        // Advance local tracking (message is consumed from SHM perspective)
        match self.role {
            ShmRole::Server => self.local_req_seq = expected_seq,
            ShmRole::Client => self.local_resp_seq = expected_seq,
        }

        // mlen==0 after sequence advance indicates SHM corruption (send rejects 0-length)
        if mlen == 0 {
            return Err(ShmError::BadHeader);
        }

        // Message larger than caller buffer or area capacity
        if mlen > max_copy {
            return Err(ShmError::MsgTooLarge);
        }

        Ok(mlen)
    }

    /// Close client (munmap, close fd, no unlink).
    pub fn close(&mut self) {
        if !self.base.is_null() {
            unsafe { libc::munmap(self.base as *mut libc::c_void, self.region_size) };
            self.base = ptr::null_mut();
        }
        if self.fd >= 0 {
            unsafe { libc::close(self.fd) };
            self.fd = -1;
        }
        self.region_size = 0;
    }

    /// Destroy server (munmap, close, unlink).
    pub fn destroy(&mut self) {
        if !self.base.is_null() {
            unsafe { libc::munmap(self.base as *mut libc::c_void, self.region_size) };
            self.base = ptr::null_mut();
        }
        if self.fd >= 0 {
            unsafe { libc::close(self.fd) };
            self.fd = -1;
        }
        if !self.path.as_os_str().is_empty() {
            if let Ok(c) = std::ffi::CString::new(self.path.to_string_lossy().as_bytes()) {
                unsafe { libc::unlink(c.as_ptr()) };
            }
            self.path = PathBuf::new();
        }
        self.region_size = 0;
    }
}

impl Drop for ShmContext {
    fn drop(&mut self) {
        match self.role {
            ShmRole::Server => self.destroy(),
            ShmRole::Client => self.close(),
        }
    }
}

// ---------------------------------------------------------------------------
//  Stale session cleanup
// ---------------------------------------------------------------------------

/// Scan `run_dir` for files matching `{service_name}-*.ipcshm`, check
/// owner_pid liveness for each, and unlink stale ones.
pub fn cleanup_stale(run_dir: &str, service_name: &str) {
    let prefix = format!("{service_name}-");
    let suffix = ".ipcshm";

    let entries = match std::fs::read_dir(run_dir) {
        Ok(e) => e,
        Err(_) => return,
    };

    for entry in entries.flatten() {
        let name = match entry.file_name().into_string() {
            Ok(n) => n,
            Err(_) => continue,
        };

        if !name.starts_with(&prefix) || !name.ends_with(suffix) {
            continue;
        }

        let path = entry.path();
        let c_path = match path_to_cstring(&path) {
            Ok(c) => c,
            Err(_) => continue,
        };

        // Open read-only to inspect the header
        let fd = unsafe { libc::open(c_path.as_ptr(), libc::O_RDONLY) };
        if fd < 0 {
            // The entry vanished after readdir() (for example, a dangling symlink
            // target disappeared) — remove the stale directory entry. Any other
            // open failure is ambiguous, so leave the entry alone.
            if should_unlink_cleanup_open_failure(errno()) {
                unsafe { libc::unlink(c_path.as_ptr()) };
            }
            continue;
        }

        let mut st: libc::stat = unsafe { std::mem::zeroed() };
        if unsafe { libc::fstat(fd, &mut st) } != 0 || (st.st_size as usize) < HEADER_LEN as usize {
            unsafe {
                libc::close(fd);
                libc::unlink(c_path.as_ptr());
            }
            continue;
        }

        let map = unsafe {
            libc::mmap(
                ptr::null_mut(),
                HEADER_LEN as usize,
                libc::PROT_READ,
                libc::MAP_SHARED,
                fd,
                0,
            )
        };
        unsafe { libc::close(fd) };

        if map == libc::MAP_FAILED {
            unsafe { libc::unlink(c_path.as_ptr()) };
            continue;
        }

        let hdr = map as *const RegionHeader;
        let magic = unsafe { (*hdr).magic };
        if magic != REGION_MAGIC {
            unsafe {
                libc::munmap(map, HEADER_LEN as usize);
                libc::unlink(c_path.as_ptr());
            }
            continue;
        }

        let owner = unsafe { (*hdr).owner_pid };
        let gen = unsafe { (*hdr).owner_generation };
        unsafe { libc::munmap(map, HEADER_LEN as usize) };

        // If owner is dead (or generation is zero / legacy), unlink
        if !pid_alive(owner) || gen == 0 {
            unsafe { libc::unlink(c_path.as_ptr()) };
        }
    }
}

// ---------------------------------------------------------------------------
//  Internal helpers
// ---------------------------------------------------------------------------

/// Round v up to REGION_ALIGNMENT. Returns None if the rounded value would
/// overflow u32 — callers must reject such inputs rather than silently wrap.
fn align64(v: u32) -> Option<u32> {
    v.checked_add(REGION_ALIGNMENT - 1)
        .map(|x| x & !(REGION_ALIGNMENT - 1))
}

/// Validate service_name: only [a-zA-Z0-9._-], non-empty, not "." or "..".
fn validate_service_name(name: &str) -> Result<(), ShmError> {
    if name.is_empty() {
        return Err(ShmError::BadParam("empty service name".into()));
    }
    if name == "." || name == ".." {
        return Err(ShmError::BadParam(
            "service name cannot be '.' or '..'".into(),
        ));
    }
    for c in name.bytes() {
        match c {
            b'a'..=b'z' | b'A'..=b'Z' | b'0'..=b'9' | b'.' | b'_' | b'-' => {}
            _ => {
                return Err(ShmError::BadParam(format!(
                    "service name contains invalid character: {:?}",
                    c as char
                )))
            }
        }
    }
    Ok(())
}

fn build_shm_path(run_dir: &str, service_name: &str, session_id: u64) -> Result<PathBuf, ShmError> {
    validate_service_name(service_name)?;
    let path = Path::new(run_dir).join(format!("{service_name}-{session_id:016x}.ipcshm"));
    if path.to_string_lossy().len() >= 256 {
        return Err(ShmError::PathTooLong);
    }
    Ok(path)
}

fn path_to_cstring(path: &Path) -> Result<std::ffi::CString, ShmError> {
    std::ffi::CString::new(path.to_string_lossy().as_bytes())
        .map_err(|_| ShmError::BadParam("path contains null byte".into()))
}

fn errno() -> i32 {
    unsafe { *libc::__errno_location() }
}

fn pid_alive(pid: i32) -> bool {
    if pid <= 0 {
        return false;
    }
    let ret = unsafe { libc::kill(pid, 0) };
    ret == 0 || errno() == libc::EPERM
}

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

// Atomic helpers: operate on raw pointers into the mmap'd region.
// These match the C implementation's __atomic builtins.

fn atomic_load_u64(base: *mut u8, offset: usize) -> u64 {
    let ptr = unsafe { base.add(offset) as *const std::sync::atomic::AtomicU64 };
    unsafe { (*ptr).load(std::sync::atomic::Ordering::Acquire) }
}

fn atomic_load_u32(base: *mut u8, offset: usize) -> u32 {
    let ptr = unsafe { base.add(offset) as *const std::sync::atomic::AtomicU32 };
    unsafe { (*ptr).load(std::sync::atomic::Ordering::Acquire) }
}

fn atomic_store_u32(base: *mut u8, offset: usize, val: u32) {
    let ptr = unsafe { base.add(offset) as *const std::sync::atomic::AtomicU32 };
    unsafe { (*ptr).store(val, std::sync::atomic::Ordering::Release) };
}

fn atomic_add_u64(base: *mut u8, offset: usize, val: u64) {
    let ptr = unsafe { base.add(offset) as *const std::sync::atomic::AtomicU64 };
    unsafe { (*ptr).fetch_add(val, std::sync::atomic::Ordering::Release) };
}

fn atomic_add_u32(base: *mut u8, offset: usize, val: u32) {
    let ptr = unsafe { base.add(offset) as *const std::sync::atomic::AtomicU32 };
    unsafe { (*ptr).fetch_add(val, std::sync::atomic::Ordering::Release) };
}

fn futex_wake(addr: *mut u32, count: i32) -> i32 {
    unsafe {
        libc::syscall(
            libc::SYS_futex,
            addr,
            FUTEX_WAKE,
            count,
            ptr::null::<libc::timespec>(),
            ptr::null::<u32>(),
            0i32,
        ) as i32
    }
}

fn futex_wait(addr: *mut u32, expected: u32, timeout: Option<&libc::timespec>) -> i32 {
    let tsp = match timeout {
        Some(ts) => ts as *const libc::timespec,
        None => ptr::null(),
    };
    unsafe {
        libc::syscall(
            libc::SYS_futex,
            addr,
            FUTEX_WAIT,
            expected,
            tsp,
            ptr::null::<u32>(),
            0i32,
        ) as i32
    }
}

// ---------------------------------------------------------------------------
//  Stale region recovery
// ---------------------------------------------------------------------------

#[derive(PartialEq, Eq)]
#[allow(dead_code)]
enum StaleResult {
    NotExist,
    Recovered,
    LiveServer,
    Invalid,
}

fn should_unlink_cleanup_open_failure(err: i32) -> bool {
    err == libc::ENOENT
}

fn classify_stale_open_failure(err: i32) -> StaleResult {
    if err == libc::ENOENT {
        StaleResult::NotExist
    } else {
        StaleResult::Invalid
    }
}

#[allow(dead_code)]
fn check_shm_stale(path: &Path) -> StaleResult {
    let c_path = match path_to_cstring(path) {
        Ok(c) => c,
        Err(_) => return StaleResult::NotExist,
    };

    let mut st: libc::stat = unsafe { std::mem::zeroed() };
    if unsafe { libc::stat(c_path.as_ptr(), &mut st) } != 0 {
        return StaleResult::NotExist;
    }

    if (st.st_size as usize) < HEADER_LEN as usize {
        unsafe { libc::unlink(c_path.as_ptr()) };
        return StaleResult::Invalid;
    }

    let fd = unsafe { libc::open(c_path.as_ptr(), libc::O_RDONLY) };
    if fd < 0 {
        return classify_stale_open_failure(errno());
    }

    let map = unsafe {
        libc::mmap(
            ptr::null_mut(),
            HEADER_LEN as usize,
            libc::PROT_READ,
            libc::MAP_SHARED,
            fd,
            0,
        )
    };
    unsafe { libc::close(fd) };

    if map == libc::MAP_FAILED {
        unsafe { libc::unlink(c_path.as_ptr()) };
        return StaleResult::Invalid;
    }

    let hdr = map as *const RegionHeader;
    let magic = unsafe { (*hdr).magic };
    if magic != REGION_MAGIC {
        unsafe {
            libc::munmap(map, HEADER_LEN as usize);
            libc::unlink(c_path.as_ptr());
        }
        return StaleResult::Invalid;
    }

    let owner = unsafe { (*hdr).owner_pid };
    let gen = unsafe { (*hdr).owner_generation };
    unsafe { libc::munmap(map, HEADER_LEN as usize) };

    if pid_alive(owner) && gen != 0 {
        return StaleResult::LiveServer;
    }

    // Dead owner or zero generation (PID reuse / legacy) — stale
    unsafe { libc::unlink(c_path.as_ptr()) };
    StaleResult::Recovered
}

// ---------------------------------------------------------------------------
//  Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
#[path = "shm_tests.rs"]
mod tests;
