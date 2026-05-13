// SPDX-License-Identifier: GPL-3.0-or-later
//
// Evaluator: walks the Plan IR against an EvalContext, calling into the
// storage adapter for selectors and combining results with the operator
// and aggregation layers.

#![allow(dead_code)] // EvalError::Other and a couple of builder helpers
                    // are reserved for chunks 4/5.

mod aggregation;
mod binop;
mod context;
mod dispatch;
mod functions;
mod lookback;
mod select;
mod types;
mod unary;

pub use context::EvalContext;
pub use dispatch::eval;
#[allow(unused_imports)]
pub use types::{labels_signature, EvalError, EvalResult, Sample, Series};
