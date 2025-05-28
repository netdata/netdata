extern crate cbindgen;

use std::env;

fn main() {
    let crate_dir = env::var("CARGO_MANIFEST_DIR").unwrap();

    cbindgen::Builder::new()
        .with_crate(crate_dir)
        .with_language(cbindgen::Language::C)
        .with_cpp_compat(true)
        .with_include_guard("JOURNAL_READER_FFI_H")
        .rename_item("SdJournal", "sd_journal")
        .rename_item("SdId128", "sd_id128_t")
        .generate()
        .expect("Unable to generate bindings")
        .write_to_file("journal_reader_ffi.h");

    println!("cargo:rerun-if-changed=cbindgen.toml");
    println!("cargo:rerun-if-changed=src/");
}
