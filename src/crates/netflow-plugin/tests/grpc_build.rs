//! Regeneration check for vendored protobuf files.
//!
//! This test compiles the .proto definitions under `proto/` and compares the
//! output against the committed files in `src/routing/proto/`.
//!
//! - Locally: if there is a diff, the test **overwrites** the committed files
//!   and fails with instructions to re-run and commit.
//! - In CI (`CI` env var set): the test fails without overwriting.
//!
//! Run with: `cargo test -p netflow-plugin --test grpc_build`

use std::path::Path;

fn configure_protoc() {
    if std::env::var_os("PROTOC").is_some() {
        return;
    }
    let protoc = protoc_bin_vendored::protoc_bin_path()
        .expect("vendored protoc not available and PROTOC not set");
    unsafe {
        std::env::set_var("PROTOC", protoc);
    }
}

#[test]
fn vendored_proto_files_are_up_to_date() {
    configure_protoc();

    let manifest_dir = Path::new(env!("CARGO_MANIFEST_DIR"));
    let proto_root = manifest_dir.join("proto");
    let vendored_dir = manifest_dir.join("src/routing/proto");

    let proto_files = [
        proto_root.join("net/api/net.proto"),
        proto_root.join("route/api/route.proto"),
        proto_root.join("cmd/ris/api/ris.proto"),
    ];

    let tmp_dir = tempfile::tempdir().expect("create temp dir");

    tonic_prost_build::configure()
        .out_dir(tmp_dir.path())
        .compile_protos(&proto_files, &[proto_root.clone()])
        .expect("protobuf compilation failed");

    let generated_files = ["bio.net.rs", "bio.route.rs", "bio.ris.rs"];
    let mut stale = Vec::new();

    for name in &generated_files {
        let fresh = std::fs::read_to_string(tmp_dir.path().join(name))
            .unwrap_or_else(|e| panic!("read generated {name}: {e}"));
        let committed_path = vendored_dir.join(name);
        let committed = std::fs::read_to_string(&committed_path).unwrap_or_default();

        if fresh != committed {
            stale.push(*name);

            if std::env::var_os("CI").is_none() {
                std::fs::write(&committed_path, &fresh)
                    .unwrap_or_else(|e| panic!("write {}: {e}", committed_path.display()));
            }
        }
    }

    if !stale.is_empty() {
        let files = stale.join(", ");
        if std::env::var_os("CI").is_some() {
            panic!(
                "vendored proto files are stale: {files}\n\
                 Run `cargo test -p netflow-plugin --test grpc_build` locally and commit the updated files."
            );
        } else {
            panic!(
                "vendored proto files were stale and have been updated: {files}\n\
                 Please re-run tests and commit the changes."
            );
        }
    }
}
