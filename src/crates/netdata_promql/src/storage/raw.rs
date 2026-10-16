// SPDX-License-Identifier: GPL-3.0-or-later
//
// Raw bindgen output for the C data-source shim. Generated at build time
// from `src/database/contexts/promql-data-source.h`. Never edited by hand
// and never referenced outside the `storage` module.

#![allow(non_camel_case_types)]
#![allow(non_snake_case)]
#![allow(non_upper_case_globals)]
#![allow(dead_code)]

include!(concat!(env!("OUT_DIR"), "/shim_bindings.rs"));
