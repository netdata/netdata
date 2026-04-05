use super::*;
use crate::protocol;
use std::ffi::OsString;
use std::os::unix::ffi::OsStringExt;
use std::path::PathBuf;
use std::thread;

const TEST_RUN_DIR: &str = "/tmp/nipc_shm_rust_test";

fn ensure_run_dir() {
    let _ = std::fs::create_dir_all(TEST_RUN_DIR);
}

fn cleanup_shm(service: &str, session_id: u64) {
    let path = format!("{TEST_RUN_DIR}/{service}-{session_id:016x}.ipcshm");
    let _ = std::fs::remove_file(&path);
}

/// Build a complete message (outer header + payload) for SHM.
fn build_message(kind: u16, code: u16, message_id: u64, payload: &[u8]) -> Vec<u8> {
    let hdr = protocol::Header {
        magic: protocol::MAGIC_MSG,
        version: protocol::VERSION,
        header_len: protocol::HEADER_LEN,
        kind,
        code,
        flags: 0,
        transport_status: protocol::STATUS_OK,
        payload_len: payload.len() as u32,
        item_count: 1,
        message_id,
    };
    let mut buf = vec![0u8; protocol::HEADER_SIZE + payload.len()];
    hdr.encode(&mut buf[..protocol::HEADER_SIZE]);
    buf[protocol::HEADER_SIZE..].copy_from_slice(payload);
    buf
}

#[test]
fn test_direct_roundtrip() {
    ensure_run_dir();
    let svc = "rs_shm_rt";
    let sid: u64 = 1;
    cleanup_shm(svc, sid);

    let svc_clone = svc.to_string();
    let server_thread = thread::spawn(move || {
        let mut ctx = ShmContext::server_create(TEST_RUN_DIR, &svc_clone, sid, 4096, 4096)
            .expect("server create");

        // Receive request
        let mut buf = vec![0u8; 65536];
        let mlen = ctx.receive(&mut buf, 5000).expect("server receive");
        let msg = &buf[..mlen];
        assert!(msg.len() >= protocol::HEADER_SIZE);

        // Parse and echo as response
        let hdr = protocol::Header::decode(msg).expect("decode");
        let payload = msg[protocol::HEADER_SIZE..].to_vec();
        let resp = build_message(protocol::KIND_RESPONSE, hdr.code, hdr.message_id, &payload);
        ctx.send(&resp).expect("server send");
        ctx.destroy();
    });

    // Wait for server to create region
    thread::sleep(std::time::Duration::from_millis(50));

    let mut client = ShmContext::client_attach(TEST_RUN_DIR, svc, sid).expect("client attach");

    let payload = vec![0xCA, 0xFE, 0xBA, 0xBE];
    let msg = build_message(
        protocol::KIND_REQUEST,
        protocol::METHOD_INCREMENT,
        42,
        &payload,
    );
    client.send(&msg).expect("client send");

    let mut resp_buf = vec![0u8; 65536];
    let rlen = client.receive(&mut resp_buf, 5000).expect("client receive");
    let resp = &resp_buf[..rlen];
    assert_eq!(resp.len(), protocol::HEADER_SIZE + payload.len());

    let rhdr = protocol::Header::decode(resp).expect("decode response");
    assert_eq!(rhdr.kind, protocol::KIND_RESPONSE);
    assert_eq!(rhdr.message_id, 42);
    assert_eq!(&resp[protocol::HEADER_SIZE..], &payload[..]);

    client.close();
    server_thread.join().unwrap();
    cleanup_shm(svc, sid);
}

#[test]
fn test_multiple_roundtrips() {
    ensure_run_dir();
    let svc = "rs_shm_multi";
    let sid: u64 = 2;
    cleanup_shm(svc, sid);

    let svc_clone = svc.to_string();
    let server_thread = thread::spawn(move || {
        let mut ctx = ShmContext::server_create(TEST_RUN_DIR, &svc_clone, sid, 4096, 4096)
            .expect("server create");

        let mut buf = vec![0u8; 65536];
        for _ in 0..10 {
            let mlen = ctx.receive(&mut buf, 5000).expect("server receive");
            let msg = &buf[..mlen];
            let hdr = protocol::Header::decode(msg).expect("decode");
            let payload = msg[protocol::HEADER_SIZE..].to_vec();
            let resp = build_message(protocol::KIND_RESPONSE, hdr.code, hdr.message_id, &payload);
            ctx.send(&resp).expect("server send");
        }
        ctx.destroy();
    });

    thread::sleep(std::time::Duration::from_millis(50));
    let mut client = ShmContext::client_attach(TEST_RUN_DIR, svc, sid).expect("client attach");

    let mut resp_buf = vec![0u8; 65536];
    for i in 0u64..10 {
        let payload = vec![i as u8];
        let msg = build_message(protocol::KIND_REQUEST, 1, i + 1, &payload);
        client.send(&msg).expect("client send");
        let rlen = client.receive(&mut resp_buf, 5000).expect("client receive");
        let resp = &resp_buf[..rlen];
        let rhdr = protocol::Header::decode(resp).expect("decode");
        assert_eq!(rhdr.kind, protocol::KIND_RESPONSE);
        assert_eq!(rhdr.message_id, i + 1);
        assert_eq!(resp[protocol::HEADER_SIZE], i as u8);
    }

    client.close();
    server_thread.join().unwrap();
    cleanup_shm(svc, sid);
}

#[test]
fn test_stale_recovery() {
    ensure_run_dir();
    let svc = "rs_shm_stale";
    let sid: u64 = 3;
    cleanup_shm(svc, sid);

    // Create a region, corrupt owner_pid to simulate dead process
    let mut first =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("first create");
    let hdr = first.base as *mut RegionHeader;
    unsafe { (*hdr).owner_pid = 99999 }; // very unlikely alive
    first.close(); // close without unlink

    // cleanup_stale should remove the stale file
    cleanup_stale(TEST_RUN_DIR, svc);

    // Now O_EXCL create should succeed
    let mut second = ShmContext::server_create(TEST_RUN_DIR, svc, sid, 2048, 2048)
        .expect("stale recovery create");
    assert!(second.request_capacity >= 2048);
    second.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_large_message() {
    ensure_run_dir();
    let svc = "rs_shm_large";
    let sid: u64 = 4;
    cleanup_shm(svc, sid);

    let svc_clone = svc.to_string();
    let server_thread = thread::spawn(move || {
        let mut ctx = ShmContext::server_create(TEST_RUN_DIR, &svc_clone, sid, 65536, 65536)
            .expect("server create");
        let mut buf = vec![0u8; 65536];
        let mlen = ctx.receive(&mut buf, 5000).expect("server receive");
        let msg = &buf[..mlen];
        let hdr = protocol::Header::decode(msg).expect("decode");
        let payload = msg[protocol::HEADER_SIZE..].to_vec();
        let resp = build_message(protocol::KIND_RESPONSE, hdr.code, hdr.message_id, &payload);
        ctx.send(&resp).expect("server send");
        ctx.destroy();
    });

    thread::sleep(std::time::Duration::from_millis(50));
    let mut client = ShmContext::client_attach(TEST_RUN_DIR, svc, sid).expect("client attach");

    // 60000 bytes of payload
    let payload: Vec<u8> = (0..60000).map(|i| (i & 0xFF) as u8).collect();
    let msg = build_message(protocol::KIND_REQUEST, 1, 999, &payload);
    client.send(&msg).expect("send large");

    let mut resp_buf = vec![0u8; 65536];
    let rlen = client.receive(&mut resp_buf, 5000).expect("receive large");
    let resp = &resp_buf[..rlen];
    assert_eq!(resp.len(), protocol::HEADER_SIZE + payload.len());
    assert_eq!(&resp[protocol::HEADER_SIZE..], &payload[..]);

    client.close();
    server_thread.join().unwrap();
    cleanup_shm(svc, sid);
}

#[test]
fn test_shm_chaos_forged_length() {
    // Verify that forged/malicious req_len and resp_len values in the SHM
    // header are handled safely: no panic, no out-of-bounds read.
    ensure_run_dir();
    let svc = "rs_shm_forge";
    let sid: u64 = 100;
    cleanup_shm(svc, sid);

    let mut server_ctx =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");

    let mut client_ctx = ShmContext::client_attach(TEST_RUN_DIR, svc, sid).expect("client attach");

    let base = server_ctx.base;
    let req_cap = server_ctx.request_capacity as usize;
    let resp_cap = server_ctx.response_capacity as usize;

    // --- Test forged req_len (server-side receive) ---

    // Forged lengths: 0 → BadHeader (corruption), within capacity → Ok,
    // over capacity → MsgTooLarge.
    let forged_req_lengths: &[(u32, bool)] = &[
        // (0, ...) tested separately below as BadHeader
        (1, false),                  // tiny valid
        (req_cap as u32 - 1, false), // just under capacity
        (req_cap as u32, false),     // exactly at capacity
        (req_cap as u32 + 1, true),  // one over capacity
        (0x0001_0000, true),         // moderately large
        (0x7FFF_FFFF, true),         // large positive
        (0xFFFF_FFFF, true),         // u32::MAX
    ];

    let mut recv_buf = vec![0u8; 65536];

    for &(forged_len, expect_too_large) in forged_req_lengths {
        // Write garbage into the request area
        unsafe {
            let req_area = base.add(server_ctx.request_offset as usize);
            std::ptr::write_bytes(req_area, 0xAB, req_cap);
        }

        // Store forged req_len at offset 48
        atomic_store_u32(base, OFF_REQ_LEN, forged_len);

        // Increment req_seq at offset 32 to signal a new "message"
        atomic_add_u64(base, OFF_REQ_SEQ, 1);

        // Bump req_signal at offset 56 to wake futex
        atomic_add_u32(base, OFF_REQ_SIGNAL, 1);

        let result = server_ctx.receive(&mut recv_buf, 100);

        if expect_too_large {
            assert_eq!(
                result.unwrap_err(),
                ShmError::MsgTooLarge,
                "forged req_len={forged_len:#x} should return MsgTooLarge"
            );
        } else {
            let n = result.unwrap_or_else(|e| {
                panic!("forged req_len={forged_len:#x} should succeed, got {e:?}")
            });
            assert_eq!(
                n, forged_len as usize,
                "returned length should match forged req_len={forged_len:#x}"
            );
        }
    }

    // --- Test forged resp_len (client-side receive) ---

    let forged_resp_lengths: &[(u32, bool)] = &[
        // (0, ...) tested separately below as BadHeader
        (1, false),
        (resp_cap as u32 - 1, false),
        (resp_cap as u32, false),
        (resp_cap as u32 + 1, true),
        (0x0001_0000, true),
        (0x7FFF_FFFF, true),
        (0xFFFF_FFFF, true),
    ];

    for &(forged_len, expect_too_large) in forged_resp_lengths {
        // Write garbage into the response area
        unsafe {
            let resp_area = base.add(server_ctx.response_offset as usize);
            std::ptr::write_bytes(resp_area, 0xCD, resp_cap);
        }

        // Store forged resp_len at offset 52
        atomic_store_u32(base, OFF_RESP_LEN, forged_len);

        // Increment resp_seq at offset 40
        atomic_add_u64(base, OFF_RESP_SEQ, 1);

        // Bump resp_signal at offset 60
        atomic_add_u32(base, OFF_RESP_SIGNAL, 1);

        let result = client_ctx.receive(&mut recv_buf, 100);

        if expect_too_large {
            assert_eq!(
                result.unwrap_err(),
                ShmError::MsgTooLarge,
                "forged resp_len={forged_len:#x} should return MsgTooLarge"
            );
        } else {
            let n = result.unwrap_or_else(|e| {
                panic!("forged resp_len={forged_len:#x} should succeed, got {e:?}")
            });
            assert_eq!(
                n, forged_len as usize,
                "returned length should match forged resp_len={forged_len:#x}"
            );
        }
    }

    // --- mlen==0 → BadHeader (corruption indicator) ---
    // Server-side: forge req_len=0
    atomic_store_u32(base, OFF_REQ_LEN, 0);
    atomic_add_u64(base, OFF_REQ_SEQ, 1);
    atomic_add_u32(base, OFF_REQ_SIGNAL, 1);
    assert_eq!(
        server_ctx.receive(&mut recv_buf, 100).unwrap_err(),
        ShmError::BadHeader,
        "forged req_len=0 should return BadHeader"
    );

    // Client-side: forge resp_len=0
    atomic_store_u32(base, OFF_RESP_LEN, 0);
    atomic_add_u64(base, OFF_RESP_SEQ, 1);
    atomic_add_u32(base, OFF_RESP_SIGNAL, 1);
    assert_eq!(
        client_ctx.receive(&mut recv_buf, 100).unwrap_err(),
        ShmError::BadHeader,
        "forged resp_len=0 should return BadHeader"
    );

    client_ctx.close();
    server_ctx.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_shm_multi_client() {
    // Verify multiple clients with independent SHM regions have no
    // cross-contamination: each client/server pair exchanges unique data.
    ensure_run_dir();
    let svc = "rs_shm_mc";
    let session_ids: [u64; 3] = [1, 2, 3];

    for &sid in &session_ids {
        cleanup_shm(svc, sid);
    }

    let svc_name = svc.to_string();

    // Spawn 3 server threads, one per session
    let server_handles: Vec<_> = session_ids
        .iter()
        .map(|&sid| {
            let svc_clone = svc_name.clone();
            thread::spawn(move || {
                let mut ctx = ShmContext::server_create(TEST_RUN_DIR, &svc_clone, sid, 4096, 4096)
                    .expect(&format!("server create sid={sid}"));

                // Receive request
                let mut buf = vec![0u8; 8192];
                let mlen = ctx
                    .receive(&mut buf, 5000)
                    .expect(&format!("server receive sid={sid}"));
                let msg = &buf[..mlen];
                let hdr = protocol::Header::decode(msg).expect("decode req");
                let req_payload = msg[protocol::HEADER_SIZE..].to_vec();

                // Echo back as response with same message_id
                let resp = build_message(
                    protocol::KIND_RESPONSE,
                    hdr.code,
                    hdr.message_id,
                    &req_payload,
                );
                ctx.send(&resp).expect(&format!("server send sid={sid}"));
                ctx.destroy();
            })
        })
        .collect();

    // Let servers initialize
    thread::sleep(std::time::Duration::from_millis(100));

    // Attach 3 clients, send unique payloads, verify isolation
    let client_handles: Vec<_> = session_ids
        .iter()
        .map(|&sid| {
            let svc_clone = svc_name.clone();
            thread::spawn(move || {
                let mut client = ShmContext::client_attach(TEST_RUN_DIR, &svc_clone, sid)
                    .expect(&format!("client attach sid={sid}"));

                // Unique payload: session_id repeated to fill a pattern
                let unique_byte = sid as u8;
                let payload: Vec<u8> = vec![unique_byte; 64];
                let msg_id = 1000 + sid;

                let msg = build_message(
                    protocol::KIND_REQUEST,
                    protocol::METHOD_INCREMENT,
                    msg_id,
                    &payload,
                );
                client.send(&msg).expect(&format!("client send sid={sid}"));

                // Receive response
                let mut resp_buf = vec![0u8; 8192];
                let rlen = client
                    .receive(&mut resp_buf, 5000)
                    .expect(&format!("client receive sid={sid}"));
                let resp = &resp_buf[..rlen];

                let rhdr = protocol::Header::decode(resp).expect("decode resp");
                assert_eq!(rhdr.kind, protocol::KIND_RESPONSE);
                assert_eq!(
                    rhdr.message_id, msg_id,
                    "sid={sid}: message_id mismatch (cross-contamination?)"
                );

                // Verify the payload matches what we sent
                let resp_payload = &resp[protocol::HEADER_SIZE..];
                assert_eq!(
                    resp_payload,
                    &payload[..],
                    "sid={sid}: payload mismatch (cross-contamination?)"
                );

                // Verify every byte is our unique marker
                for (i, &b) in resp_payload.iter().enumerate() {
                    assert_eq!(
                        b, unique_byte,
                        "sid={sid}: byte {i} is {b:#x}, expected {unique_byte:#x}"
                    );
                }

                client.close();
            })
        })
        .collect();

    for h in client_handles {
        h.join().unwrap();
    }
    for h in server_handles {
        h.join().unwrap();
    }

    for &sid in &session_ids {
        cleanup_shm(svc, sid);
    }
}

// -----------------------------------------------------------------------
//  ShmError Display coverage (lines 73-88)
// -----------------------------------------------------------------------

#[test]
fn shm_error_display_all_variants() {
    let cases: Vec<(ShmError, &str)> = vec![
        (ShmError::PathTooLong, "SHM path exceeds limit"),
        (ShmError::Open(2), "open failed: errno 2"),
        (ShmError::Truncate(28), "ftruncate failed: errno 28"),
        (ShmError::Mmap(12), "mmap failed: errno 12"),
        (ShmError::BadMagic, "SHM header magic mismatch"),
        (ShmError::BadVersion, "SHM header version mismatch"),
        (ShmError::BadHeader, "SHM header_len mismatch"),
        (ShmError::BadSize, "SHM file too small for declared areas"),
        (ShmError::AddrInUse, "SHM region owned by live server"),
        (ShmError::NotReady, "SHM server not ready"),
        (ShmError::MsgTooLarge, "message exceeds SHM area capacity"),
        (ShmError::Timeout, "SHM futex wait timed out"),
        (ShmError::BadParam("test".into()), "bad parameter: test"),
        (ShmError::PeerDead, "SHM owner process has exited"),
    ];
    for (err, expected) in cases {
        assert_eq!(format!("{}", err), expected);
    }
    let e: &dyn std::error::Error = &ShmError::PathTooLong;
    let _ = format!("{e}");
}

// -----------------------------------------------------------------------
//  ShmContext accessors: role(), fd() (lines 164-165, 169-170)
// -----------------------------------------------------------------------

#[test]
fn test_shm_role_and_fd() {
    ensure_run_dir();
    let svc = "rs_shm_role";
    let sid: u64 = 50;
    cleanup_shm(svc, sid);

    let server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");
    assert_eq!(server.role(), ShmRole::Server);
    assert!(server.fd() >= 0);

    let client = ShmContext::client_attach(TEST_RUN_DIR, svc, sid).expect("client attach");
    assert_eq!(client.role(), ShmRole::Client);
    assert!(client.fd() >= 0);

    // Cleanup
    let mut c = client;
    let mut s = server;
    c.close();
    s.destroy();
    cleanup_shm(svc, sid);
}

// -----------------------------------------------------------------------
//  ShmContext::owner_alive() (lines 174-191)
// -----------------------------------------------------------------------

#[test]
fn test_owner_alive() {
    ensure_run_dir();
    let svc = "rs_shm_alive";
    let sid: u64 = 51;
    cleanup_shm(svc, sid);

    let server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");

    // Owner is the current process, so should be alive
    assert!(server.owner_alive());

    // Client should also report alive (same process owns it)
    let client = ShmContext::client_attach(TEST_RUN_DIR, svc, sid).expect("client attach");
    assert!(client.owner_alive());

    let mut c = client;
    let mut s = server;
    c.close();
    s.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_owner_alive_dead_pid() {
    ensure_run_dir();
    let svc = "rs_shm_dead";
    let sid: u64 = 52;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");

    // Forge a dead PID
    let hdr = server.base as *mut RegionHeader;
    unsafe { (*hdr).owner_pid = 99999 };

    assert!(!server.owner_alive());

    server.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_owner_alive_null_base() {
    ensure_run_dir();
    let svc = "rs_shm_null";
    let sid: u64 = 53;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");

    // Null base -> not alive
    let saved_base = server.base;
    server.base = std::ptr::null_mut();
    assert!(!server.owner_alive());

    // Restore for cleanup
    server.base = saved_base;
    server.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_owner_alive_generation_mismatch() {
    ensure_run_dir();
    let svc = "rs_shm_gen";
    let sid: u64 = 54;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");

    // Forge a different generation in the header
    let hdr = server.base as *mut RegionHeader;
    unsafe { (*hdr).owner_generation = server.owner_generation + 1 };

    // Generation mismatch -> not alive
    assert!(!server.owner_alive());

    server.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_owner_alive_legacy_generation_skips_generation_check() {
    ensure_run_dir();
    let svc = "rs_shm_gen_legacy";
    let sid: u64 = 55;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");

    let hdr = server.base as *mut RegionHeader;
    unsafe { (*hdr).owner_generation = server.owner_generation.wrapping_add(123) };
    server.owner_generation = 0;

    assert!(
        server.owner_alive(),
        "cached generation 0 should skip owner-generation mismatch checks"
    );

    server.destroy();
    cleanup_shm(svc, sid);
}

// -----------------------------------------------------------------------
//  ShmContext::send() error paths (lines 436-437, 458)
// -----------------------------------------------------------------------

#[test]
fn test_send_null_context() {
    ensure_run_dir();
    let svc = "rs_shm_sendnull";
    let sid: u64 = 55;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");

    // Empty message -> error
    assert!(matches!(server.send(&[]), Err(ShmError::BadParam(_))));

    // Null base -> error
    let saved_base = server.base;
    server.base = std::ptr::null_mut();
    assert!(matches!(
        server.send(&[1, 2, 3]),
        Err(ShmError::BadParam(_))
    ));
    server.base = saved_base;

    server.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_send_msg_too_large() {
    ensure_run_dir();
    let svc = "rs_shm_sendlrg";
    let sid: u64 = 56;
    cleanup_shm(svc, sid);

    // Small capacity
    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 64, 64).expect("server create");

    // Create a message bigger than response capacity
    let big = vec![0u8; 1024];
    assert_eq!(server.send(&big).unwrap_err(), ShmError::MsgTooLarge);

    server.destroy();
    cleanup_shm(svc, sid);
}

// -----------------------------------------------------------------------
//  ShmContext::receive() error paths (lines 494-498)
// -----------------------------------------------------------------------

#[test]
fn test_receive_null_context() {
    ensure_run_dir();
    let svc = "rs_shm_recvnull";
    let sid: u64 = 57;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");

    // Empty buffer -> error
    assert!(matches!(
        server.receive(&mut [], 100),
        Err(ShmError::BadParam(_))
    ));

    // Null base -> error
    let saved_base = server.base;
    server.base = std::ptr::null_mut();
    let mut buf = [0u8; 64];
    assert!(matches!(
        server.receive(&mut buf, 100),
        Err(ShmError::BadParam(_))
    ));
    server.base = saved_base;

    server.destroy();
    cleanup_shm(svc, sid);
}

// -----------------------------------------------------------------------
//  Timeout (line 575, 593)
// -----------------------------------------------------------------------

#[test]
fn test_receive_timeout() {
    ensure_run_dir();
    let svc = "rs_shm_timeout";
    let sid: u64 = 58;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");

    // No client sends anything, so receive must timeout
    let mut buf = [0u8; 1024];
    let result = server.receive(&mut buf, 50); // 50ms timeout
    assert_eq!(result.unwrap_err(), ShmError::Timeout);

    server.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_receive_without_deadline_waits_for_message() {
    ensure_run_dir();
    let svc = "rs_shm_wait_forever";
    let sid: u64 = 580;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");
    let mut client = ShmContext::client_attach(TEST_RUN_DIR, svc, sid).expect("client attach");

    let sender = thread::spawn({
        let svc_name = svc.to_string();
        move || {
            thread::sleep(std::time::Duration::from_millis(50));
            let mut ctx =
                ShmContext::client_attach(TEST_RUN_DIR, &svc_name, sid).expect("attach sender");
            let msg = build_message(protocol::KIND_REQUEST, 1, 9, &[0xAA, 0xBB]);
            ctx.send(&msg).expect("send");
            ctx.close();
        }
    });

    let mut buf = [0u8; 128];
    let n = server.receive(&mut buf, 0).expect("receive with timeout 0");
    assert_eq!(n, protocol::HEADER_SIZE + 2);

    sender.join().unwrap();
    client.close();
    server.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_receive_with_timeout_wakes_for_message() {
    ensure_run_dir();
    let svc = "rs_shm_wait_budget";
    let sid: u64 = 581;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");
    let mut client = ShmContext::client_attach(TEST_RUN_DIR, svc, sid).expect("client attach");

    let sender = thread::spawn({
        let svc_name = svc.to_string();
        move || {
            thread::sleep(std::time::Duration::from_millis(50));
            let mut ctx =
                ShmContext::client_attach(TEST_RUN_DIR, &svc_name, sid).expect("attach sender");
            let msg = build_message(protocol::KIND_REQUEST, 1, 10, &[0xCC, 0xDD, 0xEE]);
            ctx.send(&msg).expect("send");
            ctx.close();
        }
    });

    let mut buf = [0u8; 128];
    let n = server
        .receive(&mut buf, 5000)
        .expect("receive with finite timeout");
    assert_eq!(n, protocol::HEADER_SIZE + 3);

    let hdr = protocol::Header::decode(&buf[..protocol::HEADER_SIZE]).expect("decode");
    assert_eq!(hdr.kind, protocol::KIND_REQUEST);
    assert_eq!(hdr.message_id, 10);
    assert_eq!(&buf[protocol::HEADER_SIZE..n], &[0xCC, 0xDD, 0xEE]);

    sender.join().unwrap();
    client.close();
    server.destroy();
    cleanup_shm(svc, sid);
}

// -----------------------------------------------------------------------
//  Client attach: bad magic, bad version, bad header_len (lines 366-385)
// -----------------------------------------------------------------------

#[test]
fn test_client_attach_bad_magic() {
    ensure_run_dir();
    let svc = "rs_shm_badmag";
    let sid: u64 = 59;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");

    // Corrupt magic
    let hdr = server.base as *mut RegionHeader;
    unsafe { (*hdr).magic = 0xDEADBEEF };

    let result = ShmContext::client_attach(TEST_RUN_DIR, svc, sid);
    assert!(matches!(result, Err(ShmError::BadMagic)));

    // Restore for cleanup
    unsafe { (*hdr).magic = REGION_MAGIC };
    server.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_client_attach_bad_version() {
    ensure_run_dir();
    let svc = "rs_shm_badver";
    let sid: u64 = 60;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");

    let hdr = server.base as *mut RegionHeader;
    unsafe { (*hdr).version = 99 };

    let result = ShmContext::client_attach(TEST_RUN_DIR, svc, sid);
    assert!(matches!(result, Err(ShmError::BadVersion)));

    unsafe { (*hdr).version = REGION_VERSION };
    server.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_client_attach_bad_header_len() {
    ensure_run_dir();
    let svc = "rs_shm_badhdr";
    let sid: u64 = 61;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");

    let hdr = server.base as *mut RegionHeader;
    unsafe { (*hdr).header_len = 128 };

    let result = ShmContext::client_attach(TEST_RUN_DIR, svc, sid);
    assert!(matches!(result, Err(ShmError::BadHeader)));

    unsafe { (*hdr).header_len = HEADER_LEN };
    server.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_client_attach_partial_header_not_ready() {
    ensure_run_dir();
    let svc = "rs_shm_partial";
    let sid: u64 = 63;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");

    let hdr = server.base as *mut RegionHeader;
    let saved = unsafe {
        (
            (*hdr).request_offset,
            (*hdr).request_capacity,
            (*hdr).response_offset,
            (*hdr).response_capacity,
        )
    };

    unsafe {
        (*hdr).request_offset = 0;
        (*hdr).request_capacity = 0;
        (*hdr).response_offset = 0;
        (*hdr).response_capacity = 0;
    }

    let result = ShmContext::client_attach(TEST_RUN_DIR, svc, sid);
    assert!(matches!(result, Err(ShmError::NotReady)));

    unsafe {
        (*hdr).request_offset = saved.0;
        (*hdr).request_capacity = saved.1;
        (*hdr).response_offset = saved.2;
        (*hdr).response_capacity = saved.3;
    }
    server.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_server_create_rejects_live_region() {
    ensure_run_dir();
    let svc = "rs_shm_live_region";
    let sid: u64 = 631;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");
    let err = match ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024) {
        Ok(_) => panic!("live region should reject second create"),
        Err(err) => err,
    };
    assert_eq!(err, ShmError::AddrInUse);

    server.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_server_create_recovers_invalid_stale_file() {
    ensure_run_dir();
    let svc = "rs_shm_invalid_stale_retry";
    let sid: u64 = 6311;
    cleanup_shm(svc, sid);

    let path = build_shm_path(TEST_RUN_DIR, svc, sid).expect("path");
    std::fs::write(&path, [0u8; 8]).expect("write stale short file");

    let mut server = ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024)
        .expect("server create should recover invalid stale file");
    assert_eq!(server.role, ShmRole::Server);

    server.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_client_attach_short_file_not_ready() {
    ensure_run_dir();
    let svc = "rs_shm_short";
    let sid: u64 = 632;
    cleanup_shm(svc, sid);

    let path = build_shm_path(TEST_RUN_DIR, svc, sid).expect("path");
    let c_path = path_to_cstring(&path).expect("cstring");
    let fd = unsafe {
        libc::open(
            c_path.as_ptr(),
            libc::O_RDWR | libc::O_CREAT | libc::O_TRUNC,
            0o600,
        )
    };
    assert!(fd >= 0, "open failed: {}", errno());
    assert_eq!(unsafe { libc::ftruncate(fd, 8) }, 0, "ftruncate failed");
    unsafe { libc::close(fd) };

    let err = match ShmContext::client_attach(TEST_RUN_DIR, svc, sid) {
        Ok(_) => panic!("short file should not attach"),
        Err(err) => err,
    };
    assert_eq!(err, ShmError::NotReady);

    let _ = std::fs::remove_file(path);
}

#[test]
fn test_client_attach_region_smaller_than_declared_capacity() {
    ensure_run_dir();
    let svc = "rs_shm_truncated_region";
    let sid: u64 = 633;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");
    assert_eq!(
        unsafe { libc::ftruncate(server.fd, HEADER_LEN as libc::off_t) },
        0,
        "truncate region"
    );

    let err = match ShmContext::client_attach(TEST_RUN_DIR, svc, sid) {
        Ok(_) => panic!("truncated region should not attach"),
        Err(err) => err,
    };
    assert_eq!(err, ShmError::BadSize);

    server.destroy();
    cleanup_shm(svc, sid);
}

#[test]
fn test_client_attach_bad_size() {
    ensure_run_dir();
    let svc = "rs_shm_badsz";
    let sid: u64 = 64;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");

    // Corrupt response capacity to be huge, so region_size < needed
    let hdr = server.base as *mut RegionHeader;
    let saved_cap = unsafe { (*hdr).response_capacity };
    unsafe { (*hdr).response_capacity = 0xFFFF_FFFF };

    let result = ShmContext::client_attach(TEST_RUN_DIR, svc, sid);
    assert!(matches!(result, Err(ShmError::BadSize)));

    unsafe { (*hdr).response_capacity = saved_cap };
    server.destroy();
    cleanup_shm(svc, sid);
}

// -----------------------------------------------------------------------
//  validate_service_name (lines 764-778)
// -----------------------------------------------------------------------

#[test]
fn test_validate_service_name() {
    assert!(validate_service_name("").is_err());
    assert!(validate_service_name(".").is_err());
    assert!(validate_service_name("..").is_err());
    assert!(validate_service_name("a/b").is_err());
    assert!(validate_service_name("a b").is_err());
    assert!(validate_service_name("a@b").is_err());

    assert!(validate_service_name("valid").is_ok());
    assert!(validate_service_name("valid-name").is_ok());
    assert!(validate_service_name("valid_name").is_ok());
    assert!(validate_service_name("valid.name.123").is_ok());
}

// -----------------------------------------------------------------------
//  build_shm_path: path too long (line 785)
// -----------------------------------------------------------------------

#[test]
fn test_build_shm_path_too_long() {
    // Path = "{run_dir}/{name}-{session_id:016x}.ipcshm" must be >= 256
    // /tmp/aaa...aaa-0000000000000001.ipcshm
    // 5 + name_len + 1 + 16 + 7 = 29 + name_len >= 256 -> name_len >= 227
    let long_name = "a".repeat(230);
    let result = build_shm_path("/tmp", &long_name, 1);
    assert!(matches!(result, Err(ShmError::PathTooLong)));
}

// -----------------------------------------------------------------------
//  pid_alive edge cases (lines 800-804)
// -----------------------------------------------------------------------

#[test]
fn test_pid_alive() {
    assert!(!pid_alive(0));
    assert!(!pid_alive(-1));
    // Current process should be alive
    assert!(pid_alive(unsafe { libc::getpid() }));
    // Very unlikely PID
    assert!(!pid_alive(99999));
}

// -----------------------------------------------------------------------
//  Client attach: nonexistent file (line 325)
// -----------------------------------------------------------------------

#[test]
fn test_client_attach_nonexistent() {
    ensure_run_dir();
    let result = ShmContext::client_attach(TEST_RUN_DIR, "rs_shm_nofile", 99999);
    assert!(matches!(result, Err(ShmError::Open(_))));
}

#[test]
fn test_check_shm_stale_nonexistent_returns_not_exist() {
    ensure_run_dir();
    let svc = "rs_shm_stale_missing";
    let sid: u64 = 700;
    cleanup_shm(svc, sid);

    let path = build_shm_path(TEST_RUN_DIR, svc, sid).expect("path");
    assert!(matches!(check_shm_stale(&path), StaleResult::NotExist));
}

#[test]
fn test_check_shm_stale_invalid_cstring_returns_not_exist() {
    let bad_path = PathBuf::from(OsString::from_vec(vec![
        b'/', b't', b'm', b'p', b'/', b'n', b'i', b'p', b'c', 0, b'b',
    ]));
    assert!(matches!(check_shm_stale(&bad_path), StaleResult::NotExist));
}

#[test]
fn test_cleanup_stale_missing_run_dir_is_noop() {
    cleanup_stale("/tmp/nipc_shm_rust_missing_dir", "rs_shm_missing");
}

#[test]
fn test_cleanup_stale_ignores_unrelated_and_non_utf8_entries() {
    ensure_run_dir();
    let svc = "rs_shm_cleanup_skip";
    let unrelated = PathBuf::from(format!("{TEST_RUN_DIR}/not-a-shm-entry.txt"));
    let invalid_name = OsString::from_vec(vec![
        b'r', b's', b'_', b's', b'h', b'm', b'_', b'c', b'l', b'e', b'a', b'n', b'u', b'p', b'_',
        b's', b'k', b'i', b'p', b'-', 0xff, b'.', b'i', b'p', b'c', b's', b'h', b'm',
    ]);
    let invalid_path = PathBuf::from(TEST_RUN_DIR).join(invalid_name);

    let _ = std::fs::remove_file(&unrelated);
    let _ = std::fs::remove_file(&invalid_path);
    std::fs::write(&unrelated, b"skip").expect("write unrelated file");
    std::fs::write(&invalid_path, b"skip").expect("write invalid utf8 file");

    cleanup_stale(TEST_RUN_DIR, svc);

    assert!(unrelated.exists(), "unrelated entries should be ignored");
    assert!(invalid_path.exists(), "non-UTF8 entries should be ignored");

    let _ = std::fs::remove_file(&unrelated);
    let _ = std::fs::remove_file(&invalid_path);
}

#[test]
fn test_cleanup_stale_invalid_entries() {
    ensure_run_dir();
    let svc = "rs_shm_cleanup_invalid";
    let short_sid: u64 = 701;
    let magic_sid: u64 = 702;
    let unreadable_sid: u64 = 703;

    for sid in [short_sid, magic_sid, unreadable_sid] {
        cleanup_shm(svc, sid);
    }

    let short_path = build_shm_path(TEST_RUN_DIR, svc, short_sid).expect("short path");
    std::fs::write(&short_path, [0u8; 8]).expect("write short file");

    let mut bad_magic =
        ShmContext::server_create(TEST_RUN_DIR, svc, magic_sid, 1024, 1024).expect("server create");
    let hdr = bad_magic.base as *mut RegionHeader;
    unsafe { (*hdr).magic = 0xDEADBEEF };
    bad_magic.close();

    let unreadable_path = build_shm_path(TEST_RUN_DIR, svc, unreadable_sid).expect("path");
    let unreadable_c = path_to_cstring(&unreadable_path).expect("cstring");
    let unreadable_fd = unsafe {
        libc::open(
            unreadable_c.as_ptr(),
            libc::O_RDWR | libc::O_CREAT | libc::O_TRUNC,
            0o000,
        )
    };
    assert!(unreadable_fd >= 0, "open unreadable");
    unsafe { libc::close(unreadable_fd) };

    cleanup_stale(TEST_RUN_DIR, svc);

    assert!(
        !short_path.exists(),
        "short invalid entry should be removed"
    );
    assert!(!build_shm_path(TEST_RUN_DIR, svc, magic_sid)
        .unwrap()
        .exists());
    // Under non-root: unreadable file is preserved (EACCES skip).
    // Under root: chmod 000 has no effect, so the file gets opened,
    // inspected, and removed normally.
    if unsafe { libc::geteuid() } != 0 {
        assert!(
            unreadable_path.exists(),
            "unreadable entry should be preserved (EACCES skip)"
        );
    }
    // Clean up
    unsafe { libc::chmod(unreadable_c.as_ptr(), 0o600) };
    std::fs::remove_file(&unreadable_path).ok();
}

#[test]
fn test_check_shm_stale_short_file_invalid() {
    ensure_run_dir();
    let svc = "rs_shm_stale_short_direct";
    let sid: u64 = 704;
    cleanup_shm(svc, sid);

    let path = build_shm_path(TEST_RUN_DIR, svc, sid).expect("path");
    std::fs::write(&path, [0u8; 8]).expect("write short file");

    assert!(matches!(check_shm_stale(&path), StaleResult::Invalid));
    assert!(!path.exists(), "short stale file should be removed");
}

#[test]
fn test_check_shm_stale_bad_magic_invalid() {
    ensure_run_dir();
    let svc = "rs_shm_stale_magic_direct";
    let sid: u64 = 705;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");
    let hdr = server.base as *mut RegionHeader;
    unsafe { (*hdr).magic = 0xDEADBEEF };
    server.close();

    let path = build_shm_path(TEST_RUN_DIR, svc, sid).expect("path");
    assert!(matches!(check_shm_stale(&path), StaleResult::Invalid));
    assert!(!path.exists(), "bad magic stale file should be removed");
}

#[test]
fn test_check_shm_stale_zero_generation_recovers() {
    ensure_run_dir();
    let svc = "rs_shm_stale_zero_gen";
    let sid: u64 = 7051;
    cleanup_shm(svc, sid);

    let mut server =
        ShmContext::server_create(TEST_RUN_DIR, svc, sid, 1024, 1024).expect("server create");
    let hdr = server.base as *mut RegionHeader;
    unsafe { (*hdr).owner_generation = 0 };
    server.close();

    let path = build_shm_path(TEST_RUN_DIR, svc, sid).expect("path");
    assert!(matches!(check_shm_stale(&path), StaleResult::Recovered));
    assert!(
        !path.exists(),
        "zero-generation stale file should be removed"
    );
}

#[test]
fn test_check_shm_stale_open_failure_invalid() {
    ensure_run_dir();
    let svc = "rs_shm_stale_open_fail";
    let sid: u64 = 7052;
    cleanup_shm(svc, sid);

    let path = build_shm_path(TEST_RUN_DIR, svc, sid).expect("path");
    let c_path = path_to_cstring(&path).expect("cstring");
    let fd = unsafe {
        libc::open(
            c_path.as_ptr(),
            libc::O_CREAT | libc::O_TRUNC | libc::O_RDWR,
            0o600,
        )
    };
    assert!(fd >= 0, "open failed: {}", errno());
    assert_eq!(
        unsafe { libc::ftruncate(fd, HEADER_LEN as libc::off_t) },
        0,
        "ftruncate failed: {}",
        errno()
    );
    unsafe { libc::close(fd) };
    assert_eq!(
        unsafe { libc::chmod(c_path.as_ptr(), 0) },
        0,
        "chmod failed: {}",
        errno()
    );

    assert!(matches!(check_shm_stale(&path), StaleResult::Invalid));
    // Under non-root: file preserved (EACCES). Under root: chmod 000
    // has no effect, so the file is opened, inspected, and removed.
    if unsafe { libc::geteuid() } != 0 {
        assert!(
            path.exists(),
            "unopenable file should be preserved (EACCES skip)"
        );
    }
    unsafe { libc::chmod(c_path.as_ptr(), 0o600) };
    std::fs::remove_file(&path).ok();
}

#[test]
fn test_check_shm_stale_directory_symlink_invalid() {
    ensure_run_dir();
    let svc = "rs_shm_stale_dir_symlink";
    let sid: u64 = 7053;
    cleanup_shm(svc, sid);

    let path = build_shm_path(TEST_RUN_DIR, svc, sid).expect("path");
    let target = PathBuf::from(format!("{TEST_RUN_DIR}/{svc}-target-dir"));
    let _ = std::fs::remove_dir_all(&target);
    std::fs::create_dir_all(&target).expect("create target dir");
    std::os::unix::fs::symlink(&target, &path).expect("create symlink");

    assert!(matches!(check_shm_stale(&path), StaleResult::Invalid));
    assert!(
        !path.exists(),
        "directory symlink stale entry should be removed"
    );
    assert!(target.is_dir(), "target directory should remain");

    std::fs::remove_dir_all(&target).expect("remove target dir");
}

#[test]
fn test_cleanup_stale_unlinks_dangling_symlink() {
    ensure_run_dir();
    let svc = "rs_shm_cleanup_symlink";
    let sid: u64 = 706;
    cleanup_shm(svc, sid);

    let path = build_shm_path(TEST_RUN_DIR, svc, sid).expect("path");
    let target = path.with_extension("missing-target");
    let _ = std::fs::remove_file(&target);
    std::os::unix::fs::symlink(&target, &path).expect("create dangling symlink");

    cleanup_stale(TEST_RUN_DIR, svc);

    assert!(!path.exists(), "dangling symlink entry should be removed");
}

#[test]
fn test_cleanup_stale_unlinks_directory_symlink_when_mmap_fails() {
    ensure_run_dir();
    let svc = "rs_shm_cleanup_dir_symlink";
    let sid: u64 = 707;
    cleanup_shm(svc, sid);

    let path = build_shm_path(TEST_RUN_DIR, svc, sid).expect("path");
    let target = PathBuf::from(format!("{TEST_RUN_DIR}/{svc}-target-dir"));
    let _ = std::fs::remove_dir_all(&target);
    std::fs::create_dir_all(&target).expect("create target dir");
    std::os::unix::fs::symlink(&target, &path).expect("create symlink");

    cleanup_stale(TEST_RUN_DIR, svc);

    assert!(!path.exists(), "directory symlink entry should be removed");
    assert!(target.is_dir(), "target directory should remain");

    std::fs::remove_dir_all(&target).expect("remove target dir");
}
