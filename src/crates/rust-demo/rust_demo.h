// Hand-written C header for the rust-demo smoke crate. Kept in sync with
// src/crates/rust-demo/src/lib.rs. The crate exposes only two symbols, so
// using cbindgen would add more build-time dependencies than it would save.

#ifndef ND_RUST_DEMO_H
#define ND_RUST_DEMO_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

int32_t nd_rust_add(int32_t a, int32_t b);
const char *nd_rust_version(void);

#ifdef __cplusplus
}
#endif

#endif /* ND_RUST_DEMO_H */
