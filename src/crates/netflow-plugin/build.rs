use std::env;

fn main() {
    emit_path("NETDATA_BUILD_CACHE_DIR", "/var/cache/netdata");
    emit_path("NETDATA_BUILD_LIB_DIR", "/var/lib/netdata");
    emit_path("NETDATA_BUILD_STOCK_DATA_DIR", "/usr/share/netdata");
}

fn emit_path(name: &str, fallback: &str) {
    println!("cargo:rerun-if-env-changed={name}");
    let value = env::var(name)
        .ok()
        .filter(|value| !value.trim().is_empty())
        .unwrap_or_else(|| fallback.to_string());
    println!("cargo:rustc-env={name}={value}");
}
