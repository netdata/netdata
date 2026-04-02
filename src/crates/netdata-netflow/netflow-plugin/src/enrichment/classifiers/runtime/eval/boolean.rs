use super::*;

impl BoolExpr {
    pub(crate) fn eval_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &mut ExporterClassification,
    ) -> Result<bool> {
        match self {
            BoolExpr::Term(term) => term.eval_exporter(exporter, classification),
            BoolExpr::And(left, right) => {
                if !left.eval_exporter(exporter, classification)? {
                    return Ok(false);
                }
                right.eval_exporter(exporter, classification)
            }
            BoolExpr::Or(left, right) => {
                if left.eval_exporter(exporter, classification)? {
                    return Ok(true);
                }
                right.eval_exporter(exporter, classification)
            }
            BoolExpr::Not(inner) => Ok(!inner.eval_exporter(exporter, classification)?),
        }
    }

    pub(crate) fn eval_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        exporter_classification: &ExporterClassification,
        classification: &mut InterfaceClassification,
    ) -> Result<bool> {
        match self {
            BoolExpr::Term(term) => {
                term.eval_interface(exporter, interface, exporter_classification, classification)
            }
            BoolExpr::And(left, right) => {
                if !left.eval_interface(
                    exporter,
                    interface,
                    exporter_classification,
                    classification,
                )? {
                    return Ok(false);
                }
                right.eval_interface(exporter, interface, exporter_classification, classification)
            }
            BoolExpr::Or(left, right) => {
                if left.eval_interface(
                    exporter,
                    interface,
                    exporter_classification,
                    classification,
                )? {
                    return Ok(true);
                }
                right.eval_interface(exporter, interface, exporter_classification, classification)
            }
            BoolExpr::Not(inner) => Ok(!inner.eval_interface(
                exporter,
                interface,
                exporter_classification,
                classification,
            )?),
        }
    }
}

impl RuleTerm {
    pub(crate) fn eval_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &mut ExporterClassification,
    ) -> Result<bool> {
        match self {
            RuleTerm::Condition(condition) => condition.eval_exporter(exporter, classification),
            RuleTerm::Action(action) => action.eval_exporter(exporter, classification),
        }
    }

    pub(crate) fn eval_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        exporter_classification: &ExporterClassification,
        classification: &mut InterfaceClassification,
    ) -> Result<bool> {
        match self {
            RuleTerm::Condition(condition) => condition.eval_interface(
                exporter,
                interface,
                exporter_classification,
                classification,
            ),
            RuleTerm::Action(action) => {
                action.eval_interface(exporter, interface, exporter_classification, classification)
            }
        }
    }
}
