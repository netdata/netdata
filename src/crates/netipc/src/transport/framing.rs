//! Framing shared by the UDS and Named Pipe transports.
//!
//! Message send with transparent chunking, receive with reassembly and
//! validation, and the HELLO / HELLO_ACK negotiation. Each transport supplies
//! its packet I/O and its error type; everything on the wire is decided here.

use crate::protocol::{
    self, ChunkHeader, FLAG_BATCH, HEADER_SIZE, Header, Hello, HelloAck, KIND_REQUEST,
    KIND_RESPONSE, MAGIC_CHUNK, MAGIC_MSG, NipcError, VERSION, align8,
};
use std::collections::HashSet;

pub(crate) const HELLO_PAYLOAD_SIZE: usize = 44;
pub(crate) const HELLO_ACK_PAYLOAD_SIZE: usize = 48;

/// The transport errors the shared framing reports.
pub(crate) trait FramingError: Sized {
    fn protocol(msg: String) -> Self;
    fn chunk(msg: String) -> Self;
    fn incompatible(msg: String) -> Self;
    fn handshake(msg: String) -> Self;
    fn bad_param(msg: String) -> Self;
    fn limit_exceeded() -> Self;
    fn auth_failed() -> Self;
    fn no_profile() -> Self;
    fn duplicate_msg_id(id: u64) -> Self;
    fn unknown_msg_id(id: u64) -> Self;
    /// True when a failed send leaves the session unusable, so every
    /// in-flight request fails with it.
    fn fails_session(&self) -> bool;
    /// True when a failed receive means the peer is gone, so every in-flight
    /// request fails with it. A timeout or abort leaves them pending.
    fn is_recv_failure(&self) -> bool;
}

/// One direction of an established session: the limits for the messages
/// this side sends (or receives) and the agreed packet size.
#[derive(Clone, Copy)]
pub(crate) struct Framing {
    pub client: bool,
    pub max_payload: u32,
    pub max_items: u32,
    pub packet_size: u32,
}

pub(crate) fn header_version_incompatible(buf: &[u8], expected_code: u16) -> bool {
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

pub(crate) fn hello_layout_incompatible(buf: &[u8]) -> bool {
    buf.len() >= 2 && u16::from_ne_bytes(buf[0..2].try_into().unwrap()) != 1
}

pub(crate) fn hello_ack_layout_incompatible(buf: &[u8]) -> bool {
    buf.len() >= 2 && u16::from_ne_bytes(buf[0..2].try_into().unwrap()) != 1
}

/// Highest set bit in a bitmask (0 if empty).
pub(crate) fn highest_bit(mask: u32) -> u32 {
    if mask == 0 {
        return 0;
    }
    1u32 << (31 - mask.leading_zeros())
}

pub(crate) fn apply_default(val: u32, def: u32) -> u32 {
    if val == 0 { def } else { val }
}

// ---------------------------------------------------------------------------
//  Send / receive
// ---------------------------------------------------------------------------

/// Send one logical message. `hdr` is the caller's outer header (kind, code,
/// flags, item_count, message_id); this sets magic/version/header_len/
/// payload_len. A message larger than the packet size goes out in chunks.
/// `write` sends one packet made of a 32-byte header and a payload slice.
pub(crate) fn send_message<E: FramingError>(
    f: Framing,
    inflight: &mut HashSet<u64>,
    hdr: &mut Header,
    payload: &[u8],
    mut write: impl FnMut(&[u8], &[u8]) -> Result<(), E>,
) -> Result<(), E> {
    // Validate payload against negotiated directional limits before transmitting.
    // The u32 cast below requires payload.len() <= u32::MAX.
    if payload.len() > f.max_payload as usize
        || payload.len() > (u32::MAX as usize).saturating_sub(HEADER_SIZE)
    {
        return Err(E::limit_exceeded());
    }
    if hdr.item_count > f.max_items {
        return Err(E::limit_exceeded());
    }

    // Client-side: track in-flight message_ids for requests
    let tracked = f.client && hdr.kind == KIND_REQUEST;
    if tracked && !inflight.insert(hdr.message_id) {
        return Err(E::duplicate_msg_id(hdr.message_id));
    }

    // Fill envelope fields
    hdr.magic = MAGIC_MSG;
    hdr.version = VERSION;
    hdr.header_len = protocol::HEADER_LEN;
    hdr.payload_len = payload.len() as u32;

    let result = send_packets(f.packet_size, hdr, payload, &mut write);

    if let Err(err) = &result
        && tracked
    {
        if err.fails_session() {
            inflight.clear();
        } else {
            inflight.remove(&hdr.message_id);
        }
    }

    result
}

fn send_packets<E: FramingError>(
    packet_size: u32,
    hdr: &Header,
    payload: &[u8],
    write: &mut impl FnMut(&[u8], &[u8]) -> Result<(), E>,
) -> Result<(), E> {
    let total_msg = HEADER_SIZE + payload.len();
    // total_message_len is a u32 wire field; larger totals are
    // unrepresentable (parity with the C nipc_uds_header_payload_len).
    if total_msg > u32::MAX as usize {
        return Err(E::bad_param(
            "total message length exceeds protocol limit".into(),
        ));
    }

    // Single packet?
    if total_msg <= packet_size as usize {
        let mut hdr_buf = [0u8; HEADER_SIZE];
        hdr.encode(&mut hdr_buf);
        return write(&hdr_buf, payload);
    }

    // Chunked send
    let chunk_payload_budget = packet_size as usize - HEADER_SIZE;
    if chunk_payload_budget == 0 {
        return Err(E::bad_param("packet_size too small".into()));
    }

    let first_chunk_payload = payload.len().min(chunk_payload_budget);
    let remaining_after_first = payload.len() - first_chunk_payload;

    let continuation_chunks = if remaining_after_first > 0 {
        1 + ((remaining_after_first - 1) / chunk_payload_budget)
    } else {
        0
    };
    let chunk_count = 1 + continuation_chunks as u32;

    // First chunk: outer header + first part of payload
    let mut hdr_buf = [0u8; HEADER_SIZE];
    hdr.encode(&mut hdr_buf);
    write(&hdr_buf, &payload[..first_chunk_payload])?;

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
        write(&chk_buf, &payload[offset..offset + this_chunk])?;

        offset += this_chunk;
    }

    Ok(())
}

/// Receive one logical message. `read` reads one packet into the buffer it
/// is given and returns its length. The first packet lands in `first`; a
/// chunked message is reassembled in `recv_buf`, its continuations read
/// through `pkt_buf`. The returned payload borrows one of the two.
pub(crate) fn receive_message<'a, E: FramingError>(
    f: Framing,
    inflight: &mut HashSet<u64>,
    first: &'a mut [u8],
    recv_buf: &'a mut Vec<u8>,
    pkt_buf: &mut Vec<u8>,
    mut read: impl FnMut(&mut [u8]) -> Result<usize, E>,
) -> Result<(Header, &'a [u8]), E> {
    // Read first packet. A lost peer ends the session's pending requests.
    let n = read(first).inspect_err(|e| {
        if f.client && e.is_recv_failure() {
            inflight.clear();
        }
    })?;

    if n < HEADER_SIZE {
        return Err(E::protocol("packet too short for header".into()));
    }

    let hdr =
        Header::decode(&first[..n]).map_err(|e| E::protocol(format!("header decode: {e}")))?;

    // Validate payload_len and item_count against the negotiated limits of
    // this direction: a server receives requests, a client responses.
    if hdr.payload_len > f.max_payload {
        return Err(E::limit_exceeded());
    }
    if hdr.item_count > f.max_items {
        return Err(E::limit_exceeded());
    }

    // Client-side: validate response message_id is in-flight
    if f.client && hdr.kind == KIND_RESPONSE && !inflight.remove(&hdr.message_id) {
        return Err(E::unknown_msg_id(hdr.message_id));
    }

    // total_message_len is a u32 wire field; reject unrepresentable totals
    // so the chunk-header comparison below cannot truncate-match (parity
    // with the C nipc_uds_header_payload_len). The sum is checked because
    // it overflows a 32-bit usize.
    let Some(total_msg) = HEADER_SIZE
        .checked_add(hdr.payload_len as usize)
        .filter(|&total| total <= u32::MAX as usize)
    else {
        return Err(E::protocol(
            "total message length exceeds protocol limit".into(),
        ));
    };

    if n > total_msg {
        return Err(E::protocol("packet exceeds declared payload_len".into()));
    }

    // Non-chunked: entire message in one packet
    if n == total_msg {
        let payload = &first[HEADER_SIZE..total_msg];
        validate_batch_dir(&hdr, payload)?;
        return Ok((hdr, payload));
    }

    // Chunked: first packet has partial payload
    let first_payload_bytes = n - HEADER_SIZE;

    // Grow recv_buf to hold full payload
    let needed = hdr.payload_len as usize;
    if recv_buf.len() < needed {
        recv_buf.resize(needed, 0);
    }

    // Copy first chunk's payload
    recv_buf[..first_payload_bytes].copy_from_slice(&first[HEADER_SIZE..n]);

    let mut assembled = first_payload_bytes;
    let chunk_payload_budget = f.packet_size as usize - HEADER_SIZE;

    // Expected chunk count
    let remaining_after_first = needed - first_payload_bytes;
    let expected_continuations = if remaining_after_first > 0 && chunk_payload_budget > 0 {
        1 + ((remaining_after_first - 1) / chunk_payload_budget)
    } else {
        0
    };
    let expected_chunk_count = 1 + expected_continuations as u32;

    if pkt_buf.len() < f.packet_size as usize {
        pkt_buf.resize(f.packet_size as usize, 0);
    }

    let mut ci = 1u32;
    while assembled < needed {
        let cn = read(pkt_buf).inspect_err(|e| {
            if f.client && e.is_recv_failure() {
                inflight.clear();
            }
        })?;

        if cn < HEADER_SIZE {
            return Err(E::chunk("continuation too short".into()));
        }

        let chk = ChunkHeader::decode(&pkt_buf[..cn])
            .map_err(|e| E::chunk(format!("chunk header: {e}")))?;

        // Validate chunk header
        if chk.message_id != hdr.message_id {
            return Err(E::chunk("message_id mismatch".into()));
        }
        if chk.chunk_index != ci {
            return Err(E::chunk(format!(
                "chunk_index mismatch: expected {ci}, got {}",
                chk.chunk_index
            )));
        }
        if chk.chunk_count != expected_chunk_count {
            return Err(E::chunk("chunk_count mismatch".into()));
        }
        // A sender that uses smaller chunks than the budget would need more
        // chunks than the count it must declare.
        if ci >= expected_chunk_count {
            return Err(E::chunk(format!(
                "chunk_index {ci} beyond chunk_count {expected_chunk_count}"
            )));
        }
        if chk.total_message_len != total_msg as u32 {
            return Err(E::chunk("total_message_len mismatch".into()));
        }

        let chunk_data = cn - HEADER_SIZE;
        if chunk_data != chk.chunk_payload_len as usize {
            return Err(E::chunk("chunk_payload_len mismatch".into()));
        }
        if chunk_data > needed - assembled {
            return Err(E::chunk("chunk exceeds payload_len".into()));
        }

        recv_buf[assembled..assembled + chunk_data].copy_from_slice(&pkt_buf[HEADER_SIZE..cn]);
        assembled += chunk_data;
        ci += 1;
    }

    let payload = &recv_buf[..needed];
    validate_batch_dir(&hdr, payload)?;
    Ok((hdr, payload))
}

fn validate_batch_dir<E: FramingError>(hdr: &Header, payload: &[u8]) -> Result<(), E> {
    if hdr.flags & FLAG_BATCH == 0 || hdr.item_count <= 1 {
        return Ok(());
    }
    let dir_bytes = hdr.item_count as usize * 8;
    let dir_aligned = align8(dir_bytes);
    if payload.len() < dir_aligned {
        return Err(E::protocol("batch directory exceeds payload".into()));
    }
    let packed_area_len = (payload.len() - dir_aligned) as u32;
    protocol::batch_dir_validate(&payload[..dir_bytes], hdr.item_count, packed_area_len)
        .map_err(|e| E::protocol(format!("batch directory: {e:?}")))
}

// ---------------------------------------------------------------------------
//  Handshake
// ---------------------------------------------------------------------------

fn control_header(code: u16, status: u16, payload_len: usize) -> [u8; HEADER_SIZE] {
    let hdr = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: protocol::HEADER_LEN,
        kind: protocol::KIND_CONTROL,
        flags: 0,
        code,
        transport_status: status,
        payload_len: payload_len as u32,
        item_count: 1,
        message_id: 0,
    };
    let mut buf = [0u8; HEADER_SIZE];
    hdr.encode(&mut buf);
    buf
}

/// The HELLO packet a client sends.
pub(crate) fn hello_packet(hello: &Hello) -> [u8; HEADER_SIZE + HELLO_PAYLOAD_SIZE] {
    let mut pkt = [0u8; HEADER_SIZE + HELLO_PAYLOAD_SIZE];
    pkt[..HEADER_SIZE].copy_from_slice(&control_header(
        protocol::CODE_HELLO,
        protocol::STATUS_OK,
        HELLO_PAYLOAD_SIZE,
    ));
    hello.encode(&mut pkt[HEADER_SIZE..]);
    pkt
}

fn hello_ack_packet(ack: &HelloAck, status: u16) -> [u8; HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE] {
    let mut pkt = [0u8; HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE];
    pkt[..HEADER_SIZE].copy_from_slice(&control_header(
        protocol::CODE_HELLO_ACK,
        status,
        HELLO_ACK_PAYLOAD_SIZE,
    ));
    ack.encode(&mut pkt[HEADER_SIZE..]);
    pkt
}

/// Decode and check the HELLO_ACK packet `buf` answering a HELLO that
/// offered the `offered` profiles. The agreed packet size is left to the
/// transport, whose limits differ.
pub(crate) fn parse_hello_ack<E: FramingError>(buf: &[u8], offered: u32) -> Result<HelloAck, E> {
    let ack_hdr = match Header::decode(buf) {
        Ok(hdr) => hdr,
        Err(NipcError::BadVersion) => {
            return Err(E::incompatible("ack header version mismatch".into()));
        }
        Err(e) => return Err(E::protocol(format!("ack header: {e}"))),
    };

    if ack_hdr.kind != protocol::KIND_CONTROL || ack_hdr.code != protocol::CODE_HELLO_ACK {
        return Err(E::protocol("expected HELLO_ACK".into()));
    }

    // Check transport_status for rejection
    match ack_hdr.transport_status {
        protocol::STATUS_OK => {}
        protocol::STATUS_AUTH_FAILED => return Err(E::auth_failed()),
        protocol::STATUS_UNSUPPORTED => return Err(E::no_profile()),
        protocol::STATUS_INCOMPATIBLE => {
            return Err(E::incompatible("ack transport_status incompatible".into()));
        }
        protocol::STATUS_LIMIT_EXCEEDED => return Err(E::limit_exceeded()),
        status => return Err(E::handshake(format!("transport_status={status}"))),
    }

    // Decode hello-ack payload
    if buf.len() < HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE {
        return Err(E::protocol("ack payload truncated".into()));
    }
    let payload = &buf[HEADER_SIZE..];
    let ack = match HelloAck::decode(payload) {
        Ok(ack) => ack,
        Err(NipcError::BadLayout) if hello_ack_layout_incompatible(payload) => {
            return Err(E::incompatible(
                "ack payload layout version mismatch".into(),
            ));
        }
        Err(e) => return Err(E::protocol(format!("ack payload: {e}"))),
    };
    // HELLO_ACK is exactly one 48-byte payload: extra bytes in the packet, or
    // a declared length a stream peer may still be sending, would otherwise
    // be read as the session's first message.
    if ack_hdr.payload_len as usize != HELLO_ACK_PAYLOAD_SIZE
        || buf.len() != HEADER_SIZE + HELLO_ACK_PAYLOAD_SIZE
    {
        return Err(E::protocol("ack payload length mismatch".into()));
    }

    if !super::selected_profile_offered(ack.selected_profile, offered) {
        return Err(E::protocol("selected profile was not offered".into()));
    }

    Ok(ack)
}

/// What a server offers in the handshake, with defaults applied and the
/// profiles already limited to what the platform provides.
pub(crate) struct ServerOffer {
    pub packet_size: u32,
    pub supported_profiles: u32,
    pub preferred_profiles: u32,
    pub max_request_payload_bytes: u32,
    pub max_response_payload_bytes: u32,
    pub auth_token: u64,
}

/// Answer the HELLO packet `buf` with HELLO_ACK. On success returns the ACK
/// sent, which holds the session's agreed values. Rejections are sent on a
/// best-effort basis; only a failed success ACK is reported as a send error.
///
/// Once a profile is selected and the payload limits are agreed, and before
/// HELLO_ACK is sent, `prepare(profile, max_request_payload_bytes,
/// max_response_payload_bytes)` sets up that profile's resources. Returning
/// false drops the profile and selection runs again.
pub(crate) fn server_negotiate<E: FramingError>(
    buf: &[u8],
    offer: &ServerOffer,
    session_id: u64,
    mut prepare: Option<&mut dyn FnMut(u32, u32, u32) -> bool>,
    mut send: impl FnMut(&[u8]) -> Result<(), E>,
) -> Result<HelloAck, E> {
    let mut send_rejection = |status: u16| {
        let ack = HelloAck {
            layout_version: 1,
            ..HelloAck::default()
        };
        let _ = send(&hello_ack_packet(&ack, status));
    };

    let hdr = match Header::decode(buf) {
        Ok(hdr) => hdr,
        Err(NipcError::BadVersion) if header_version_incompatible(buf, protocol::CODE_HELLO) => {
            send_rejection(protocol::STATUS_INCOMPATIBLE);
            return Err(E::incompatible("hello header version mismatch".into()));
        }
        Err(e) => return Err(E::protocol(format!("hello header: {e}"))),
    };

    if hdr.kind != protocol::KIND_CONTROL || hdr.code != protocol::CODE_HELLO {
        return Err(E::protocol("expected HELLO".into()));
    }

    let payload = &buf[HEADER_SIZE..];
    let hello = match Hello::decode(payload) {
        Ok(hello) => hello,
        Err(NipcError::BadLayout) if hello_layout_incompatible(payload) => {
            send_rejection(protocol::STATUS_INCOMPATIBLE);
            return Err(E::incompatible(
                "hello payload layout version mismatch".into(),
            ));
        }
        Err(e) => return Err(E::protocol(format!("hello payload: {e}"))),
    };
    // HELLO is exactly one 44-byte payload, as HELLO_ACK is 48.
    if hdr.payload_len as usize != HELLO_PAYLOAD_SIZE
        || buf.len() != HEADER_SIZE + HELLO_PAYLOAD_SIZE
    {
        return Err(E::protocol("hello payload length mismatch".into()));
    }

    let mut s_profiles = offer.supported_profiles;
    let mut intersection = hello.supported_profiles & s_profiles;
    if intersection == 0 {
        send_rejection(protocol::STATUS_UNSUPPORTED);
        return Err(E::no_profile());
    }

    if hello.auth_token != offer.auth_token {
        send_rejection(protocol::STATUS_AUTH_FAILED);
        return Err(E::auth_failed());
    }

    // Select profile
    let select = |intersection: u32| {
        let preferred = intersection & hello.preferred_profiles & offer.preferred_profiles;
        highest_bit(if preferred != 0 {
            preferred
        } else {
            intersection
        })
    };
    let mut selected = select(intersection);

    if hello.max_request_payload_bytes > offer.max_request_payload_bytes {
        send_rejection(protocol::STATUS_LIMIT_EXCEEDED);
        return Err(E::limit_exceeded());
    }

    // Negotiate limits:
    // - request payload and batch size are client-proposed and echoed
    // - response payload is server-authoritative
    // - response batch size is symmetric with request batch size
    let agreed_req_pay = hello.max_request_payload_bytes;
    let agreed_req_bat = hello.max_request_batch_items;
    let agreed_resp_pay = offer.max_response_payload_bytes;
    let agreed_resp_bat = agreed_req_bat;
    let agreed_pkt = hello.packet_size.min(offer.packet_size);

    // packet_size must be large enough for a usable message packet
    if agreed_pkt <= HEADER_SIZE as u32 {
        send_rejection(protocol::STATUS_INCOMPATIBLE);
        return Err(E::incompatible(
            "packet size too small for negotiated session".into(),
        ));
    }

    // Set up the selected profile before the ACK promises it. A profile the
    // caller cannot prepare is dropped, as if the server never offered it.
    if let Some(prepare) = prepare.as_mut() {
        while !prepare(selected, agreed_req_pay, agreed_resp_pay) {
            s_profiles &= !selected;
            intersection &= !selected;
            if intersection == 0 {
                send_rejection(protocol::STATUS_UNSUPPORTED);
                return Err(E::no_profile());
            }
            selected = select(intersection);
        }
    }

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
    send(&hello_ack_packet(&ack, protocol::STATUS_OK))?;
    Ok(ack)
}

#[cfg(test)]
mod tests {
    use super::*;

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

    /// `pkt` with 16 more bytes, the header declaring them too when `declare`.
    fn padded(pkt: &[u8], declare: bool) -> Vec<u8> {
        let mut out = pkt.to_vec();
        out.extend_from_slice(&[0u8; 16]);
        if declare {
            let len = (pkt.len() - HEADER_SIZE + 16) as u32;
            out[16..20].copy_from_slice(&len.to_ne_bytes());
        }
        out
    }

    /// HELLO and HELLO_ACK are exactly 44 and 48 payload bytes: a packet with
    /// more, declared or not, is a protocol error.
    #[test]
    fn test_handshake_payload_length_exact() {
        let ack = HelloAck {
            layout_version: 1,
            selected_profile: protocol::PROFILE_BASELINE,
            agreed_packet_size: 4096,
            ..HelloAck::default()
        };
        let pkt = hello_ack_packet(&ack, protocol::STATUS_OK);
        assert!(parse_hello_ack::<TestError>(&pkt, protocol::PROFILE_BASELINE).is_ok());
        for declare in [false, true] {
            assert!(matches!(
                parse_hello_ack::<TestError>(&padded(&pkt, declare), protocol::PROFILE_BASELINE),
                Err(TestError::Protocol(_))
            ));
        }

        let offer = ServerOffer {
            packet_size: 4096,
            supported_profiles: protocol::PROFILE_BASELINE,
            preferred_profiles: 0,
            max_request_payload_bytes: 1024,
            max_response_payload_bytes: 1024,
            auth_token: 0,
        };
        let hello = Hello {
            layout_version: 1,
            supported_profiles: protocol::PROFILE_BASELINE,
            max_request_payload_bytes: 1024,
            max_request_batch_items: 1,
            max_response_payload_bytes: 1024,
            max_response_batch_items: 1,
            packet_size: 4096,
            ..Hello::default()
        };
        let pkt = hello_packet(&hello);
        assert!(server_negotiate::<TestError>(&pkt, &offer, 1, None, |_| Ok(())).is_ok());
        for declare in [false, true] {
            assert!(matches!(
                server_negotiate::<TestError>(&padded(&pkt, declare), &offer, 1, None, |_| Ok(())),
                Err(TestError::Protocol(_))
            ));
        }
    }

    #[test]
    fn test_highest_bit() {
        assert_eq!(highest_bit(0), 0);
        assert_eq!(highest_bit(1), 1);
        assert_eq!(highest_bit(0b0101), 4);
        assert_eq!(highest_bit(0b1000), 8);
        assert_eq!(highest_bit(0xFF), 128);
        assert_eq!(highest_bit(u32::MAX), 1 << 31);
    }

    #[derive(Debug, PartialEq)]
    enum TestError {
        Protocol(String),
        Other,
    }

    impl FramingError for TestError {
        fn protocol(msg: String) -> Self {
            TestError::Protocol(msg)
        }
        fn chunk(_: String) -> Self {
            TestError::Other
        }
        fn incompatible(_: String) -> Self {
            TestError::Other
        }
        fn handshake(_: String) -> Self {
            TestError::Other
        }
        fn bad_param(_: String) -> Self {
            TestError::Other
        }
        fn limit_exceeded() -> Self {
            TestError::Other
        }
        fn auth_failed() -> Self {
            TestError::Other
        }
        fn no_profile() -> Self {
            TestError::Other
        }
        fn duplicate_msg_id(_: u64) -> Self {
            TestError::Other
        }
        fn unknown_msg_id(_: u64) -> Self {
            TestError::Other
        }
        fn fails_session(&self) -> bool {
            false
        }
        fn is_recv_failure(&self) -> bool {
            false
        }
    }

    #[test]
    fn test_receive_rejects_unrepresentable_total_length() {
        // With a u32::MAX payload limit, a payload_len near u32::MAX passes
        // the limit check, but 32 + payload_len does not fit the u32
        // total_message_len of chunk headers.
        let hdr = Header {
            magic: MAGIC_MSG,
            version: VERSION,
            header_len: protocol::HEADER_LEN,
            kind: protocol::KIND_REQUEST,
            payload_len: u32::MAX - 8,
            item_count: 1,
            ..Header::default()
        };
        let mut packet = [0u8; HEADER_SIZE];
        hdr.encode(&mut packet);

        let f = Framing {
            client: false,
            max_payload: u32::MAX,
            max_items: 1,
            packet_size: 65536,
        };
        let mut first = [0u8; 64];
        let mut recv_buf = Vec::new();
        let mut pkt_buf = Vec::new();
        let result = receive_message(
            f,
            &mut HashSet::new(),
            &mut first,
            &mut recv_buf,
            &mut pkt_buf,
            |buf: &mut [u8]| -> Result<usize, TestError> {
                buf[..HEADER_SIZE].copy_from_slice(&packet);
                Ok(HEADER_SIZE)
            },
        );
        assert_eq!(
            result.unwrap_err(),
            TestError::Protocol("total message length exceeds protocol limit".into())
        );
        // Rejected before reassembly reserved the declared payload.
        assert!(recv_buf.is_empty());
    }

    #[test]
    fn test_apply_default() {
        assert_eq!(apply_default(0, 42), 42);
        assert_eq!(apply_default(10, 42), 10);
    }
}
