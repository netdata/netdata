// SPDX-License-Identifier: GPL-3.0-or-later
//
// Evaluator: walks the Plan IR against an EvalContext, calling into the
// storage adapter for selectors and combining results with the operator
// and aggregation layers.

//! PromQL expression evaluator.
//!
//! The evaluator walks a [`Plan`](crate::plan::Plan) IR tree against an
//! [`EvalContext`], dispatching each node type to a dedicated submodule:
//!
//! - [`select`] — vector and matrix selector evaluation
//! - [`binop`] — binary operators (arithmetic, comparison, set)
//! - [`unary`] — unary negation
//! - [`aggregation`] — aggregation operators (sum, avg, topk, etc.)
//! - [`functions`] — built-in functions (rate, histogram_quantile, etc.)
//! - [`labelops`] — label_replace / label_join
//! - [`absent`] — absent / absent_over_time
//! - [`subquery`] — subquery evaluation
//! - [`fused`] — fused aggregation + rollup streaming path
//!
//! Results are column-oriented [`Series`] values grouped in an [`EvalResult`].

#![allow(dead_code)] // EvalError::Other and a couple of builder helpers
// are reserved for chunks 4/5.

mod absent;
mod aggregation;
mod binop;
mod context;
mod dispatch;
mod functions;
mod fused;
mod grid;
mod labelops;
mod select;
mod subquery;
mod types;
mod unary;

pub use context::EvalContext;
pub use dispatch::eval;
pub use grid::Grid;
#[allow(unused_imports)]
pub use types::{EvalError, EvalResult, Sample, Series, labels_signature};
