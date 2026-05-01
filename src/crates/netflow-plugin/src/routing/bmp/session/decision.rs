use super::super::*;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(in crate::routing::bmp) enum BmpSessionDecision {
    Process,
    CloseMissingInitiation,
    CloseTermination,
}

pub(in crate::routing::bmp) fn bmp_session_decision(
    message: &BmpMessageValue,
    initialized: &mut bool,
) -> BmpSessionDecision {
    if !*initialized {
        if matches!(message, BmpMessageValue::Initiation(_)) {
            *initialized = true;
            return BmpSessionDecision::Process;
        }
        return BmpSessionDecision::CloseMissingInitiation;
    }

    if matches!(message, BmpMessageValue::Termination(_)) {
        return BmpSessionDecision::CloseTermination;
    }

    BmpSessionDecision::Process
}
