//! Encode/decode test messages to/from files for cross-language interop testing.
//!
//! Usage:
//!   interop_codec encode <output_dir>   - encode all test messages to files
//!   interop_codec decode <input_dir>    - decode files and verify correctness
//!
//! Returns 0 on success, 1 on failure.

use netipc::protocol::*;
use std::fs;
use std::path::Path;
use std::process;

fn write_file(dir: &str, name: &str, data: &[u8]) {
    let path = Path::new(dir).join(name);
    fs::write(&path, data).unwrap_or_else(|e| {
        eprintln!("ERROR: cannot write {}: {}", path.display(), e);
        process::exit(1);
    });
}

fn read_file(dir: &str, name: &str) -> Vec<u8> {
    let path = Path::new(dir).join(name);
    fs::read(&path).unwrap_or_else(|e| {
        eprintln!("ERROR: cannot read {}: {}", path.display(), e);
        process::exit(1);
    })
}

struct Checker {
    pass: u32,
    fail: u32,
}

impl Checker {
    fn new() -> Self {
        Checker { pass: 0, fail: 0 }
    }

    fn check(&mut self, cond: bool, name: &str) {
        if cond {
            self.pass += 1;
        } else {
            self.fail += 1;
            eprintln!("FAIL: {}", name);
        }
    }

    fn report(&self, label: &str) -> bool {
        println!("{}: {} passed, {} failed", label, self.pass, self.fail);
        self.fail == 0
    }
}

fn do_encode(dir: &str) {
    // 1. Outer message header
    {
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
        let mut buf = [0u8; 32];
        h.encode(&mut buf);
        write_file(dir, "header.bin", &buf);
    }

    // 2. Chunk continuation header
    {
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
        let mut buf = [0u8; 32];
        c.encode(&mut buf);
        write_file(dir, "chunk_header.bin", &buf);
    }

    // 3. Hello payload
    {
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
        let mut buf = [0u8; 44];
        h.encode(&mut buf);
        write_file(dir, "hello.bin", &buf);
    }

    // 4. Hello-ack payload
    {
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
            session_id: 1,
        };
        let mut buf = [0u8; 48];
        h.encode(&mut buf);
        write_file(dir, "hello_ack.bin", &buf);
    }

    // 5. Cgroups request
    {
        let r = CgroupsRequest {
            layout_version: 1,
            flags: 0,
        };
        let mut buf = [0u8; 4];
        r.encode(&mut buf);
        write_file(dir, "cgroups_req.bin", &buf);
    }

    // 6. Cgroups snapshot response (multi-item)
    {
        let mut buf = [0u8; 8192];
        let mut b = CgroupsBuilder::new(&mut buf, 3, 1, 999);

        b.add(100, 0, 1, b"init.scope", b"/sys/fs/cgroup/init.scope")
            .unwrap();
        b.add(
            200,
            0x02,
            0,
            b"system.slice/docker-abc.scope",
            b"/sys/fs/cgroup/system.slice/docker-abc.scope",
        )
        .unwrap();
        b.add(300, 0, 1, b"", b"").unwrap();

        let total = b.finish();
        write_file(dir, "cgroups_resp.bin", &buf[..total]);
    }

    // 7. Empty cgroups snapshot
    {
        let mut buf = [0u8; 8192];
        let b = CgroupsBuilder::new(&mut buf, 0, 0, 42);
        let total = b.finish();
        write_file(dir, "cgroups_resp_empty.bin", &buf[..total]);
    }

    // 8. CGROUPS_LOOKUP request variants
    {
        let mut buf = [0u8; 8192];
        let total = encode_cgroups_lookup_request(
            &[b"/sys/fs/cgroup/a", b"/system.slice/docker-abc.scope"],
            &mut buf,
        )
        .unwrap();
        write_file(dir, "cgroups_lookup_req.bin", &buf[..total]);

        let total = encode_cgroups_lookup_request(&[], &mut buf).unwrap();
        write_file(dir, "cgroups_lookup_req_empty.bin", &buf[..total]);
    }

    // 9. CGROUPS_LOOKUP response variants
    {
        let mut buf = [0u8; 8192];
        let mut b = CgroupsLookupBuilder::new(&mut buf, 1, 100);
        b.add(
            CGROUP_LOOKUP_KNOWN,
            ORCHESTRATOR_K8S,
            b"/kubepods.slice/pod-a",
            b"pod-a",
            &[
                (b"namespace".as_slice(), b"default".as_slice()),
                (b"pod".as_slice(), b"web".as_slice()),
            ],
        )
        .unwrap();
        let total = b.finish().unwrap();
        write_file(
            dir,
            "cgroups_lookup_resp_known_with_labels.bin",
            &buf[..total],
        );
    }
    {
        let mut buf = [0u8; 8192];
        let mut b = CgroupsLookupBuilder::new(&mut buf, 1, 101);
        b.add(
            CGROUP_LOOKUP_KNOWN,
            ORCHESTRATOR_DOCKER,
            b"/docker/abc",
            b"",
            &[],
        )
        .unwrap();
        let total = b.finish().unwrap();
        write_file(
            dir,
            "cgroups_lookup_resp_known_no_labels.bin",
            &buf[..total],
        );
    }
    {
        let mut buf = [0u8; 8192];
        let mut b = CgroupsLookupBuilder::new(&mut buf, 1, 102);
        b.add(
            CGROUP_LOOKUP_UNKNOWN_RETRY_LATER,
            0,
            b"/missing/retry",
            b"",
            &[],
        )
        .unwrap();
        let total = b.finish().unwrap();
        write_file(dir, "cgroups_lookup_resp_unknown_retry.bin", &buf[..total]);
    }
    {
        let mut buf = [0u8; 8192];
        let mut b = CgroupsLookupBuilder::new(&mut buf, 1, 103);
        b.add(CGROUP_LOOKUP_UNKNOWN_PERMANENT, 0, b"/gone", b"", &[])
            .unwrap();
        let total = b.finish().unwrap();
        write_file(
            dir,
            "cgroups_lookup_resp_unknown_permanent.bin",
            &buf[..total],
        );
    }
    {
        let mut buf = [0u8; 8192];
        let b = CgroupsLookupBuilder::new(&mut buf, 0, 104);
        let total = b.finish().unwrap();
        write_file(dir, "cgroups_lookup_resp_empty.bin", &buf[..total]);
    }

    // 10. APPS_LOOKUP request variants
    {
        let mut buf = [0u8; 8192];
        let total = encode_apps_lookup_request(&[0, 1234, 4321], &mut buf).unwrap();
        write_file(dir, "apps_lookup_req.bin", &buf[..total]);

        let total = encode_apps_lookup_request(&[], &mut buf).unwrap();
        write_file(dir, "apps_lookup_req_empty.bin", &buf[..total]);
    }

    // 11. APPS_LOOKUP response variants
    {
        let mut buf = [0u8; 8192];
        let mut b = AppsLookupBuilder::new(&mut buf, 1, 200);
        b.add(
            PID_LOOKUP_KNOWN,
            APPS_CGROUP_KNOWN,
            ORCHESTRATOR_DOCKER,
            1234,
            1,
            1000,
            123456,
            b"123456789012345",
            b"/docker/abc",
            b"container-a",
            &[
                (b"image".as_slice(), b"nginx:latest".as_slice()),
                (b"service".as_slice(), b"web".as_slice()),
            ],
        )
        .unwrap();
        let total = b.finish().unwrap();
        write_file(dir, "apps_lookup_resp_known_full.bin", &buf[..total]);
    }
    {
        let mut buf = [0u8; 8192];
        let mut b = AppsLookupBuilder::new(&mut buf, 1, 201);
        b.add(
            PID_LOOKUP_KNOWN,
            APPS_CGROUP_UNKNOWN_RETRY_LATER,
            0,
            1235,
            1,
            1000,
            123457,
            b"app",
            b"/pending",
            b"",
            &[],
        )
        .unwrap();
        let total = b.finish().unwrap();
        write_file(dir, "apps_lookup_resp_known_retry.bin", &buf[..total]);
    }
    {
        let mut buf = [0u8; 8192];
        let mut b = AppsLookupBuilder::new(&mut buf, 1, 202);
        b.add(
            PID_LOOKUP_KNOWN,
            APPS_CGROUP_UNKNOWN_PERMANENT,
            0,
            1236,
            1,
            1000,
            123458,
            b"app2",
            b"/permanent",
            b"",
            &[],
        )
        .unwrap();
        let total = b.finish().unwrap();
        write_file(dir, "apps_lookup_resp_known_permanent.bin", &buf[..total]);
    }
    {
        let mut buf = [0u8; 8192];
        let mut b = AppsLookupBuilder::new(&mut buf, 1, 203);
        b.add(
            PID_LOOKUP_KNOWN,
            APPS_CGROUP_HOST_ROOT,
            0,
            1237,
            1,
            0,
            123459,
            b"sshd",
            b"",
            b"",
            &[],
        )
        .unwrap();
        let total = b.finish().unwrap();
        write_file(dir, "apps_lookup_resp_known_host_root.bin", &buf[..total]);
    }
    {
        let mut buf = [0u8; 8192];
        let mut b = AppsLookupBuilder::new(&mut buf, 1, 204);
        b.add(
            PID_LOOKUP_UNKNOWN,
            APPS_CGROUP_KNOWN,
            0,
            0,
            0,
            NIPC_UID_UNSET,
            0,
            b"",
            b"",
            b"",
            &[],
        )
        .unwrap();
        let total = b.finish().unwrap();
        write_file(dir, "apps_lookup_resp_unknown_pid.bin", &buf[..total]);
    }
    {
        let mut buf = [0u8; 8192];
        let b = AppsLookupBuilder::new(&mut buf, 0, 205);
        let total = b.finish().unwrap();
        write_file(dir, "apps_lookup_resp_empty.bin", &buf[..total]);
    }
}

fn do_decode(dir: &str) -> bool {
    let mut c = Checker::new();

    // 1. Outer message header
    {
        let data = read_file(dir, "header.bin");
        let hdr = Header::decode(&data);
        c.check(hdr.is_ok(), "decode header");
        if let Ok(h) = hdr {
            c.check(h.magic == MAGIC_MSG, "header magic");
            c.check(h.version == VERSION, "header version");
            c.check(h.header_len == HEADER_LEN, "header header_len");
            c.check(h.kind == KIND_REQUEST, "header kind");
            c.check(h.flags == FLAG_BATCH, "header flags");
            c.check(h.code == METHOD_CGROUPS_SNAPSHOT, "header code");
            c.check(h.transport_status == STATUS_OK, "header transport_status");
            c.check(h.payload_len == 12345, "header payload_len");
            c.check(h.item_count == 42, "header item_count");
            c.check(h.message_id == 0xDEAD_BEEF_CAFE_BABE, "header message_id");
        }
    }

    // 2. Chunk continuation header
    {
        let data = read_file(dir, "chunk_header.bin");
        let chk = ChunkHeader::decode(&data);
        c.check(chk.is_ok(), "decode chunk");
        if let Ok(ch) = chk {
            c.check(ch.magic == MAGIC_CHUNK, "chunk magic");
            c.check(ch.message_id == 0x1234_5678_90AB_CDEF, "chunk message_id");
            c.check(ch.total_message_len == 100000, "chunk total_message_len");
            c.check(ch.chunk_index == 3, "chunk chunk_index");
            c.check(ch.chunk_count == 10, "chunk chunk_count");
            c.check(ch.chunk_payload_len == 8192, "chunk chunk_payload_len");
        }
    }

    // 3. Hello payload
    {
        let data = read_file(dir, "hello.bin");
        let hello = Hello::decode(&data);
        c.check(hello.is_ok(), "decode hello");
        if let Ok(h) = hello {
            c.check(
                h.supported_profiles == (PROFILE_BASELINE | PROFILE_SHM_FUTEX),
                "hello supported",
            );
            c.check(h.preferred_profiles == PROFILE_SHM_FUTEX, "hello preferred");
            c.check(h.max_request_payload_bytes == 4096, "hello max_req_payload");
            c.check(h.max_request_batch_items == 100, "hello max_req_batch");
            c.check(
                h.max_response_payload_bytes == 1048576,
                "hello max_resp_payload",
            );
            c.check(h.max_response_batch_items == 1, "hello max_resp_batch");
            c.check(h.auth_token == 0xAABB_CCDD_EEFF_0011, "hello auth_token");
            c.check(h.packet_size == 65536, "hello packet_size");
        }
    }

    // 4. Hello-ack payload
    {
        let data = read_file(dir, "hello_ack.bin");
        let ack = HelloAck::decode(&data);
        c.check(ack.is_ok(), "decode hello_ack");
        if let Ok(h) = ack {
            c.check(
                h.server_supported_profiles == 0x07,
                "hello_ack server_supported",
            );
            c.check(h.intersection_profiles == 0x05, "hello_ack intersection");
            c.check(
                h.selected_profile == PROFILE_SHM_FUTEX,
                "hello_ack selected",
            );
            c.check(
                h.agreed_max_request_payload_bytes == 2048,
                "hello_ack req_payload",
            );
            c.check(
                h.agreed_max_request_batch_items == 50,
                "hello_ack req_batch",
            );
            c.check(
                h.agreed_max_response_payload_bytes == 65536,
                "hello_ack resp_payload",
            );
            c.check(
                h.agreed_max_response_batch_items == 1,
                "hello_ack resp_batch",
            );
            c.check(h.agreed_packet_size == 32768, "hello_ack pkt_size");
        }
    }

    // 5. Cgroups request
    {
        let data = read_file(dir, "cgroups_req.bin");
        let req = CgroupsRequest::decode(&data);
        c.check(req.is_ok(), "decode cgroups_req");
        if let Ok(r) = req {
            c.check(r.layout_version == 1, "cgroups_req layout_version");
            c.check(r.flags == 0, "cgroups_req flags");
        }
    }

    // 6. Cgroups snapshot response (multi-item)
    {
        let data = read_file(dir, "cgroups_resp.bin");
        let view = CgroupsResponseView::decode(&data);
        c.check(view.is_ok(), "decode cgroups_resp");
        if let Ok(v) = view {
            c.check(v.item_count == 3, "cgroups_resp item_count");
            c.check(v.systemd_enabled == 1, "cgroups_resp systemd_enabled");
            c.check(v.generation == 999, "cgroups_resp generation");

            if let Ok(item) = v.item(0) {
                c.check(item.hash == 100, "item 0 hash");
                c.check(item.options == 0, "item 0 options");
                c.check(item.enabled == 1, "item 0 enabled");
                c.check(item.name.as_bytes() == b"init.scope", "item 0 name");
                c.check(
                    item.path.as_bytes() == b"/sys/fs/cgroup/init.scope",
                    "item 0 path",
                );
            }

            if let Ok(item) = v.item(1) {
                c.check(item.hash == 200, "item 1 hash");
                c.check(item.options == 0x02, "item 1 options");
                c.check(item.enabled == 0, "item 1 enabled");
                c.check(
                    item.name.as_bytes() == b"system.slice/docker-abc.scope",
                    "item 1 name",
                );
            }

            if let Ok(item) = v.item(2) {
                c.check(item.hash == 300, "item 2 hash");
                c.check(item.name.len == 0, "item 2 name empty");
                c.check(item.path.len == 0, "item 2 path empty");
            }
        }
    }

    // 7. Empty cgroups snapshot
    {
        let data = read_file(dir, "cgroups_resp_empty.bin");
        let view = CgroupsResponseView::decode(&data);
        c.check(view.is_ok(), "decode cgroups_resp_empty");
        if let Ok(v) = view {
            c.check(v.item_count == 0, "empty item_count");
            c.check(v.systemd_enabled == 0, "empty systemd_enabled");
            c.check(v.generation == 42, "empty generation");
        }
    }

    // 8. CGROUPS_LOOKUP request variants
    {
        let data = read_file(dir, "cgroups_lookup_req.bin");
        let view = CgroupsLookupRequestView::decode(&data);
        c.check(view.is_ok(), "decode cgroups_lookup_req");
        if let Ok(v) = view {
            c.check(v.item_count == 2, "cgroups_lookup_req item_count");
            c.check(
                v.item(0).unwrap().as_bytes() == b"/sys/fs/cgroup/a",
                "cgroups_lookup_req item0",
            );
        }
    }
    {
        let data = read_file(dir, "cgroups_lookup_req_empty.bin");
        let view = CgroupsLookupRequestView::decode(&data);
        c.check(view.is_ok(), "decode cgroups_lookup_req_empty");
        if let Ok(v) = view {
            c.check(v.item_count == 0, "cgroups_lookup_req_empty count");
        }
    }

    // 9. CGROUPS_LOOKUP response variants
    {
        let data = read_file(dir, "cgroups_lookup_resp_known_with_labels.bin");
        let view = CgroupsLookupResponseView::decode(&data);
        c.check(view.is_ok(), "decode cgroups_lookup known labels");
        if let Ok(v) = view {
            c.check(v.generation == 100, "cgroups_lookup generation");
            let item = v.item(0).unwrap();
            c.check(item.status == CGROUP_LOOKUP_KNOWN, "cgroups_lookup status");
            c.check(
                item.orchestrator == ORCHESTRATOR_K8S,
                "cgroups_lookup orchestrator",
            );
            c.check(item.label_count == 2, "cgroups_lookup label_count");
            c.check(
                item.label(0).unwrap().key.as_bytes() == b"namespace",
                "cgroups_lookup label",
            );
        }
    }
    for file in [
        "cgroups_lookup_resp_known_no_labels.bin",
        "cgroups_lookup_resp_unknown_retry.bin",
        "cgroups_lookup_resp_unknown_permanent.bin",
        "cgroups_lookup_resp_empty.bin",
    ] {
        let data = read_file(dir, file);
        c.check(CgroupsLookupResponseView::decode(&data).is_ok(), file);
    }

    // 10. APPS_LOOKUP request variants
    {
        let data = read_file(dir, "apps_lookup_req.bin");
        let view = AppsLookupRequestView::decode(&data);
        c.check(view.is_ok(), "decode apps_lookup_req");
        if let Ok(v) = view {
            c.check(v.item_count == 3, "apps_lookup_req item_count");
            c.check(v.item(0).unwrap() == 0, "apps_lookup_req pid0");
        }
    }
    {
        let data = read_file(dir, "apps_lookup_req_empty.bin");
        let view = AppsLookupRequestView::decode(&data);
        c.check(view.is_ok(), "decode apps_lookup_req_empty");
        if let Ok(v) = view {
            c.check(v.item_count == 0, "apps_lookup_req_empty count");
        }
    }

    // 11. APPS_LOOKUP response variants
    {
        let data = read_file(dir, "apps_lookup_resp_known_full.bin");
        let view = AppsLookupResponseView::decode(&data);
        c.check(view.is_ok(), "decode apps_lookup known full");
        if let Ok(v) = view {
            let item = v.item(0).unwrap();
            c.check(item.pid == 1234, "apps_lookup pid");
            c.check(item.comm.len == 15, "apps_lookup comm boundary");
            c.check(
                item.cgroup_status == APPS_CGROUP_KNOWN,
                "apps_lookup cgroup status",
            );
            c.check(
                item.label(0).unwrap().value.as_bytes() == b"nginx:latest",
                "apps_lookup label",
            );
        }
    }
    for file in [
        "apps_lookup_resp_known_retry.bin",
        "apps_lookup_resp_known_permanent.bin",
        "apps_lookup_resp_known_host_root.bin",
        "apps_lookup_resp_unknown_pid.bin",
        "apps_lookup_resp_empty.bin",
    ] {
        let data = read_file(dir, file);
        c.check(AppsLookupResponseView::decode(&data).is_ok(), file);
    }

    c.report("Rust decode")
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    if args.len() != 3 {
        eprintln!("Usage: {} <encode|decode> <dir>", args[0]);
        process::exit(1);
    }

    match args[1].as_str() {
        "encode" => do_encode(&args[2]),
        "decode" => {
            if !do_decode(&args[2]) {
                process::exit(1);
            }
        }
        other => {
            eprintln!("Unknown command: {}", other);
            process::exit(1);
        }
    }
}
