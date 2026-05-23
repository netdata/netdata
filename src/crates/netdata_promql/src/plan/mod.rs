// SPDX-License-Identifier: GPL-3.0-or-later
//
// Plan IR module: typed intermediate representation lowered from the
// `promql_parser` AST. The evaluator executes this IR; the parse cache
// stores `Arc<Plan>` keyed by query string.

//! PromQL Plan IR and lowering.
//!
//! This module contains:
//!
//! - [`Plan`] — a typed, executable representation of a PromQL query,
//!   lowered from the `promql-parser` AST. Type errors are caught at
//!   lowering time rather than at evaluation.
//! - [`lower_query`] / [`lower`] — the lowering functions that translate
//!   a PromQL string or AST expression into a [`Plan`].
//!
//! The Plan IR mirrors the AST nodes the evaluator supports. Each node
//! carries a known [`ValueType`] determined at lowering time so the
//! evaluator can dispatch without runtime type checks.

#![allow(dead_code)] // FuncKind and call args are reserved for chunk 4.

mod ir;
mod lower;

#[allow(unused_imports)]
pub use ir::{
    AggrKind, AtMod, BinopKind, Cardinality, FuncKind, FusedSource, Grouping, LabelOpKind,
    MatchKeys, MatchSpec, Plan, ValueType,
};
#[allow(unused_imports)]
pub use lower::{LowerError, lower, lower_query};
