// SPDX-License-Identifier: GPL-3.0-or-later

#include "google/protobuf/arena.h"
#include "netdata.h"

#include "otel_iterator.h"
#include "otel_sort.h"
#include "otel_config.h"
#include "otel_transform.h"
#include "otel_process.h"

#include "libnetdata/required_dummies.h"

#include "CLI/CLI.hpp"
#include "fmt/core.h"
#include "opentelemetry/proto/collector/metrics/v1/metrics_service.grpc.pb.h"
#include "grpcpp/grpcpp.h"

#include <iostream>
#include <memory>

using grpc::Server;
using grpc::Status;

static google::protobuf::ArenaOptions ArenaOpts = {
    .start_block_size = 16 * 1024 * 1024,
    .max_block_size = 512 * 1024 * 1024,
};

static void printClientMetadata(const grpc::ServerContext *context)
{
    const auto &client_metadata = context->client_metadata();
    for (const auto &pair : client_metadata) {
        std::cout << "Key: " << pair.first << ", Value: " << pair.second << std::endl;
    }
}

class MetricsServiceImpl final : public opentelemetry::proto::collector::metrics::v1::MetricsService::Service {
    using ExportMetricsServiceRequest = opentelemetry::proto::collector::metrics::v1::ExportMetricsServiceRequest;
    using ExportMetricsServiceResponse = opentelemetry::proto::collector::metrics::v1::ExportMetricsServiceResponse;

public:
    MetricsServiceImpl(otel::Config *Cfg) : Cfg(Cfg), ProcCtx(Cfg), Arena(ArenaOpts)
    {
    }

    Status Export(
        grpc::ServerContext *Ctx,
        const ExportMetricsServiceRequest *Request,
        ExportMetricsServiceResponse *Response) override
    {
        (void)Ctx;
        (void)Response;

        auto *RPF = pb::Arena::Create<pb::RepeatedPtrField<pb::ResourceMetrics> >(&Arena, Request->resource_metrics());
        RPF->CopyFrom(Request->resource_metrics());

        pb::transformResourceMetrics(Cfg, *RPF);
        pb::sortResourceMetrics(RPF);

        otel::MetricsDataProcessor MDP(ProcCtx);
        for (const otel::Element &E : otel::Data(*RPF, MDP)) {
            /* ... */
        }

        fmt::println(
            "Received {} resource metrics ({} KiB)", Request->resource_metrics_size(), Request->ByteSizeLong() / 1024);

        fmt::println(
            "{:.4} / {:.4} MiB",
            static_cast<double>(Arena.SpaceUsed()) / (1024 * 1024),
            static_cast<double>(Arena.SpaceAllocated()) / (1024 * 1024));

        fmt::println("");

        Arena.Reset();

        return Status::OK;
    }

private:
    otel::Config *Cfg;
    otel::ProcessorContext ProcCtx;
    pb::Arena Arena;
};

static void RunServer(otel::Config *Cfg)
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
