// Smoke-test Rust crate linked into the netdata Agent so we can confirm the
// host toolchain (notably MSYS2/UCRT64 on Windows) can build and link Rust.
//
// The functions below are intentionally trivial; the value of this crate is
// that it exists and gets linked, not what it computes.

use std::ffi::{c_char, c_int};

static ND_RUST_DEMO_VERSION: &[u8] = b"rust-demo 0.1.0\0";

/// Add two 32-bit integers using wrapping semantics so the call has
/// deterministic behaviour on overflow.
#[no_mangle]
pub extern "C" fn nd_rust_add(a: c_int, b: c_int) -> c_int {
    a.wrapping_add(b)
}

/// Return a static, NUL-terminated string identifying this crate. The pointer
/// is valid for the lifetime of the process and must not be freed by the
/// caller.
#[no_mangle]
pub extern "C" fn nd_rust_version() -> *const c_char {
    ND_RUST_DEMO_VERSION.as_ptr() as *const c_char
}
