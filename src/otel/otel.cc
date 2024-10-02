// SPDX-License-Identifier: GPL-3.0-or-later

#include "otel_sort.h"
#include "otel_config.h"
#include "otel_transform.h"

#include "libnetdata/required_dummies.h"

#include "CLI/CLI.hpp"
#include "fmt/core.h"
#include "opentelemetry/proto/collector/metrics/v1/metrics_service.grpc.pb.h"
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
    MetricsServiceImpl(otel::Config *Cfg) : Cfg(Cfg) {}

    Status Export(
        grpc::ServerContext *Ctx,
        const opentelemetry::proto::collector::metrics::v1::ExportMetricsServiceRequest *Request,
        opentelemetry::proto::collector::metrics::v1::ExportMetricsServiceResponse *Response) override
    {
        (void) Ctx;
        (void) Response;

        auto *RMs = pb::Arena::Create<pb::RepeatedPtrField<pb::ResourceMetrics>>(&Arena, Request->resource_metrics());
        RMs->CopyFrom(Request->resource_metrics());

        pb::transformResourceMetrics(Cfg, *RMs);
        pb::sortResourceMetrics(RMs);

        std::cout << "Received " << Request->resource_metrics_size() << " resource metrics";
        std::cout << " (" << Request->ByteSizeLong() / 1024 << " KiB)\n";

        Arena.Reset();

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

private:
    otel::Config *Cfg;
    pb::Arena Arena;
};

void RunServer(otel::Config *Cfg)
{
    std::string Address("localhost:21212");
    MetricsServiceImpl MS(Cfg);

    grpc::ServerBuilder Builder;
    Builder.AddListeningPort(Address, grpc::InsecureServerCredentials());
    Builder.RegisterService(&MS);

    std::unique_ptr<Server> Srv(Builder.BuildAndStart());
    std::cout << "Server listening on " << Address << std::endl;
    Srv->Wait();
}

int main(int argc, char **argv)
{
    CLI::App App{"OTEL plugin"};

    std::string Path;
    App.add_option("--config", Path, "Path to the receivers configuration file");

    CLI11_PARSE(App, argc, argv);

    absl::StatusOr<otel::Config *> Cfg = otel::Config::load(Path);
    if (!Cfg.ok()) {
        fmt::print(stderr, "{}\n", Cfg.status().ToString());
        return 1;
    }

    RunServer(*Cfg);
    return 0;
}
