use std::path::Path;
use std::process::Command;

fn configure_protoc() -> Result<(), Box<dyn std::error::Error>> {
    println!("cargo:rerun-if-env-changed=PROTOC");

    if std::env::var_os("PROTOC").is_some() {
        return Ok(());
    }

    match protoc_bin_vendored::protoc_bin_path() {
        Ok(protoc) => {
            // SAFETY: build scripts run in a single process and this process does not
            // concurrently read `PROTOC` while we set it.
            #[allow(unused_unsafe)]
            unsafe {
                std::env::set_var("PROTOC", protoc);
            }
            Ok(())
        }
        Err(vendored_err) => {
            match Command::new("protoc").arg("--version").status() {
                Ok(status) if status.success() => {
                    println!(
                        "cargo:warning=vendored protoc unavailable ({vendored_err}); falling back to protoc from PATH"
                    );
                    Ok(())
                }
                Ok(status) => Err(format!(
                    "vendored protoc unavailable ({vendored_err}) and 'protoc --version' exited with status {status}; set PROTOC or install protoc"
                )
                .into()),
                Err(path_err) => Err(format!(
                    "vendored protoc unavailable ({vendored_err}) and failed to execute 'protoc' from PATH ({path_err}); set PROTOC or install protoc"
                )
                .into()),
            }
        }
    }
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    configure_protoc()?;

    let proto_root = Path::new("proto");
    let proto_files = [
        proto_root.join("net/api/net.proto"),
        proto_root.join("route/api/route.proto"),
        proto_root.join("cmd/ris/api/ris.proto"),
    ];
    for proto in &proto_files {
        println!("cargo:rerun-if-changed={}", proto.display());
    }

    tonic_prost_build::configure().compile_protos(&proto_files, &[proto_root.to_path_buf()])?;

    Ok(())
}
