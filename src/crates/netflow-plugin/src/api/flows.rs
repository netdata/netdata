mod handler;
mod model;
mod params;

pub(crate) use handler::NetflowFlowsHandler;
pub(crate) use model::{FLOWS_FUNCTION_NAME, FlowsFunctionResponse};
#[cfg(test)]
pub(crate) use model::{FLOWS_FUNCTION_VERSION, FLOWS_UPDATE_EVERY_SECONDS};
#[cfg(test)]
pub(crate) use params::flows_required_params;
