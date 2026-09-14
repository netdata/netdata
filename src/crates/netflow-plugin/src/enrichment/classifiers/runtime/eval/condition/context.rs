use super::{ConditionExpr, compare, text};
use crate::enrichment::{
    ExporterClassification, ExporterInfo, InterfaceClassification, InterfaceInfo, ResolvedValue,
    ValueExpr,
};
use anyhow::Result;

pub(super) struct EvalContext<'a> {
    exporter: Option<&'a ExporterInfo>,
    interface: Option<&'a InterfaceInfo>,
    exporter_classification: Option<&'a ExporterClassification>,
    interface_classification: Option<&'a InterfaceClassification>,
}

impl<'a> EvalContext<'a> {
    fn resolve(&self, expr: &ValueExpr) -> Result<ResolvedValue> {
        expr.resolve(
            self.exporter,
            self.interface,
            self.exporter_classification,
            self.interface_classification,
        )
    }

    pub(super) fn resolve_binary(
        &self,
        left: &ValueExpr,
        right: &ValueExpr,
    ) -> Result<(ResolvedValue, ResolvedValue)> {
        Ok((self.resolve(left)?, self.resolve(right)?))
    }
}

impl ConditionExpr {
    pub(crate) fn eval_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &ExporterClassification,
    ) -> Result<bool> {
        self.eval_with_context(Some(exporter), None, Some(classification), None)
    }

    pub(crate) fn eval_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        exporter_classification: &ExporterClassification,
        classification: &InterfaceClassification,
    ) -> Result<bool> {
        self.eval_with_context(
            Some(exporter),
            Some(interface),
            Some(exporter_classification),
            Some(classification),
        )
    }

    pub(crate) fn eval_with_context(
        &self,
        exporter: Option<&ExporterInfo>,
        interface: Option<&InterfaceInfo>,
        exporter_classification: Option<&ExporterClassification>,
        interface_classification: Option<&InterfaceClassification>,
    ) -> Result<bool> {
        let context = EvalContext {
            exporter,
            interface,
            exporter_classification,
            interface_classification,
        };

        match self {
            ConditionExpr::Literal(value) => Ok(*value),
            ConditionExpr::Equals(left, right) => compare::eval_equals(&context, left, right),
            ConditionExpr::NotEquals(left, right) => {
                compare::eval_not_equals(&context, left, right)
            }
            ConditionExpr::Greater(left, right) => compare::eval_greater(&context, left, right),
            ConditionExpr::GreaterOrEqual(left, right) => {
                compare::eval_greater_or_equal(&context, left, right)
            }
            ConditionExpr::Less(left, right) => compare::eval_less(&context, left, right),
            ConditionExpr::LessOrEqual(left, right) => {
                compare::eval_less_or_equal(&context, left, right)
            }
            ConditionExpr::In(left, right) => compare::eval_in(&context, left, right),
            ConditionExpr::Contains(left, right) => text::eval_contains(&context, left, right),
            ConditionExpr::StartsWith(left, right) => text::eval_starts_with(&context, left, right),
            ConditionExpr::EndsWith(left, right) => text::eval_ends_with(&context, left, right),
            ConditionExpr::Matches(left, right, compiled) => {
                text::eval_matches(&context, left, right, compiled.as_ref())
            }
        }
    }
}
