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
