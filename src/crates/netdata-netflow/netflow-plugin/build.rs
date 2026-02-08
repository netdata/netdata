use std::path::Path;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let protoc = protoc_bin_vendored::protoc_bin_path()?;
    // SAFETY: build scripts run in a single process and this process does not
    // concurrently read `PROTOC` while we set it.
    unsafe {
        std::env::set_var("PROTOC", protoc);
    }

    let proto_root = Path::new("proto");
    let proto_files = [
        proto_root.join("net/api/net.proto"),
        proto_root.join("route/api/route.proto"),
        proto_root.join("cmd/ris/api/ris.proto"),
    ];
    for proto in &proto_files {
        println!("cargo:rerun-if-changed={}", proto.display());
    }

    tonic_prost_build::configure().compile_protos(
        &proto_files
            .iter()
            .map(|path| path.as_path())
            .collect::<Vec<_>>(),
        &[proto_root],
    )?;

    Ok(())
}
