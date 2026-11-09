//! Stream-socket packet I/O, the macOS and FreeBSD baseline (macOS has no
//! AF_UNIX SOCK_SEQPACKET; FreeBSD 15 SEQPACKET keeps record boundaries only
//! with MSG_EOR). A packet is the 32-byte header plus the bytes it declares,
//! so the bytes on the wire match the SEQPACKET transport. Built on every
//! POSIX system so its tests run everywhere; only macOS and FreeBSD sessions
//! use it.

use super::{Pending, ReceiveWait, UdsError, raw_recv, wait_fd_readable};
use crate::protocol::{HEADER_SIZE, Header, MAGIC_CHUNK, MAGIC_MSG};
use std::os::unix::io::RawFd;

fn field_u32(hdr: &[u8], offset: usize) -> u32 {
    u32::from_ne_bytes(hdr[offset..offset + 4].try_into().unwrap())
}

/// Bytes that follow a received 32-byte header: `min(payload_len,
/// packet_size - 32)` after an outer header, `chunk_payload_len` after a
/// continuation header, none after any other magic. `packet_size` 0
/// (handshake) caps an outer header at `buf_len`. Bytes beyond `buf_len`
/// are not counted, as a SEQPACKET recv into a short buffer drops them; the
/// caller's length checks then fail.
pub(super) fn body_len(hdr: &[u8], buf_len: usize, packet_size: u32) -> usize {
    let declared = match field_u32(hdr, 0) {
        MAGIC_MSG => {
            let cap = if packet_size as usize > HEADER_SIZE {
                packet_size as usize - HEADER_SIZE
            } else {
                buf_len - HEADER_SIZE
            };
            (field_u32(hdr, 16) as usize).min(cap)
        }
        MAGIC_CHUNK => field_u32(hdr, 28) as usize,
        _ => 0,
    };
    declared.min(buf_len - HEADER_SIZE)
}

/// Read exactly `dst.len()` bytes, waiting before each read when `wait` is
/// set (None blocks). EOF before the first byte is a disconnect (`Recv`);
/// EOF inside a packet (any byte read, or `mid_packet`) is a truncated
/// packet (`Protocol`).
pub(super) fn read_exact(
    fd: RawFd,
    dst: &mut [u8],
    wait: Option<&ReceiveWait<'_>>,
    mid_packet: bool,
) -> Result<(), UdsError> {
    read_exact_counted(fd, dst, wait, mid_packet, &mut 0)
}

/// `read_exact` that records in `got` the bytes it read, also on failure.
fn read_exact_counted(
    fd: RawFd,
    dst: &mut [u8],
    wait: Option<&ReceiveWait<'_>>,
    mid_packet: bool,
    got: &mut usize,
) -> Result<(), UdsError> {
    *got = 0;
    while *got < dst.len() {
        if let Some(wait) = wait {
            wait_fd_readable(fd, wait)?;
        }
        #[cfg(test)]
        let end = if short_io::take(&short_io::RECV) && dst.len() - *got > 1 {
            *got + 1
        } else {
            dst.len()
        };
        #[cfg(not(test))]
        let end = dst.len();
        match raw_recv(fd, &mut dst[*got..end]) {
            Ok(n) => *got += n,
            Err(UdsError::Recv(0)) if mid_packet || *got > 0 => {
                return Err(UdsError::Protocol("truncated packet".into()));
            }
            Err(e) => return Err(e),
        }
    }
    Ok(())
}

/// Read one packet (header, then its body). `buf` must hold the header.
pub(super) fn recv_packet(
    fd: RawFd,
    buf: &mut [u8],
    packet_size: u32,
    wait: Option<&ReceiveWait<'_>>,
) -> Result<usize, UdsError> {
    if buf.len() < HEADER_SIZE {
        return Err(UdsError::BadParam("packet buffer too small".into()));
    }
    read_exact(fd, &mut buf[..HEADER_SIZE], wait, false)?;
    let body = body_len(&buf[..HEADER_SIZE], buf.len(), packet_size);
    read_exact(fd, &mut buf[HEADER_SIZE..HEADER_SIZE + body], wait, true)?;
    Ok(HEADER_SIZE + body)
}

/// `read_exact` that takes the session's pending bytes first. Sets
/// `consumed` when it takes any byte, also on failure.
fn read_exact_pending(
    fd: RawFd,
    pending: &mut Pending,
    dst: &mut [u8],
    wait: Option<&ReceiveWait<'_>>,
    mid_packet: bool,
    consumed: &mut bool,
) -> Result<(), UdsError> {
    let got = pending.take(dst);
    *consumed |= got > 0;
    if got == dst.len() {
        return Ok(());
    }
    let mut read = 0;
    let result = read_exact_counted(fd, &mut dst[got..], wait, mid_packet || got > 0, &mut read);
    *consumed |= read > 0;
    result
}

/// Read one packet of a session's message, from its pending bytes first. The first packet's header is
/// checked before its body is read: one that is not an outer header, or
/// declares more than this direction's limits, comes back alone so the
/// framing rejects it without waiting for bytes a stream peer may never
/// send. A truncated continuation is a chunk error, as on SEQPACKET.
/// `consumed` is set once any byte of the message has been taken.
#[allow(clippy::too_many_arguments)]
pub(super) fn recv_session_packet(
    fd: RawFd,
    pending: &mut Pending,
    buf: &mut [u8],
    first: bool,
    packet_size: u32,
    max_payload: u32,
    max_items: u32,
    wait: Option<&ReceiveWait<'_>>,
    consumed: &mut bool,
) -> Result<usize, UdsError> {
    if buf.len() < HEADER_SIZE {
        return Err(UdsError::Protocol("packet too short for header".into()));
    }
    let result = (|| {
        read_exact_pending(fd, pending, &mut buf[..HEADER_SIZE], wait, false, consumed)?;
        let hdr = &buf[..HEADER_SIZE];
        if first
            && Header::decode(hdr).map_or(true, |h| {
                h.payload_len > max_payload || h.item_count > max_items
            })
        {
            return Ok(HEADER_SIZE);
        }
        let body = body_len(hdr, buf.len(), packet_size);
        read_exact_pending(
            fd,
            pending,
            &mut buf[HEADER_SIZE..HEADER_SIZE + body],
            wait,
            true,
            consumed,
        )?;
        Ok(HEADER_SIZE + body)
    })();
    match result {
        Err(UdsError::Protocol(msg)) if !first => Err(UdsError::Chunk(msg)),
        r => r,
    }
}

/// Send header + payload completely, resuming after partial writes.
pub(super) fn send_all(fd: RawFd, hdr: &[u8], payload: &[u8]) -> Result<(), UdsError> {
    let mut parts = [hdr, payload];
    let mut first = 0;
    while first < parts.len() {
        if parts[first].is_empty() {
            first += 1;
            continue;
        }
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
        #[cfg(test)]
        if short_io::take(&short_io::SENDMSG) {
            iov[0].iov_len = 1;
            iovcnt = 1;
        }

        // Zero-initialize: musl's 64-bit msghdr has private padding fields.
        let mut msg: libc::msghdr = unsafe { std::mem::zeroed() };
        msg.msg_iov = iov.as_mut_ptr();
        msg.msg_iovlen = iovcnt as _;

        // A stream write may move part of the data; EINTR means none moved.
        let n = super::retry_eintr(
            #[cfg(test)]
            &super::eintr_inject::SENDMSG,
            || unsafe { libc::sendmsg(fd, &msg, super::SEND_FLAGS) },
        )
        .map_err(UdsError::Send)?;
        if n == 0 {
            return Err(UdsError::Send(0));
        }

        let mut left = n as usize;
        while first < parts.len() && left >= parts[first].len() {
            left -= parts[first].len();
            first += 1;
        }
        if first < parts.len() {
            parts[first] = &parts[first][left..];
        }
    }
    Ok(())
}

/// Test-only short I/O: the next N reads of more than one byte, or writes,
/// made on the current thread move one byte, as a stream socket may.
#[cfg(test)]
pub(crate) mod short_io {
    use std::cell::Cell;
    use std::thread::LocalKey;

    thread_local! {
        pub(crate) static RECV: Cell<u32> = const { Cell::new(0) };
        pub(crate) static SENDMSG: Cell<u32> = const { Cell::new(0) };
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

    pub(crate) fn pending() -> u32 {
        RECV.with(Cell::get) + SENDMSG.with(Cell::get)
    }
}

#[cfg(test)]
mod tests {
    use super::super::eintr_inject;
    use super::*;
    use crate::protocol::{ChunkHeader, KIND_REQUEST, VERSION};

    fn stream_pair() -> (RawFd, RawFd) {
        let mut fds = [0; 2];
        let rc = unsafe { libc::socketpair(libc::AF_UNIX, libc::SOCK_STREAM, 0, fds.as_mut_ptr()) };
        assert_eq!(rc, 0);
        (fds[0], fds[1])
    }

    fn close(fd: RawFd) {
        unsafe { libc::close(fd) };
    }

    fn outer(declared: u32, body: &[u8]) -> Vec<u8> {
        let hdr = Header {
            magic: MAGIC_MSG,
            version: VERSION,
            header_len: HEADER_SIZE as u16,
            kind: KIND_REQUEST,
            code: 1,
            item_count: 1,
            message_id: 9,
            payload_len: declared,
            ..Default::default()
        };
        let mut pkt = vec![0u8; HEADER_SIZE];
        hdr.encode(&mut pkt);
        pkt.extend_from_slice(body);
        pkt
    }

    fn continuation(declared: u32, body: &[u8]) -> Vec<u8> {
        let chk = ChunkHeader {
            magic: MAGIC_CHUNK,
            version: VERSION,
            message_id: 9,
            chunk_index: 1,
            chunk_count: 2,
            chunk_payload_len: declared,
            ..Default::default()
        };
        let mut pkt = vec![0u8; HEADER_SIZE];
        chk.encode(&mut pkt);
        pkt.extend_from_slice(body);
        pkt
    }

    #[test]
    fn test_body_len_comes_from_the_header() {
        let pkt = outer(100, &[]);
        assert_eq!(body_len(&pkt, 1024, 1024), 100, "whole payload fits");
        assert_eq!(body_len(&pkt, 1024, 48), 16, "first chunk fills the packet");
        assert_eq!(body_len(&pkt, 80, 0), 48, "handshake: capped by the buffer");
        assert_eq!(body_len(&pkt, 64, 1024), 32, "bytes past the buffer");
        assert_eq!(body_len(&continuation(7, &[]), 1024, 48), 7);
        let mut unknown = vec![0u8; HEADER_SIZE];
        unknown[..4].copy_from_slice(&0x1234_5678u32.to_ne_bytes());
        assert_eq!(body_len(&unknown, 1024, 1024), 0, "unknown magic");
    }

    #[test]
    fn test_packets_round_trip_and_eof_mapping() {
        let (a, b) = stream_pair();
        let pkt1 = outer(5, b"hello");
        let pkt2 = continuation(3, b"abc");
        send_all(a, &pkt1[..HEADER_SIZE], &pkt1[HEADER_SIZE..]).unwrap();
        send_all(a, &pkt2, &[]).unwrap();

        let mut buf = [0u8; 64];
        assert_eq!(recv_packet(b, &mut buf, 64, None).unwrap(), pkt1.len());
        assert_eq!(&buf[..pkt1.len()], &pkt1[..]);
        assert_eq!(recv_packet(b, &mut buf, 64, None).unwrap(), pkt2.len());
        assert_eq!(&buf[..pkt2.len()], &pkt2[..]);

        let wait = ReceiveWait::new(1, None);
        assert_eq!(
            recv_packet(b, &mut buf, 64, Some(&wait)),
            Err(UdsError::Timeout),
            "idle stream times out"
        );

        let truncated = outer(5, b"he");
        send_all(a, &truncated, &[]).unwrap();
        unsafe { libc::shutdown(a, libc::SHUT_WR) };
        assert!(
            matches!(
                recv_packet(b, &mut buf, 64, None),
                Err(UdsError::Protocol(_))
            ),
            "EOF inside the body"
        );
        close(a);
        close(b);

        let (a, b) = stream_pair();
        send_all(a, &truncated[..10], &[]).unwrap();
        unsafe { libc::shutdown(a, libc::SHUT_WR) };
        assert!(
            matches!(
                recv_packet(b, &mut buf, 64, None),
                Err(UdsError::Protocol(_))
            ),
            "EOF inside the header"
        );
        close(a);
        close(b);

        let (a, b) = stream_pair();
        close(a);
        assert_eq!(
            recv_packet(b, &mut buf, 64, None),
            Err(UdsError::Recv(0)),
            "EOF between packets is a disconnect"
        );
        close(b);
    }

    #[test]
    fn test_session_packet_checks_the_first_header_first() {
        let (a, b) = stream_pair();
        let mut buf = [0u8; 256];

        // Over the payload limit: the header comes back alone, the body is
        // never waited for.
        send_all(a, &outer(200, &[]), &[]).unwrap();
        assert_eq!(
            recv_session_packet(
                b,
                &mut Pending::default(),
                &mut buf,
                true,
                256,
                100,
                1,
                None,
                &mut false
            )
            .unwrap(),
            HEADER_SIZE
        );

        // Any header that does not decode, here a wrong version: header only.
        let mut bad_version = outer(50, &[]);
        bad_version[4..6].copy_from_slice(&(VERSION + 1).to_ne_bytes());
        send_all(a, &bad_version, &[]).unwrap();
        assert_eq!(
            recv_session_packet(
                b,
                &mut Pending::default(),
                &mut buf,
                true,
                256,
                100,
                1,
                None,
                &mut false
            )
            .unwrap(),
            HEADER_SIZE
        );

        // A continuation header where a message starts: header only.
        send_all(a, &continuation(3, b"abc"), &[]).unwrap();
        assert_eq!(
            recv_session_packet(
                b,
                &mut Pending::default(),
                &mut buf,
                true,
                256,
                100,
                1,
                None,
                &mut false
            )
            .unwrap(),
            HEADER_SIZE
        );
        // Its body is still queued; read it off as a raw packet body.
        read_exact(b, &mut buf[..3], None, true).unwrap();

        // A truncated continuation is a chunk error.
        send_all(a, &continuation(5, b"ab"), &[]).unwrap();
        unsafe { libc::shutdown(a, libc::SHUT_WR) };
        assert!(matches!(
            recv_session_packet(
                b,
                &mut Pending::default(),
                &mut buf,
                false,
                256,
                100,
                1,
                None,
                &mut false
            ),
            Err(UdsError::Chunk(_))
        ));
        close(a);
        close(b);
    }

    #[test]
    fn test_one_byte_transfers_and_eintr_yield_whole_packets() {
        let (a, b) = stream_pair();
        let pkt = outer(5, b"hello");

        // 36 one-byte writes leave one byte for the last call; reads of more
        // than one byte are cut to one, 31 in the header and 4 in the body.
        short_io::SENDMSG.with(|c| c.set(pkt.len() as u32 - 1));
        eintr_inject::SENDMSG.with(|c| c.set(2));
        send_all(a, &pkt[..HEADER_SIZE], &pkt[HEADER_SIZE..]).unwrap();

        short_io::RECV.with(|c| c.set(pkt.len() as u32 - 2));
        eintr_inject::RECV.with(|c| c.set(2));
        let mut buf = [0u8; 64];
        assert_eq!(recv_packet(b, &mut buf, 64, None).unwrap(), pkt.len());
        assert_eq!(&buf[..pkt.len()], &pkt[..]);
        assert_eq!(short_io::pending() + eintr_inject::pending(), 0);
        close(a);
        close(b);
    }
}
