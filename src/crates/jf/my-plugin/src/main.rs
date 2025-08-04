use netdata_bridge::{
    netdata::{PingRequest, PongResponse, WorkRequest, WorkResponse},
    NetdataPlugin,
};
use tokio::time::{sleep, Duration};
use tonic::{Request, Response, Status};

#[derive(Default)]
pub struct MyPlugin;

#[tonic::async_trait]
impl NetdataPlugin for MyPlugin {
    async fn ping(&self, _request: Request<PingRequest>) -> Result<Response<PongResponse>, Status> {
        Ok(Response::new(PongResponse {
            message: "Hello from my plugin!".to_string(),
        }))
    }

    async fn do_work(
        &self,
        request: Request<WorkRequest>,
    ) -> Result<Response<WorkResponse>, Status> {
        let req = request.into_inner();

        // Simulate some work
        println!(
            "Starting job: {} (duration: {}ms)",
            req.job_id, req.duration_ms
        );

        // Sleep for the requested duration
        sleep(Duration::from_millis(req.duration_ms)).await;

        let result = format!("Completed processing for job: {}", req.job_id);
        println!("Finished job: {}", req.job_id);

        Ok(Response::new(WorkResponse {
            job_id: req.job_id,
            result,
        }))
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let plugin = MyPlugin::default();
    netdata_bridge::run_plugin(plugin).await
}
