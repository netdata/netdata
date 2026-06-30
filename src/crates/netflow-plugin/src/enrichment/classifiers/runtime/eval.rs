use super::model::{
    ExporterClassification, ExporterInfo, ExporterTarget, InterfaceClassification, InterfaceInfo,
    InterfaceTarget,
};
use super::value::ValueExpr;
use super::*;

mod action;
mod boolean;
mod condition;

pub(crate) use action::ActionExpr;
pub(crate) use condition::ConditionExpr;

#[derive(Debug, Clone)]
pub(crate) enum BoolExpr {
    Term(RuleTerm),
    And(Box<BoolExpr>, Box<BoolExpr>),
    Or(Box<BoolExpr>, Box<BoolExpr>),
    Not(Box<BoolExpr>),
}

#[derive(Debug, Clone)]
pub(crate) enum RuleTerm {
    Condition(ConditionExpr),
    Action(ActionExpr),
}
