mod flows;

pub(crate) use flows::NetflowFlowsHandler;
#[cfg(test)]
pub(crate) use flows::{
    FLOWS_FUNCTION_VERSION, FLOWS_UPDATE_EVERY_SECONDS, FlowsFunctionResponse,
    flows_required_params,
};
