// SPDX-License-Identifier: GPL-3.0-or-later
//
// Plan IR module: typed intermediate representation lowered from the
// `promql_parser` AST. The evaluator executes this IR; the parse cache
// stores `Arc<Plan>` keyed by query string.

#![allow(dead_code)] // FuncKind and call args are reserved for chunk 4.

mod ir;
mod lower;

#[allow(unused_imports)]
pub use ir::{
    AggrKind, AtMod, BinopKind, Cardinality, FuncKind, Grouping, MatchKeys, MatchSpec, Plan,
    ValueType,
};
#[allow(unused_imports)]
pub use lower::{lower, lower_query, LowerError};
