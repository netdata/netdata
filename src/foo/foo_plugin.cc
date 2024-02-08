#include <iostream>
#include <memory>
#include <string>

#include <grpc++/grpc++.h>
#include <grpc++/ext/proto_server_reflection_plugin.h>
#include <grpcpp/grpcpp.h>
#include "src/foo/proto/hello.grpc.pb.h"

using grpc::Server;
using grpc::ServerBuilder;
using grpc::ServerContext;
using grpc::Status;
using helloworld::Greeter;
using helloworld::HelloReply;
using helloworld::HelloRequest;

// Logic and data behind the server's behavior.
class GreeterServiceImpl final : public Greeter::Service {
    Status SayHello(ServerContext *context, const HelloRequest *request, HelloReply *reply) override
    {
        (void) context;
        
        std::string prefix("Hello ");
        reply->set_message(prefix + request->name());
        return Status::OK;
    }
};

static void RunServer()
{
    std::string server_address("0.0.0.0:50051");
    GreeterServiceImpl service;

    grpc::EnableDefaultHealthCheckService(true);
    grpc::reflection::InitProtoReflectionServerBuilderPlugin();

    ServerBuilder builder;

    builder.AddListeningPort(server_address, grpc::InsecureServerCredentials());
    builder.RegisterService(&service);

    std::unique_ptr<Server> server(builder.BuildAndStart());
    std::cout << "Server listening on " << server_address << std::endl;
    server->Wait();
}

extern "C" void *grpc_main(void *arg)
{
    (void) arg;
        
    RunServer();
    return nullptr;
}
