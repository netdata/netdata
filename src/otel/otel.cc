// SPDX-License-Identifier: GPL-3.0-or-later

#include "generated_proto/opentelemetry/proto/collector/metrics/v1/metrics_service.grpc.pb.h"
#include "grpcpp/grpcpp.h"

#include <iostream>
#include <memory>

using grpc::Server;
using grpc::Status;

void printClientMetadata(const grpc::ServerContext* context) {
    const auto& client_metadata = context->client_metadata();
    for (const auto& pair : client_metadata) {
        std::cout << "Key: " << pair.first << ", Value: " << pair.second << std::endl;
    }
}

class MetricsServiceImpl final : public opentelemetry::proto::collector::metrics::v1::MetricsService::Service {
public:
    Status Export(
        grpc::ServerContext *Ctx,
        const opentelemetry::proto::collector::metrics::v1::ExportMetricsServiceRequest *Request,
        opentelemetry::proto::collector::metrics::v1::ExportMetricsServiceResponse *Response) override
    {
        (void) Ctx;
        (void) Response;
        
        std::cout << "Received " << Request->resource_metrics_size() << " resource metrics\n";

        #if 0
        for (const auto &RMs : Request->resource_metrics()) {
            std::cout << "Resource:\n";
            for (const auto &Attr : RMs.resource().attributes()) {
                std::cout << "  " << Attr.key() << ": " << Attr.value().string_value() << "\n";
            }

            for (const auto &SMs : RMs.scope_metrics()) {
                for (const auto &metric : SMs.metrics()) {
                    std::cout << "Metric: " << metric.name() << "\n";
                    std::cout << "  Description: " << metric.description() << "\n";
                    std::cout << "  Unit: " << metric.unit() << "\n";
                    // Process different types of metrics (gauge, sum, histogram, etc.)
                    // This is a simplified example, you may want to handle different metric types
                }
            }
        }
        #endif

        return Status::OK;
    }
};

void RunServer()
{
    std::string Address("localhost:21212");
    MetricsServiceImpl MS;

    grpc::ServerBuilder Builder;
    Builder.AddListeningPort(Address, grpc::InsecureServerCredentials());
    Builder.RegisterService(&MS);

    std::unique_ptr<Server> Srv(Builder.BuildAndStart());
    std::cout << "Server listening on " << Address << std::endl;
    Srv->Wait();
}

int main(int argc, char **argv)
{
    (void) argc;
    (void) argv;

    RunServer();
    return 0;
}
