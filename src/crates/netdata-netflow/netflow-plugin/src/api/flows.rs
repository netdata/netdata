mod handler;
mod model;
mod params;

pub(crate) use handler::NetflowFlowsHandler;
#[cfg(test)]
pub(crate) use model::{FLOWS_FUNCTION_VERSION, FLOWS_UPDATE_EVERY_SECONDS, FlowsFunctionResponse};
#[cfg(test)]
pub(crate) use params::flows_required_params;
