// SPDX-License-Identifier: GPL-3.0-or-later

#include "google/protobuf/arena.h"

#include "fmt_utils.h"
#include "otel_utils.h"
#include "otel_config.h"
#include "otel_iterator.h"

#include "libnetdata/required_dummies.h"

#include "CLI/CLI.hpp"
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
    MetricsServiceImpl(otel::Config *Cfg) : Cfg(Cfg), Arena(ArenaOpts)
    {
    }

    Status Export(
        grpc::ServerContext *Ctx,
        const ExportMetricsServiceRequest *Request,
        ExportMetricsServiceResponse *Response) override
    {
        (void)Ctx;
        (void)Response;

        fmt::println(
            "Received {} resource metrics ({} KiB)", Request->resource_metrics_size(), Request->ByteSizeLong() / 1024);

        OtelData OD(&Request->resource_metrics());

        for (const OtelElement &OE : OD) {
            std::cout << "Name: " << OE.M->name() << "\n";
        }

        fmt::println("");
        return Status::OK;
    }

private:
    otel::Config *Cfg;
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

#if 0
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
#else
int main()
{
    // String value
    pb::AnyValue string_value;
    string_value.set_string_value("Hello, OpenTelemetry!");
    std::cout << fmt::format("String value: {}\n", string_value);

    // Boolean value
    pb::AnyValue bool_value;
    bool_value.set_bool_value(true);
    std::cout << fmt::format("Boolean value: {}\n", bool_value);

    // Integer value
    pb::AnyValue int_value;
    int_value.set_int_value(42);
    std::cout << fmt::format("Integer value: {}\n", int_value);

    // Double value
    pb::AnyValue double_value;
    double_value.set_double_value(3.14159);
    std::cout << fmt::format("Double value: {}\n", double_value);

    // Array value
    pb::AnyValue array_value;
    auto *array = array_value.mutable_array_value();
    array->add_values()->set_string_value("one");
    array->add_values()->set_int_value(2);
    array->add_values()->set_bool_value(true);
    std::cout << fmt::format("Array value: {}\n", array_value);

    // KVList value (commented out in your formatter)
    pb::AnyValue kvlist_value;
    auto *kvlist = kvlist_value.mutable_kvlist_value();

    auto *kv1 = kvlist->add_values();
    kv1->set_key("key1");
    kv1->mutable_value()->set_string_value("value1");

    auto *kv2 = kvlist->add_values();
    kv2->set_key("key2");
    kv2->mutable_value()->set_int_value(42);

    auto *kv3 = kvlist->add_values();
    kv3->set_key("key3");
    auto *nested_kvlist = kv3->mutable_value()->mutable_kvlist_value();

    // Adding values to the nested KeyValueList
    auto *nested_kv1 = nested_kvlist->add_values();
    nested_kv1->set_key("nested_key1");
    nested_kv1->mutable_value()->set_string_value("nested_value1");

    auto *nested_kv2 = nested_kvlist->add_values();
    nested_kv2->set_key("nested_key2");
    nested_kv2->mutable_value()->set_double_value(3.14);

    std::cout << fmt::format("KVList value: {}\n", kvlist_value);

    // Bytes value
    pb::AnyValue bytes_value;
    bytes_value.set_bytes_value("\x00\x01\x02\x03\x04");
    std::cout << fmt::format("Bytes value: {}\n", bytes_value);

    // Unknown value
    pb::AnyValue unknown_value;
    std::cout << fmt::format("Unknown value: {}\n", unknown_value);

    return 0;
}
#endif
