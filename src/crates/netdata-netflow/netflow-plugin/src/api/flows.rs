mod handler;
mod model;
mod params;

pub(crate) use handler::NetflowFlowsHandler;
#[allow(unused_imports)]
pub(crate) use model::{FLOWS_FUNCTION_VERSION, FLOWS_UPDATE_EVERY_SECONDS, FlowsFunctionResponse};
#[allow(unused_imports)]
pub(crate) use params::flows_required_params;
