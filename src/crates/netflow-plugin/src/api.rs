mod flows;

pub(crate) use flows::NetflowFlowsHandler;
pub(crate) use flows::{FLOWS_FUNCTION_NAME, FlowsFunctionResponse};
#[cfg(test)]
pub(crate) use flows::{FLOWS_FUNCTION_VERSION, FLOWS_UPDATE_EVERY_SECONDS, flows_required_params};
