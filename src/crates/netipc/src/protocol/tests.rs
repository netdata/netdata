use super::*;

// -----------------------------------------------------------------------
//  Outer message header tests
// -----------------------------------------------------------------------

#[test]
fn header_roundtrip() {
    let h = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: HEADER_LEN,
        kind: KIND_REQUEST,
        flags: FLAG_BATCH,
        code: METHOD_CGROUPS_SNAPSHOT,
        transport_status: STATUS_OK,
        payload_len: 12345,
        item_count: 42,
        message_id: 0xDEAD_BEEF_CAFE_BABE,
    };

    let mut buf = [0u8; 64];
    let n = h.encode(&mut buf);
    assert_eq!(n, 32);

    let out = Header::decode(&buf[..n]).unwrap();
    assert_eq!(out, h);
}

#[test]
fn header_encode_too_small() {
    let h = Header::default();
    let mut buf = [0u8; 16];
    assert_eq!(h.encode(&mut buf), 0);
}

#[test]
fn header_decode_truncated() {
    let buf = [0u8; 31];
    assert_eq!(Header::decode(&buf), Err(NipcError::Truncated));
}

#[test]
fn header_decode_bad_magic() {
    let h = Header {
        magic: 0x12345678,
        version: VERSION,
        header_len: HEADER_LEN,
        kind: KIND_REQUEST,
        ..Default::default()
    };
    let mut buf = [0u8; 32];
    h.encode(&mut buf);
    assert_eq!(Header::decode(&buf), Err(NipcError::BadMagic));
}

#[test]
fn header_decode_bad_version() {
    let h = Header {
        magic: MAGIC_MSG,
        version: 99,
        header_len: HEADER_LEN,
        kind: KIND_REQUEST,
        ..Default::default()
    };
    let mut buf = [0u8; 32];
    h.encode(&mut buf);
    assert_eq!(Header::decode(&buf), Err(NipcError::BadVersion));
}

#[test]
fn header_decode_bad_header_len() {
    let h = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: 64,
        kind: KIND_REQUEST,
        ..Default::default()
    };
    let mut buf = [0u8; 32];
    h.encode(&mut buf);
    assert_eq!(Header::decode(&buf), Err(NipcError::BadHeaderLen));
}

#[test]
fn header_decode_bad_kind() {
    // kind = 0
    let h = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: HEADER_LEN,
        kind: 0,
        ..Default::default()
    };
    let mut buf = [0u8; 32];
    h.encode(&mut buf);
    assert_eq!(Header::decode(&buf), Err(NipcError::BadKind));

    // kind = 4
    let h2 = Header { kind: 4, ..h };
    h2.encode(&mut buf);
    assert_eq!(Header::decode(&buf), Err(NipcError::BadKind));
}

#[test]
fn header_all_kinds() {
    for k in KIND_REQUEST..=KIND_CONTROL {
        let h = Header {
            magic: MAGIC_MSG,
            version: VERSION,
            header_len: HEADER_LEN,
            kind: k,
            ..Default::default()
        };
        let mut buf = [0u8; 32];
        h.encode(&mut buf);
        let out = Header::decode(&buf).unwrap();
        assert_eq!(out.kind, k);
    }
}

#[test]
fn header_wire_bytes() {
    let h = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: HEADER_LEN,
        kind: KIND_REQUEST,
        flags: 0,
        code: METHOD_CGROUPS_SNAPSHOT,
        transport_status: STATUS_OK,
        payload_len: 4,
        item_count: 1,
        message_id: 1,
    };

    let mut buf = [0u8; 32];
    h.encode(&mut buf);

    // magic = 0x4e495043 LE: 43 50 49 4e
    assert_eq!(&buf[0..4], &[0x43, 0x50, 0x49, 0x4e]);
    // version = 1 LE: 01 00
    assert_eq!(&buf[4..6], &[0x01, 0x00]);
    // header_len = 32 LE: 20 00
    assert_eq!(&buf[6..8], &[0x20, 0x00]);
    // kind = 1 LE: 01 00
    assert_eq!(&buf[8..10], &[0x01, 0x00]);
    // code = 2 LE: 02 00
    assert_eq!(&buf[12..14], &[0x02, 0x00]);
}

// -----------------------------------------------------------------------
//  Chunk continuation header tests
// -----------------------------------------------------------------------

#[test]
fn chunk_header_roundtrip() {
    let c = ChunkHeader {
        magic: MAGIC_CHUNK,
        version: VERSION,
        flags: 0,
        message_id: 0x1234_5678_90AB_CDEF,
        total_message_len: 100000,
        chunk_index: 3,
        chunk_count: 10,
        chunk_payload_len: 8192,
    };

    let mut buf = [0u8; 64];
    let n = c.encode(&mut buf);
    assert_eq!(n, 32);

    let out = ChunkHeader::decode(&buf[..n]).unwrap();
    assert_eq!(out, c);
}

#[test]
fn chunk_decode_truncated() {
    let buf = [0u8; 31];
    assert_eq!(ChunkHeader::decode(&buf), Err(NipcError::Truncated));
}

#[test]
fn chunk_decode_bad_magic() {
    let c = ChunkHeader {
        magic: MAGIC_MSG, // wrong magic for chunk
        version: VERSION,
        ..Default::default()
    };
    let mut buf = [0u8; 32];
    c.encode(&mut buf);
    assert_eq!(ChunkHeader::decode(&buf), Err(NipcError::BadMagic));
}

#[test]
fn chunk_decode_bad_version() {
    let c = ChunkHeader {
        magic: MAGIC_CHUNK,
        version: 2,
        ..Default::default()
    };
    let mut buf = [0u8; 32];
    c.encode(&mut buf);
    assert_eq!(ChunkHeader::decode(&buf), Err(NipcError::BadVersion));
}

#[test]
fn chunk_encode_too_small() {
    let c = ChunkHeader::default();
    let mut buf = [0u8; 16];
    assert_eq!(c.encode(&mut buf), 0);
}

#[test]
fn chunk_wire_bytes() {
    let c = ChunkHeader {
        magic: MAGIC_CHUNK,
        version: VERSION,
        flags: 0,
        message_id: 1,
        total_message_len: 256,
        chunk_index: 1,
        chunk_count: 3,
        chunk_payload_len: 100,
    };

    let mut buf = [0u8; 32];
    c.encode(&mut buf);

    // magic = 0x4e43484b LE: 4b 48 43 4e
    assert_eq!(&buf[0..4], &[0x4b, 0x48, 0x43, 0x4e]);
}

// -----------------------------------------------------------------------
//  Batch item directory tests
// -----------------------------------------------------------------------

#[test]
fn batch_dir_roundtrip() {
    let entries = [
        BatchEntry {
            offset: 0,
            length: 100,
        },
        BatchEntry {
            offset: 104,
            length: 200,
        },
        BatchEntry {
            offset: 304,
            length: 50,
        },
    ];

    let mut buf = [0u8; 64];
    let n = batch_dir_encode(&entries, &mut buf);
    assert_eq!(n, 24);

    let out = batch_dir_decode(&buf[..n], 3, 400).unwrap();
    assert_eq!(out[0], entries[0]);
    assert_eq!(out[1], entries[1]);
    assert_eq!(out[2], entries[2]);
}

#[test]
fn batch_dir_decode_truncated() {
    let buf = [0u8; 12];
    assert_eq!(batch_dir_decode(&buf, 2, 1000), Err(NipcError::Truncated));
}

#[test]
fn batch_dir_decode_oob() {
    let e = BatchEntry {
        offset: 0,
        length: 200,
    };
    let mut buf = [0u8; 8];
    batch_dir_encode(&[e], &mut buf);
    assert_eq!(batch_dir_decode(&buf, 1, 100), Err(NipcError::OutOfBounds));
}

#[test]
fn batch_dir_decode_bad_alignment() {
    let mut buf = [0u8; 8];
    // Manually write unaligned offset
    buf[0..4].copy_from_slice(&3u32.to_ne_bytes());
    buf[4..8].copy_from_slice(&10u32.to_ne_bytes());
    assert_eq!(batch_dir_decode(&buf, 1, 100), Err(NipcError::BadAlignment));
}

// -----------------------------------------------------------------------
//  Batch builder + extraction tests
// -----------------------------------------------------------------------

#[test]
fn batch_builder_roundtrip() {
    let mut buf = [0u8; 1024];
    let mut b = BatchBuilder::new(&mut buf, 4);

    let item1 = [1u8, 2, 3, 4, 5];
    let item2 = [10u8, 20, 30];
    let item3 = [0xAAu8, 0xBB];

    b.add(&item1).unwrap();
    b.add(&item2).unwrap();
    b.add(&item3).unwrap();

    let (total, count) = b.finish();
    assert_eq!(count, 3);
    assert!(total > 0);

    // Extract items
    let (data, len) = batch_item_get(&buf[..total], 3, 0).unwrap();
    assert_eq!(len as usize, item1.len());
    assert_eq!(data, &item1);

    let (data, len) = batch_item_get(&buf[..total], 3, 1).unwrap();
    assert_eq!(len as usize, item2.len());
    assert_eq!(data, &item2);

    let (data, len) = batch_item_get(&buf[..total], 3, 2).unwrap();
    assert_eq!(len as usize, item3.len());
    assert_eq!(data, &item3);
}

#[test]
fn batch_builder_overflow() {
    let mut buf = [0u8; 32];
    let mut b = BatchBuilder::new(&mut buf, 1);
    let item = [1u8];
    b.add(&item).unwrap();
    assert_eq!(b.add(&item), Err(NipcError::Overflow));
}

#[test]
fn batch_builder_buf_overflow() {
    let mut buf = [0u8; 24];
    let mut b = BatchBuilder::new(&mut buf, 1);
    let big = [0u8; 100];
    assert_eq!(b.add(&big), Err(NipcError::Overflow));
}

#[test]
fn batch_item_get_oob_index() {
    let mut buf = [0u8; 64];
    let mut b = BatchBuilder::new(&mut buf, 2);
    b.add(&[1u8]).unwrap();
    let (total, count) = b.finish();
    assert_eq!(
        batch_item_get(&buf[..total], count, 5),
        Err(NipcError::OutOfBounds)
    );
}

#[test]
fn batch_empty() {
    let mut buf = [0u8; 64];
    let b = BatchBuilder::new(&mut buf, 4);
    let (total, count) = b.finish();
    assert_eq!(count, 0);
    assert_eq!(total, 0);
}

// -----------------------------------------------------------------------
//  Hello payload tests
// -----------------------------------------------------------------------

#[test]
fn hello_roundtrip() {
    let h = Hello {
        layout_version: 1,
        flags: 0,
        supported_profiles: PROFILE_BASELINE | PROFILE_SHM_FUTEX,
        preferred_profiles: PROFILE_SHM_FUTEX,
        max_request_payload_bytes: 4096,
        max_request_batch_items: 100,
        max_response_payload_bytes: 1048576,
        max_response_batch_items: 1,
        auth_token: 0xAABB_CCDD_EEFF_0011,
        packet_size: 65536,
    };

    let mut buf = [0u8; 64];
    let n = h.encode(&mut buf);
    assert_eq!(n, 44);

    let out = Hello::decode(&buf[..n]).unwrap();
    assert_eq!(out, h);
}

#[test]
fn hello_decode_truncated() {
    let buf = [0u8; 43];
    assert_eq!(Hello::decode(&buf), Err(NipcError::Truncated));
}

#[test]
fn hello_decode_bad_layout() {
    let h = Hello {
        layout_version: 99,
        ..Default::default()
    };
    let mut buf = [0u8; 44];
    h.encode(&mut buf);
    assert_eq!(Hello::decode(&buf), Err(NipcError::BadLayout));
}

#[test]
fn hello_encode_too_small() {
    let h = Hello::default();
    let mut buf = [0u8; 10];
    assert_eq!(h.encode(&mut buf), 0);
}

// -----------------------------------------------------------------------
//  Hello-ack payload tests
// -----------------------------------------------------------------------

#[test]
fn hello_ack_roundtrip() {
    let h = HelloAck {
        layout_version: 1,
        flags: 0,
        server_supported_profiles: 0x07,
        intersection_profiles: 0x05,
        selected_profile: PROFILE_SHM_FUTEX,
        agreed_max_request_payload_bytes: 2048,
        agreed_max_request_batch_items: 50,
        agreed_max_response_payload_bytes: 65536,
        agreed_max_response_batch_items: 1,
        agreed_packet_size: 32768,
        session_id: 42,
    };

    let mut buf = [0u8; 64];
    let n = h.encode(&mut buf);
    assert_eq!(n, 48);

    let out = HelloAck::decode(&buf[..n]).unwrap();
    assert_eq!(out, h);
}

#[test]
fn hello_ack_decode_truncated() {
    let buf = [0u8; 47];
    assert_eq!(HelloAck::decode(&buf), Err(NipcError::Truncated));
}

#[test]
fn hello_ack_decode_bad_layout() {
    let h = HelloAck {
        layout_version: 0,
        ..Default::default()
    };
    let mut buf = [0u8; 48];
    h.encode(&mut buf);
    assert_eq!(HelloAck::decode(&buf), Err(NipcError::BadLayout));
}

#[test]
fn hello_ack_encode_too_small() {
    let h = HelloAck::default();
    let mut buf = [0u8; 10];
    assert_eq!(h.encode(&mut buf), 0);
}

// -----------------------------------------------------------------------
//  Cgroups snapshot request tests
// -----------------------------------------------------------------------

#[test]
fn cgroups_req_roundtrip() {
    let r = CgroupsRequest {
        layout_version: 1,
        flags: 0,
    };

    let mut buf = [0u8; 16];
    let n = r.encode(&mut buf);
    assert_eq!(n, 4);

    let out = CgroupsRequest::decode(&buf[..n]).unwrap();
    assert_eq!(out, r);
}

#[test]
fn cgroups_req_decode_truncated() {
    let buf = [0u8; 3];
    assert_eq!(CgroupsRequest::decode(&buf), Err(NipcError::Truncated));
}

#[test]
fn cgroups_req_decode_bad_layout() {
    let r = CgroupsRequest {
        layout_version: 5,
        flags: 0,
    };
    let mut buf = [0u8; 4];
    r.encode(&mut buf);
    assert_eq!(CgroupsRequest::decode(&buf), Err(NipcError::BadLayout));
}

#[test]
fn cgroups_req_encode_too_small() {
    let r = CgroupsRequest::default();
    let mut buf = [0u8; 2];
    assert_eq!(r.encode(&mut buf), 0);
}

// -----------------------------------------------------------------------
//  Cgroups snapshot response tests
// -----------------------------------------------------------------------

// Private constants needed by tests -- mirror cgroups_snapshot.rs values.
const CGROUPS_RESP_HDR_SIZE: usize = 24;
const CGROUPS_DIR_ENTRY_SIZE: usize = 8;

#[test]
fn cgroups_resp_empty() {
    let mut buf = [0u8; 4096];
    let b = CgroupsBuilder::new(&mut buf, 0, 1, 42);
    let total = b.finish();
    assert_eq!(total, 24);

    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item_count, 0);
    assert_eq!(view.systemd_enabled, 1);
    assert_eq!(view.generation, 42);
}

#[test]
fn cgroups_resp_single_item() {
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 100);

    let name = b"docker-abc123";
    let path = b"/sys/fs/cgroup/docker/abc123";
    b.add(12345, 0x01, 1, name, path).unwrap();

    let total = b.finish();
    assert!(total > 24);

    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item_count, 1);
    assert_eq!(view.systemd_enabled, 0);
    assert_eq!(view.generation, 100);

    let item = view.item(0).unwrap();
    assert_eq!(item.hash, 12345);
    assert_eq!(item.options, 0x01);
    assert_eq!(item.enabled, 1);
    assert_eq!(item.name.len as usize, name.len());
    assert_eq!(item.name.as_bytes(), name);
    assert_eq!(item.name.bytes[name.len()], 0); // NUL
    assert_eq!(item.path.len as usize, path.len());
    assert_eq!(item.path.as_bytes(), path);
    assert_eq!(item.path.bytes[path.len()], 0); // NUL
}

#[test]
fn cgroups_resp_multiple_items() {
    let mut buf = [0u8; 8192];
    let mut b = CgroupsBuilder::new(&mut buf, 5, 1, 999);

    // Item 0
    let n0 = b"init.scope";
    let p0 = b"/sys/fs/cgroup/init.scope";
    b.add(100, 0, 1, n0, p0).unwrap();

    // Item 1
    let n1 = b"system.slice/docker-abc.scope";
    let p1 = b"/sys/fs/cgroup/system.slice/docker-abc.scope";
    b.add(200, 0x02, 0, n1, p1).unwrap();

    // Item 2 - empty strings
    b.add(300, 0, 1, b"", b"").unwrap();

    let total = b.finish();

    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item_count, 3);
    assert_eq!(view.systemd_enabled, 1);
    assert_eq!(view.generation, 999);

    // Verify item 0
    let item = view.item(0).unwrap();
    assert_eq!(item.hash, 100);
    assert_eq!(item.name.len as usize, n0.len());
    assert_eq!(item.name.as_bytes(), n0);
    assert_eq!(item.path.len as usize, p0.len());
    assert_eq!(item.path.as_bytes(), p0);

    // Verify item 1
    let item = view.item(1).unwrap();
    assert_eq!(item.hash, 200);
    assert_eq!(item.options, 0x02);
    assert_eq!(item.enabled, 0);
    assert_eq!(item.name.len as usize, n1.len());
    assert_eq!(item.name.as_bytes(), n1);

    // Verify item 2 (empty strings)
    let item = view.item(2).unwrap();
    assert_eq!(item.hash, 300);
    assert_eq!(item.name.len, 0);
    assert_eq!(item.name.bytes[0], 0); // NUL
    assert_eq!(item.path.len, 0);
    assert_eq!(item.path.bytes[0], 0); // NUL

    // Out-of-bounds index
    assert_eq!(view.item(3), Err(NipcError::OutOfBounds));
}

#[test]
fn cgroups_resp_decode_truncated_header() {
    let buf = [0u8; 23];
    assert_eq!(
        CgroupsResponseView::decode(&buf).unwrap_err(),
        NipcError::Truncated
    );
}

#[test]
fn cgroups_resp_decode_bad_layout() {
    let mut buf = [0u8; 24];
    buf[0..2].copy_from_slice(&99u16.to_ne_bytes());
    assert_eq!(
        CgroupsResponseView::decode(&buf).unwrap_err(),
        NipcError::BadLayout
    );
}

#[test]
fn cgroups_resp_decode_truncated_dir() {
    // Header says item_count=2 but payload is only 24 bytes
    let mut buf = [0u8; 24];
    buf[0..2].copy_from_slice(&1u16.to_ne_bytes());
    buf[4..8].copy_from_slice(&2u32.to_ne_bytes());
    assert_eq!(
        CgroupsResponseView::decode(&buf).unwrap_err(),
        NipcError::Truncated
    );
}

#[test]
fn cgroups_resp_decode_oob_dir() {
    // Header + 1 dir entry pointing beyond payload
    let mut buf = [0u8; 64];
    buf[0..2].copy_from_slice(&1u16.to_ne_bytes());
    buf[4..8].copy_from_slice(&1u32.to_ne_bytes());
    // Dir entry at offset 24: offset=0, length=9999
    buf[24..28].copy_from_slice(&0u32.to_ne_bytes());
    buf[28..32].copy_from_slice(&9999u32.to_ne_bytes());
    assert_eq!(
        CgroupsResponseView::decode(&buf).unwrap_err(),
        NipcError::OutOfBounds
    );
}

#[test]
fn cgroups_resp_decode_item_too_small() {
    // Dir entry with length < 32
    let mut buf = [0u8; 64];
    buf[0..2].copy_from_slice(&1u16.to_ne_bytes());
    buf[4..8].copy_from_slice(&1u32.to_ne_bytes());
    buf[24..28].copy_from_slice(&0u32.to_ne_bytes());
    buf[28..32].copy_from_slice(&16u32.to_ne_bytes());
    assert_eq!(
        CgroupsResponseView::decode(&buf).unwrap_err(),
        NipcError::Truncated
    );
}

#[test]
fn cgroups_resp_item_missing_nul() {
    // Build valid snapshot then corrupt the NUL terminator
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 1);
    b.add(1, 0, 1, b"test", b"/test").unwrap();
    let total = b.finish();

    // Find item data and corrupt the name's NUL terminator
    let dir_end = CGROUPS_RESP_HDR_SIZE + 1 * CGROUPS_DIR_ENTRY_SIZE;
    let item_off = u32::from_ne_bytes(
        buf[CGROUPS_RESP_HDR_SIZE..CGROUPS_RESP_HDR_SIZE + 4]
            .try_into()
            .unwrap(),
    ) as usize;
    let item_start = dir_end + item_off;

    let noff =
        u32::from_ne_bytes(buf[item_start + 16..item_start + 20].try_into().unwrap()) as usize;
    let nlen =
        u32::from_ne_bytes(buf[item_start + 20..item_start + 24].try_into().unwrap()) as usize;

    buf[item_start + noff + nlen] = b'X'; // corrupt NUL

    // Re-decode after corruption -- header/dir still valid
    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item(0).unwrap_err(), NipcError::MissingNul);
}

#[test]
fn cgroups_resp_item_string_oob() {
    // Build valid snapshot then corrupt string length to be huge
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 1);
    b.add(1, 0, 1, b"test", b"/test").unwrap();
    let total = b.finish();

    // Corrupt name_length to huge value
    let dir_end = CGROUPS_RESP_HDR_SIZE + 1 * CGROUPS_DIR_ENTRY_SIZE;
    let item_off = u32::from_ne_bytes(
        buf[CGROUPS_RESP_HDR_SIZE..CGROUPS_RESP_HDR_SIZE + 4]
            .try_into()
            .unwrap(),
    ) as usize;
    let item_start = dir_end + item_off;

    buf[item_start + 20..item_start + 24].copy_from_slice(&99999u32.to_ne_bytes());

    // Re-decode after corruption
    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item(0).unwrap_err(), NipcError::OutOfBounds);
}

#[test]
fn cgroups_builder_overflow() {
    let mut buf = [0u8; 64]; // too small for any real item
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 0);
    let long_name = [b'A'; 200];
    assert_eq!(b.add(1, 0, 1, &long_name, b""), Err(NipcError::Overflow));
}

#[test]
fn cgroups_builder_max_items_exceeded() {
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 0);
    b.add(1, 0, 1, b"a", b"b").unwrap();
    assert_eq!(b.add(2, 0, 1, b"c", b"d"), Err(NipcError::Overflow));
}

#[test]
fn cgroups_builder_compaction() {
    let mut buf = [0u8; 4096];
    // Reserve 10 directory slots but only add 2 items
    let mut b = CgroupsBuilder::new(&mut buf, 10, 1, 77);

    b.add(10, 0, 1, b"slice-a", b"/cgroup/slice-a").unwrap();
    b.add(20, 0, 0, b"slice-b", b"/cgroup/slice-b").unwrap();

    let total = b.finish();

    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item_count, 2);
    assert_eq!(view.generation, 77);

    let item = view.item(0).unwrap();
    assert_eq!(item.hash, 10);
    assert_eq!(item.name.as_bytes(), b"slice-a");

    let item = view.item(1).unwrap();
    assert_eq!(item.hash, 20);
    assert_eq!(item.name.as_bytes(), b"slice-b");
}

// -----------------------------------------------------------------------
//  Alignment utility test
// -----------------------------------------------------------------------

#[test]
fn test_align8() {
    assert_eq!(align8(0), 0);
    assert_eq!(align8(1), 8);
    assert_eq!(align8(7), 8);
    assert_eq!(align8(8), 8);
    assert_eq!(align8(9), 16);
    assert_eq!(align8(16), 16);
    assert_eq!(align8(17), 24);
}

// -----------------------------------------------------------------------
//  Cross-language wire compatibility: C-Rust byte identity
//
//  These tests encode in Rust and verify the exact bytes match what the
//  C implementation produces for the same inputs. This ensures identical
//  wire output across languages.
// -----------------------------------------------------------------------

#[test]
fn c_rust_header_bytes_identical() {
    // Encode in Rust
    let h = Header {
        magic: MAGIC_MSG,
        version: VERSION,
        header_len: HEADER_LEN,
        kind: KIND_REQUEST,
        flags: FLAG_BATCH,
        code: METHOD_CGROUPS_SNAPSHOT,
        transport_status: STATUS_OK,
        payload_len: 12345,
        item_count: 42,
        message_id: 0xDEAD_BEEF_CAFE_BABE,
    };
    let mut rust_buf = [0u8; 32];
    h.encode(&mut rust_buf);

    // Known LE bytes for this header
    let expected: [u8; 32] = [
        0x43, 0x50, 0x49, 0x4e, // magic
        0x01, 0x00, // version
        0x20, 0x00, // header_len
        0x01, 0x00, // kind
        0x01, 0x00, // flags
        0x02, 0x00, // code
        0x00, 0x00, // transport_status
        0x39, 0x30, 0x00, 0x00, // payload_len = 12345
        0x2a, 0x00, 0x00, 0x00, // item_count = 42
        0xbe, 0xba, 0xfe, 0xca, 0xef, 0xbe, 0xad, 0xde, // message_id
    ];
    assert_eq!(rust_buf, expected);
}

#[test]
fn c_rust_chunk_bytes_identical() {
    let c = ChunkHeader {
        magic: MAGIC_CHUNK,
        version: VERSION,
        flags: 0,
        message_id: 1,
        total_message_len: 256,
        chunk_index: 1,
        chunk_count: 3,
        chunk_payload_len: 100,
    };
    let mut rust_buf = [0u8; 32];
    c.encode(&mut rust_buf);

    let expected: [u8; 32] = [
        0x4b, 0x48, 0x43, 0x4e, // magic
        0x01, 0x00, // version
        0x00, 0x00, // flags
        0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // message_id
        0x00, 0x01, 0x00, 0x00, // total_message_len = 256
        0x01, 0x00, 0x00, 0x00, // chunk_index
        0x03, 0x00, 0x00, 0x00, // chunk_count
        0x64, 0x00, 0x00, 0x00, // chunk_payload_len = 100
    ];
    assert_eq!(rust_buf, expected);
}

#[test]
fn c_rust_hello_bytes_identical() {
    let h = Hello {
        layout_version: 1,
        flags: 0,
        supported_profiles: PROFILE_BASELINE | PROFILE_SHM_FUTEX,
        preferred_profiles: PROFILE_SHM_FUTEX,
        max_request_payload_bytes: 4096,
        max_request_batch_items: 100,
        max_response_payload_bytes: 1048576,
        max_response_batch_items: 1,
        auth_token: 0xAABB_CCDD_EEFF_0011,
        packet_size: 65536,
    };

    let mut rust_buf = [0u8; 44];
    h.encode(&mut rust_buf);

    // Verify key byte positions
    assert_eq!(&rust_buf[0..2], &[0x01, 0x00]); // layout_version
    assert_eq!(&rust_buf[2..4], &[0x00, 0x00]); // flags
    assert_eq!(&rust_buf[4..8], &[0x05, 0x00, 0x00, 0x00]); // supported = 0x05
    assert_eq!(&rust_buf[8..12], &[0x04, 0x00, 0x00, 0x00]); // preferred = 0x04
    assert_eq!(&rust_buf[28..32], &[0x00, 0x00, 0x00, 0x00]); // padding = 0
    assert_eq!(
        &rust_buf[32..40],
        &[0x11, 0x00, 0xFF, 0xEE, 0xDD, 0xCC, 0xBB, 0xAA]
    ); // auth_token

    // Round-trip
    let out = Hello::decode(&rust_buf).unwrap();
    assert_eq!(out, h);
}

#[test]
fn c_rust_hello_ack_bytes_identical() {
    let h = HelloAck {
        layout_version: 1,
        flags: 0,
        server_supported_profiles: 0x07,
        intersection_profiles: 0x05,
        selected_profile: PROFILE_SHM_FUTEX,
        agreed_max_request_payload_bytes: 2048,
        agreed_max_request_batch_items: 50,
        agreed_max_response_payload_bytes: 65536,
        agreed_max_response_batch_items: 1,
        agreed_packet_size: 32768,
        session_id: 0x0000_0001_0000_0007,
    };
    let mut rust_buf = [0u8; 48];
    h.encode(&mut rust_buf);

    assert_eq!(&rust_buf[0..2], &[0x01, 0x00]);
    assert_eq!(&rust_buf[4..8], &[0x07, 0x00, 0x00, 0x00]); // server_supported
    assert_eq!(&rust_buf[12..16], &[0x04, 0x00, 0x00, 0x00]); // selected = SHM_FUTEX
    assert_eq!(&rust_buf[36..40], &[0x00, 0x00, 0x00, 0x00]); // padding
    assert_eq!(
        &rust_buf[40..48],
        &[0x07, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00]
    ); // session_id LE

    let out = HelloAck::decode(&rust_buf).unwrap();
    assert_eq!(out, h);
}

#[test]
fn c_rust_cgroups_req_bytes_identical() {
    let r = CgroupsRequest {
        layout_version: 1,
        flags: 0,
    };
    let mut rust_buf = [0u8; 4];
    r.encode(&mut rust_buf);

    assert_eq!(rust_buf, [0x01, 0x00, 0x00, 0x00]);

    let out = CgroupsRequest::decode(&rust_buf).unwrap();
    assert_eq!(out, r);
}

#[test]
fn c_rust_cgroups_snapshot_bytes_identical() {
    // Build a snapshot with the exact same inputs as the C test
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 100);
    b.add(
        12345,
        0x01,
        1,
        b"docker-abc123",
        b"/sys/fs/cgroup/docker/abc123",
    )
    .unwrap();
    let total = b.finish();

    // Verify the snapshot header bytes
    assert_eq!(&buf[0..2], &[0x01, 0x00]); // layout_version
    assert_eq!(&buf[2..4], &[0x00, 0x00]); // flags
    assert_eq!(&buf[4..8], &[0x01, 0x00, 0x00, 0x00]); // item_count
    assert_eq!(&buf[8..12], &[0x00, 0x00, 0x00, 0x00]); // systemd_enabled
    assert_eq!(&buf[12..16], &[0x00, 0x00, 0x00, 0x00]); // reserved
    assert_eq!(
        &buf[16..24],
        &[0x64, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]
    ); // generation

    // Verify it decodes correctly
    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item_count, 1);
    assert_eq!(view.generation, 100);

    let item = view.item(0).unwrap();
    assert_eq!(item.hash, 12345);
    assert_eq!(item.name.as_bytes(), b"docker-abc123");
    assert_eq!(item.path.as_bytes(), b"/sys/fs/cgroup/docker/abc123");
}

#[test]
fn cgroups_resp_dir_bad_alignment() {
    // Dir entry with unaligned offset
    let mut buf = [0u8; 128];
    buf[0..2].copy_from_slice(&1u16.to_ne_bytes());
    buf[4..8].copy_from_slice(&1u32.to_ne_bytes());
    // offset=3 (not 8-byte aligned), length=32
    buf[24..28].copy_from_slice(&3u32.to_ne_bytes());
    buf[28..32].copy_from_slice(&32u32.to_ne_bytes());
    assert_eq!(
        CgroupsResponseView::decode(&buf).unwrap_err(),
        NipcError::BadAlignment
    );
}

#[test]
fn cgroups_resp_item_bad_layout_version() {
    // Build valid snapshot, then corrupt the item's layout_version
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 1);
    b.add(1, 0, 1, b"test", b"/test").unwrap();
    let total = b.finish();

    // Corrupt item layout_version
    let dir_end = CGROUPS_RESP_HDR_SIZE + 1 * CGROUPS_DIR_ENTRY_SIZE;
    let item_off = u32::from_ne_bytes(
        buf[CGROUPS_RESP_HDR_SIZE..CGROUPS_RESP_HDR_SIZE + 4]
            .try_into()
            .unwrap(),
    ) as usize;
    let item_start = dir_end + item_off;
    buf[item_start..item_start + 2].copy_from_slice(&99u16.to_ne_bytes());

    // Re-decode after corruption
    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item(0).unwrap_err(), NipcError::BadLayout);
}

#[test]
fn cgroups_resp_item_name_off_below_header() {
    // Build valid snapshot, then set name_offset < 32
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 1);
    b.add(1, 0, 1, b"test", b"/test").unwrap();
    let total = b.finish();

    let dir_end = CGROUPS_RESP_HDR_SIZE + 1 * CGROUPS_DIR_ENTRY_SIZE;
    let item_off = u32::from_ne_bytes(
        buf[CGROUPS_RESP_HDR_SIZE..CGROUPS_RESP_HDR_SIZE + 4]
            .try_into()
            .unwrap(),
    ) as usize;
    let item_start = dir_end + item_off;
    // Set name_offset to 0 (below header)
    buf[item_start + 16..item_start + 20].copy_from_slice(&0u32.to_ne_bytes());

    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item(0).unwrap_err(), NipcError::OutOfBounds);
}

#[test]
fn cgroups_resp_item_path_off_below_header() {
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 1);
    b.add(1, 0, 1, b"test", b"/test").unwrap();
    let total = b.finish();

    let dir_end = CGROUPS_RESP_HDR_SIZE + 1 * CGROUPS_DIR_ENTRY_SIZE;
    let item_off = u32::from_ne_bytes(
        buf[CGROUPS_RESP_HDR_SIZE..CGROUPS_RESP_HDR_SIZE + 4]
            .try_into()
            .unwrap(),
    ) as usize;
    let item_start = dir_end + item_off;
    // Set path_offset to 16 (below header)
    buf[item_start + 24..item_start + 28].copy_from_slice(&16u32.to_ne_bytes());

    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item(0).unwrap_err(), NipcError::OutOfBounds);
}

#[test]
fn cgroups_resp_item_path_missing_nul() {
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 1);
    b.add(1, 0, 1, b"test", b"/test").unwrap();
    let total = b.finish();

    // Corrupt path NUL
    let dir_end = CGROUPS_RESP_HDR_SIZE + 1 * CGROUPS_DIR_ENTRY_SIZE;
    let item_off = u32::from_ne_bytes(
        buf[CGROUPS_RESP_HDR_SIZE..CGROUPS_RESP_HDR_SIZE + 4]
            .try_into()
            .unwrap(),
    ) as usize;
    let item_start = dir_end + item_off;
    let poff =
        u32::from_ne_bytes(buf[item_start + 24..item_start + 28].try_into().unwrap()) as usize;
    let plen =
        u32::from_ne_bytes(buf[item_start + 28..item_start + 32].try_into().unwrap()) as usize;
    buf[item_start + poff + plen] = b'X';

    // Re-decode after corruption
    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item(0).unwrap_err(), NipcError::MissingNul);
}

#[test]
fn cgroups_resp_item_overlap_rejected() {
    // Build a valid item, then manually set path_offset to overlap with name
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 1);
    b.add(1, 0, 1, b"hello", b"/path").unwrap();
    let total = b.finish();

    let dir_end = CGROUPS_RESP_HDR_SIZE + 1 * CGROUPS_DIR_ENTRY_SIZE;
    let item_off = u32::from_ne_bytes(
        buf[CGROUPS_RESP_HDR_SIZE..CGROUPS_RESP_HDR_SIZE + 4]
            .try_into()
            .unwrap(),
    ) as usize;
    let item_start = dir_end + item_off;

    // name_off=32, name_len=5, so name region is [32..38)
    // Set path_off=34 (inside name region), path_len=1
    buf[item_start + 24..item_start + 28].copy_from_slice(&34u32.to_ne_bytes());
    buf[item_start + 28..item_start + 32].copy_from_slice(&1u32.to_ne_bytes());
    // Ensure NUL at item[34+1]=item[35]
    buf[item_start + 35] = 0;

    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item(0).unwrap_err(), NipcError::BadLayout);
}

// -------------------------------------------------------------------
//  Proptest: fuzz / property-based tests for all decode paths
// -------------------------------------------------------------------

mod proptests {
    use super::*;
    use proptest::prelude::*;

    // Arbitrary bytes -- no decode path may panic on any input.

    proptest! {
        #[test]
        fn decode_header_never_panics(data: Vec<u8>) {
            let _ = Header::decode(&data);
        }

        #[test]
        fn decode_chunk_header_never_panics(data: Vec<u8>) {
            let _ = ChunkHeader::decode(&data);
        }

        #[test]
        fn decode_hello_never_panics(data: Vec<u8>) {
            let _ = Hello::decode(&data);
        }

        #[test]
        fn decode_hello_ack_never_panics(data: Vec<u8>) {
            let _ = HelloAck::decode(&data);
        }

        #[test]
        fn decode_cgroups_request_never_panics(data: Vec<u8>) {
            let _ = CgroupsRequest::decode(&data);
        }

        #[test]
        fn decode_cgroups_response_never_panics(data: Vec<u8>) {
            let result = CgroupsResponseView::decode(&data);
            if let Ok(view) = result {
                // Exercise item access on valid decodes.
                let limit = view.item_count.min(64);
                for i in 0..limit {
                    let _ = view.item(i);
                }
                // Out-of-bounds must not panic.
                let _ = view.item(view.item_count);
            }
        }

        #[test]
        fn decode_cgroups_lookup_request_never_panics(data: Vec<u8>) {
            let result = CgroupsLookupRequestView::decode(&data);
            if let Ok(view) = result {
                let limit = view.item_count.min(64);
                for i in 0..limit {
                    let _ = view.item(i);
                }
                let _ = view.item(view.item_count);
            }
        }

        #[test]
        fn decode_cgroups_lookup_response_never_panics(data: Vec<u8>) {
            let result = CgroupsLookupResponseView::decode(&data);
            if let Ok(view) = result {
                let limit = view.item_count.min(64);
                for i in 0..limit {
                    if let Ok(item) = view.item(i) {
                        let label_limit = item.label_count.min(64);
                        for j in 0..label_limit {
                            let _ = item.label(j.into());
                        }
                        let _ = item.label(item.label_count.into());
                    }
                }
                let _ = view.item(view.item_count);
            }
        }

        #[test]
        fn decode_apps_lookup_request_never_panics(data: Vec<u8>) {
            let result = AppsLookupRequestView::decode(&data);
            if let Ok(view) = result {
                let limit = view.item_count.min(64);
                for i in 0..limit {
                    let _ = view.item(i);
                }
                let _ = view.item(view.item_count);
            }
        }

        #[test]
        fn decode_apps_lookup_response_never_panics(data: Vec<u8>) {
            let result = AppsLookupResponseView::decode(&data);
            if let Ok(view) = result {
                let limit = view.item_count.min(64);
                for i in 0..limit {
                    if let Ok(item) = view.item(i) {
                        let label_limit = item.label_count.min(64);
                        for j in 0..label_limit {
                            let _ = item.label(j.into());
                        }
                        let _ = item.label(item.label_count.into());
                    }
                }
                let _ = view.item(view.item_count);
            }
        }

        #[test]
        fn batch_dir_decode_never_panics(
            data: Vec<u8>,
            item_count in 0u32..128,
            packed_area_len in 0u32..65536,
        ) {
            let _ = batch_dir_decode(&data, item_count, packed_area_len);
        }

        #[test]
        fn batch_item_get_never_panics(
            data: Vec<u8>,
            item_count in 0u32..128,
            index in 0u32..128,
        ) {
            let _ = batch_item_get(&data, item_count, index);
        }
    }

    // Roundtrip tests: encode random valid values, decode, verify match.

    proptest! {
        #[test]
        fn encode_decode_header_roundtrip(
            kind in 1u16..=3,
            flags in any::<u16>(),
            code in any::<u16>(),
            transport_status in any::<u16>(),
            payload_len in any::<u32>(),
            item_count in any::<u32>(),
            message_id in any::<u64>(),
        ) {
            let h = Header {
                magic: MAGIC_MSG,
                version: VERSION,
                header_len: HEADER_LEN,
                kind,
                flags,
                code,
                transport_status,
                payload_len,
                item_count,
                message_id,
            };
            let mut buf = [0u8; 64];
            let n = h.encode(&mut buf);
            prop_assert_eq!(n, HEADER_SIZE);
            let decoded = Header::decode(&buf[..n]).unwrap();
            prop_assert_eq!(decoded, h);
        }

        #[test]
        fn encode_decode_hello_roundtrip(
            supported in any::<u32>(),
            preferred in any::<u32>(),
            max_req_payload in any::<u32>(),
            max_req_batch in any::<u32>(),
            max_resp_payload in any::<u32>(),
            max_resp_batch in any::<u32>(),
            auth_token in any::<u64>(),
            packet_size in any::<u32>(),
        ) {
            let h = Hello {
                layout_version: 1,
                flags: 0,
                supported_profiles: supported,
                preferred_profiles: preferred,
                max_request_payload_bytes: max_req_payload,
                max_request_batch_items: max_req_batch,
                max_response_payload_bytes: max_resp_payload,
                max_response_batch_items: max_resp_batch,
                auth_token,
                packet_size,
            };
            let mut buf = [0u8; 64];
            let n = h.encode(&mut buf);
            prop_assert_eq!(n, HELLO_SIZE);
            let decoded = Hello::decode(&buf[..n]).unwrap();
            prop_assert_eq!(decoded, h);
        }
    }
}

// -----------------------------------------------------------------------
//  NipcError Display coverage
// -----------------------------------------------------------------------

#[test]
fn nipc_error_display_all_variants() {
    // Exercise the Display impl for every NipcError variant (lines 101-113)
    let cases: Vec<(NipcError, &str)> = vec![
        (NipcError::Truncated, "buffer too short"),
        (NipcError::BadMagic, "magic value mismatch"),
        (NipcError::BadVersion, "unsupported version"),
        (NipcError::BadHeaderLen, "header_len != 32"),
        (NipcError::BadKind, "unknown message kind"),
        (NipcError::BadLayout, "unknown layout_version"),
        (NipcError::OutOfBounds, "offset+length exceeds data"),
        (NipcError::MissingNul, "string not NUL-terminated"),
        (NipcError::BadAlignment, "item not 8-byte aligned"),
        (NipcError::BadItemCount, "item count inconsistent"),
        (NipcError::Overflow, "builder out of space"),
    ];
    for (err, expected) in cases {
        let msg = format!("{}", err);
        assert_eq!(msg, expected, "Display for {:?}", err);
    }
    // Also verify std::error::Error is implemented
    let err: &dyn std::error::Error = &NipcError::Truncated;
    let _ = format!("{err}");
}

// -----------------------------------------------------------------------
//  ChunkHeader decode: flags != 0 and chunk_payload_len == 0
// -----------------------------------------------------------------------

#[test]
fn chunk_decode_bad_flags() {
    // Line 257: flags != 0 -> BadLayout
    let c = ChunkHeader {
        magic: MAGIC_CHUNK,
        version: VERSION,
        flags: 0x01, // non-zero flags
        message_id: 1,
        total_message_len: 100,
        chunk_index: 0,
        chunk_count: 1,
        chunk_payload_len: 50,
    };
    let mut buf = [0u8; 32];
    c.encode(&mut buf);
    assert_eq!(ChunkHeader::decode(&buf), Err(NipcError::BadLayout));
}

#[test]
fn chunk_decode_zero_payload_len() {
    // Line 260: chunk_payload_len == 0 -> BadLayout
    let c = ChunkHeader {
        magic: MAGIC_CHUNK,
        version: VERSION,
        flags: 0,
        message_id: 1,
        total_message_len: 100,
        chunk_index: 0,
        chunk_count: 1,
        chunk_payload_len: 0,
    };
    let mut buf = [0u8; 32];
    c.encode(&mut buf);
    assert_eq!(ChunkHeader::decode(&buf), Err(NipcError::BadLayout));
}

// -----------------------------------------------------------------------
//  batch_dir_encode: buffer too small (line 282)
// -----------------------------------------------------------------------

#[test]
fn batch_dir_encode_too_small() {
    let entries = [
        BatchEntry {
            offset: 0,
            length: 8,
        },
        BatchEntry {
            offset: 8,
            length: 8,
        },
    ];
    let mut buf = [0u8; 12]; // needs 16, only 12
    assert_eq!(batch_dir_encode(&entries, &mut buf), 0);
}

// -----------------------------------------------------------------------
//  batch_dir_validate error paths (lines 329, 336, 339)
// -----------------------------------------------------------------------

#[test]
fn batch_dir_validate_truncated() {
    let buf = [0u8; 4]; // too short for 1 entry (needs 8)
    assert_eq!(batch_dir_validate(&buf, 1, 100), Err(NipcError::Truncated));
}

#[test]
fn batch_dir_validate_bad_alignment() {
    let mut buf = [0u8; 8];
    buf[0..4].copy_from_slice(&3u32.to_ne_bytes()); // unaligned offset
    buf[4..8].copy_from_slice(&8u32.to_ne_bytes());
    assert_eq!(
        batch_dir_validate(&buf, 1, 100),
        Err(NipcError::BadAlignment)
    );
}

#[test]
fn batch_dir_validate_out_of_bounds() {
    let mut buf = [0u8; 8];
    buf[0..4].copy_from_slice(&0u32.to_ne_bytes());
    buf[4..8].copy_from_slice(&200u32.to_ne_bytes()); // exceeds packed_area_len
    assert_eq!(
        batch_dir_validate(&buf, 1, 100),
        Err(NipcError::OutOfBounds)
    );
}

#[test]
fn batch_dir_validate_ok() {
    let mut buf = [0u8; 16];
    buf[0..4].copy_from_slice(&0u32.to_ne_bytes());
    buf[4..8].copy_from_slice(&8u32.to_ne_bytes());
    buf[8..12].copy_from_slice(&8u32.to_ne_bytes());
    buf[12..16].copy_from_slice(&8u32.to_ne_bytes());
    assert!(batch_dir_validate(&buf, 2, 100).is_ok());
}

// -----------------------------------------------------------------------
//  batch_item_get: alignment check (line 375)
// -----------------------------------------------------------------------

#[test]
fn batch_item_get_bad_alignment() {
    // Manually craft a batch payload with unaligned offset
    let mut buf = [0u8; 64];
    // Directory: 1 entry at offset 0 of buf
    buf[0..4].copy_from_slice(&3u32.to_ne_bytes()); // unaligned offset
    buf[4..8].copy_from_slice(&4u32.to_ne_bytes());
    assert_eq!(batch_item_get(&buf, 1, 0), Err(NipcError::BadAlignment));
}

#[test]
fn batch_item_get_truncated_dir() {
    // Payload too small to hold the directory
    let buf = [0u8; 4]; // needs at least 8 for 1 item directory
    assert_eq!(batch_item_get(&buf, 1, 0), Err(NipcError::Truncated));
}

// -----------------------------------------------------------------------
//  BatchBuilder::finish compaction (lines 451-456)
// -----------------------------------------------------------------------

#[test]
fn batch_builder_compaction() {
    // Reserve space for 8 items but add only 2 -- triggers compaction
    let mut buf = [0u8; 1024];
    let mut b = BatchBuilder::new(&mut buf, 8);

    let item1 = [1u8, 2, 3, 4, 5, 6, 7, 8];
    let item2 = [10u8, 20, 30, 40];

    b.add(&item1).unwrap();
    b.add(&item2).unwrap();

    // dir_end for 8 items = align8(8*8) = 64
    // final_dir_aligned for 2 items = align8(2*8) = 16
    // This triggers the copy_within compaction branch (line 453-456)
    let (total, count) = b.finish();
    assert_eq!(count, 2);
    assert!(total > 0);

    // Verify the items can still be extracted correctly
    let (data, len) = batch_item_get(&buf[..total], 2, 0).unwrap();
    assert_eq!(len as usize, item1.len());
    assert_eq!(data, &item1);

    let (data, len) = batch_item_get(&buf[..total], 2, 1).unwrap();
    assert_eq!(len as usize, item2.len());
    assert_eq!(data, &item2);
}

// -----------------------------------------------------------------------
//  HelloAck decode: flags != 0 (line 606)
// -----------------------------------------------------------------------

#[test]
fn hello_ack_decode_bad_flags() {
    let h = HelloAck {
        layout_version: 1,
        flags: 1, // non-zero flags
        ..Default::default()
    };
    let mut buf = [0u8; 48];
    h.encode(&mut buf);
    assert_eq!(HelloAck::decode(&buf), Err(NipcError::BadLayout));
}

// -----------------------------------------------------------------------
//  Hello decode: non-zero padding (line 527)
// -----------------------------------------------------------------------

#[test]
fn hello_decode_bad_padding() {
    let h = Hello {
        layout_version: 1,
        flags: 0,
        ..Default::default()
    };
    let mut buf = [0u8; 44];
    h.encode(&mut buf);
    // Corrupt the padding bytes at 28..32
    buf[28..32].copy_from_slice(&1u32.to_ne_bytes());
    assert_eq!(Hello::decode(&buf), Err(NipcError::BadLayout));
}

// -----------------------------------------------------------------------
//  Cgroups request: non-zero flags (line 45)
// -----------------------------------------------------------------------

#[test]
fn cgroups_req_decode_bad_flags() {
    let r = CgroupsRequest {
        layout_version: 1,
        flags: 1, // non-zero flags -> BadLayout
    };
    let mut buf = [0u8; 4];
    r.encode(&mut buf);
    assert_eq!(CgroupsRequest::decode(&buf), Err(NipcError::BadLayout));
}

// -----------------------------------------------------------------------
//  CgroupsResponseView: non-zero flags and reserved (lines 125, 130)
// -----------------------------------------------------------------------

#[test]
fn cgroups_resp_decode_bad_flags() {
    // Line 125: flags != 0 -> BadLayout
    let mut buf = [0u8; 24];
    buf[0..2].copy_from_slice(&1u16.to_ne_bytes()); // layout_version = 1
    buf[2..4].copy_from_slice(&1u16.to_ne_bytes()); // flags = 1 (non-zero)
    assert_eq!(
        CgroupsResponseView::decode(&buf).unwrap_err(),
        NipcError::BadLayout
    );
}

#[test]
fn cgroups_resp_decode_bad_reserved() {
    // Line 130: reserved != 0 -> BadLayout
    let mut buf = [0u8; 24];
    buf[0..2].copy_from_slice(&1u16.to_ne_bytes()); // layout_version = 1
    buf[2..4].copy_from_slice(&0u16.to_ne_bytes()); // flags = 0
    buf[12..16].copy_from_slice(&1u32.to_ne_bytes()); // reserved = 1
    assert_eq!(
        CgroupsResponseView::decode(&buf).unwrap_err(),
        NipcError::BadLayout
    );
}

// -----------------------------------------------------------------------
//  CgroupsResponseView: bad alignment in directory (line 149)
// -----------------------------------------------------------------------

#[test]
fn cgroups_resp_decode_bad_dir_alignment() {
    // dir entry with offset not aligned to 8
    let mut buf = [0u8; 128];
    buf[0..2].copy_from_slice(&1u16.to_ne_bytes()); // layout_version
    buf[4..8].copy_from_slice(&1u32.to_ne_bytes()); // item_count = 1
                                                    // Dir entry at offset 24: offset=3 (unaligned), length=32
    buf[24..28].copy_from_slice(&3u32.to_ne_bytes());
    buf[28..32].copy_from_slice(&32u32.to_ne_bytes());
    assert_eq!(
        CgroupsResponseView::decode(&buf).unwrap_err(),
        NipcError::BadAlignment
    );
}

// -----------------------------------------------------------------------
//  CgroupsItemView: bad layout_version, bad flags (lines 200-206)
// -----------------------------------------------------------------------

#[test]
fn cgroups_item_bad_layout_version() {
    // Build valid snapshot then corrupt item layout_version
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 1);
    b.add(1, 0, 1, b"test", b"/test").unwrap();
    let total = b.finish();

    // Find item start
    let dir_end = CGROUPS_RESP_HDR_SIZE + 1 * CGROUPS_DIR_ENTRY_SIZE;
    let item_off = u32::from_ne_bytes(
        buf[CGROUPS_RESP_HDR_SIZE..CGROUPS_RESP_HDR_SIZE + 4]
            .try_into()
            .unwrap(),
    ) as usize;
    let item_start = dir_end + item_off;

    // Corrupt layout_version to 99
    buf[item_start..item_start + 2].copy_from_slice(&99u16.to_ne_bytes());

    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item(0).unwrap_err(), NipcError::BadLayout);
}

#[test]
fn cgroups_item_bad_flags() {
    // Build valid snapshot then corrupt item flags
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 1);
    b.add(1, 0, 1, b"test", b"/test").unwrap();
    let total = b.finish();

    let dir_end = CGROUPS_RESP_HDR_SIZE + 1 * CGROUPS_DIR_ENTRY_SIZE;
    let item_off = u32::from_ne_bytes(
        buf[CGROUPS_RESP_HDR_SIZE..CGROUPS_RESP_HDR_SIZE + 4]
            .try_into()
            .unwrap(),
    ) as usize;
    let item_start = dir_end + item_off;

    // Corrupt flags to non-zero
    buf[item_start + 2..item_start + 4].copy_from_slice(&1u16.to_ne_bytes());

    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item(0).unwrap_err(), NipcError::BadLayout);
}

// -----------------------------------------------------------------------
//  CgroupsItemView: name_off < ITEM_HDR_SIZE (line 211)
// -----------------------------------------------------------------------

#[test]
fn cgroups_item_name_off_too_small() {
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 1);
    b.add(1, 0, 1, b"test", b"/test").unwrap();
    let total = b.finish();

    let dir_end = CGROUPS_RESP_HDR_SIZE + 1 * CGROUPS_DIR_ENTRY_SIZE;
    let item_off = u32::from_ne_bytes(
        buf[CGROUPS_RESP_HDR_SIZE..CGROUPS_RESP_HDR_SIZE + 4]
            .try_into()
            .unwrap(),
    ) as usize;
    let item_start = dir_end + item_off;

    // Set name_offset to 0 (< 32 = CGROUPS_ITEM_HDR_SIZE)
    buf[item_start + 16..item_start + 20].copy_from_slice(&0u32.to_ne_bytes());

    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item(0).unwrap_err(), NipcError::OutOfBounds);
}

// -----------------------------------------------------------------------
//  CgroupsItemView: path NUL missing (line 228)
// -----------------------------------------------------------------------

#[test]
fn cgroups_item_path_missing_nul() {
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 1);
    b.add(1, 0, 1, b"test", b"/test").unwrap();
    let total = b.finish();

    let dir_end = CGROUPS_RESP_HDR_SIZE + 1 * CGROUPS_DIR_ENTRY_SIZE;
    let item_off = u32::from_ne_bytes(
        buf[CGROUPS_RESP_HDR_SIZE..CGROUPS_RESP_HDR_SIZE + 4]
            .try_into()
            .unwrap(),
    ) as usize;
    let item_start = dir_end + item_off;

    // Find the path NUL terminator and corrupt it
    let path_off =
        u32::from_ne_bytes(buf[item_start + 24..item_start + 28].try_into().unwrap()) as usize;
    let path_len =
        u32::from_ne_bytes(buf[item_start + 28..item_start + 32].try_into().unwrap()) as usize;
    buf[item_start + path_off + path_len] = b'X'; // corrupt path NUL

    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item(0).unwrap_err(), NipcError::MissingNul);
}

// -----------------------------------------------------------------------
//  CgroupsItemView: path_off < ITEM_HDR_SIZE (line 222)
// -----------------------------------------------------------------------

#[test]
fn cgroups_item_path_off_too_small() {
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 1);
    b.add(1, 0, 1, b"test", b"/test").unwrap();
    let total = b.finish();

    let dir_end = CGROUPS_RESP_HDR_SIZE + 1 * CGROUPS_DIR_ENTRY_SIZE;
    let item_off = u32::from_ne_bytes(
        buf[CGROUPS_RESP_HDR_SIZE..CGROUPS_RESP_HDR_SIZE + 4]
            .try_into()
            .unwrap(),
    ) as usize;
    let item_start = dir_end + item_off;

    // Set path_offset to 0 (< 32 = CGROUPS_ITEM_HDR_SIZE)
    buf[item_start + 24..item_start + 28].copy_from_slice(&0u32.to_ne_bytes());

    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item(0).unwrap_err(), NipcError::OutOfBounds);
}

// -----------------------------------------------------------------------
//  CgroupsItemView: path string OOB (line 225)
// -----------------------------------------------------------------------

#[test]
fn cgroups_item_path_string_oob() {
    let mut buf = [0u8; 4096];
    let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 1);
    b.add(1, 0, 1, b"test", b"/test").unwrap();
    let total = b.finish();

    let dir_end = CGROUPS_RESP_HDR_SIZE + 1 * CGROUPS_DIR_ENTRY_SIZE;
    let item_off = u32::from_ne_bytes(
        buf[CGROUPS_RESP_HDR_SIZE..CGROUPS_RESP_HDR_SIZE + 4]
            .try_into()
            .unwrap(),
    ) as usize;
    let item_start = dir_end + item_off;

    // Corrupt path_length to huge value
    buf[item_start + 28..item_start + 32].copy_from_slice(&99999u32.to_ne_bytes());

    let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
    assert_eq!(view.item(0).unwrap_err(), NipcError::OutOfBounds);
}

// -----------------------------------------------------------------------
//  Cgroups dispatch (lines 438-447)
// -----------------------------------------------------------------------

#[test]
fn dispatch_cgroups_snapshot_bad_request() {
    // Bad request (too short) -> dispatch returns None (line 438)
    let mut resp = [0u8; 4096];
    let result =
        crate::protocol::dispatch_cgroups_snapshot(&[], &mut resp, 1, |_req, _builder| true);
    assert!(result.is_none());
}

#[test]
fn dispatch_cgroups_snapshot_handler_returns_false() {
    // Handler returns false -> dispatch returns None (lines 440-441)
    let req = CgroupsRequest {
        layout_version: 1,
        flags: 0,
    };
    let mut req_buf = [0u8; 4];
    req.encode(&mut req_buf);
    let mut resp = [0u8; 4096];
    let result =
        crate::protocol::dispatch_cgroups_snapshot(&req_buf, &mut resp, 1, |_req, _builder| false);
    assert!(result.is_none());
}

#[test]
fn dispatch_cgroups_snapshot_success() {
    let req = CgroupsRequest {
        layout_version: 1,
        flags: 0,
    };
    let mut req_buf = [0u8; 4];
    req.encode(&mut req_buf);
    let mut resp = [0u8; 4096];
    let result =
        crate::protocol::dispatch_cgroups_snapshot(&req_buf, &mut resp, 2, |_req, builder| {
            builder.add(1, 0, 1, b"cg1", b"/test").unwrap();
            true
        });
    assert!(result.is_some());
    let n = result.unwrap();
    let view = CgroupsResponseView::decode(&resp[..n]).unwrap();
    assert_eq!(view.item_count, 1);
}

// -----------------------------------------------------------------------
//  Dispatch increment / string_reverse buf overflow (lines 27, 58)
// -----------------------------------------------------------------------

#[test]
fn dispatch_increment_resp_too_small() {
    // Response buffer too small -> encode returns 0 -> dispatch returns None
    let req_val = 42u64;
    let mut req_buf = [0u8; 8];
    crate::protocol::increment_encode(req_val, &mut req_buf);
    let mut resp = [0u8; 4]; // too small for 8-byte response
    let result = crate::protocol::dispatch_increment(&req_buf, &mut resp, |v| Some(v + 1));
    assert!(result.is_none());
}

#[test]
fn dispatch_increment_handler_none() {
    let mut req_buf = [0u8; 8];
    crate::protocol::increment_encode(42, &mut req_buf);
    let mut resp = [0u8; 8];
    let result = crate::protocol::dispatch_increment(&req_buf, &mut resp, |_| None);
    assert!(result.is_none());
}

#[test]
fn dispatch_string_reverse_resp_too_small() {
    let s = b"hello";
    let mut req_buf = [0u8; 64];
    crate::protocol::string_reverse_encode(s, &mut req_buf);
    let mut resp = [0u8; 4]; // too small
    let result = crate::protocol::dispatch_string_reverse(&req_buf, &mut resp, |data| {
        Some(data.iter().rev().copied().collect())
    });
    assert!(result.is_none());
}

#[test]
fn dispatch_string_reverse_handler_none() {
    let s = b"hello";
    let mut req_buf = [0u8; 64];
    crate::protocol::string_reverse_encode(s, &mut req_buf);
    let mut resp = [0u8; 64];
    let result = crate::protocol::dispatch_string_reverse(&req_buf, &mut resp, |_| None);
    assert!(result.is_none());
}
