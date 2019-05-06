// SPDX-License-Identifier: GPL-3.0-or-later

#include <aws/core/Aws.h>
#include <aws/core/client/ClientConfiguration.h>
#include <aws/core/auth/AWSCredentials.h>
#include <aws/core/utils/Outcome.h>
#include <aws/kinesis/KinesisClient.h>
#include <aws/kinesis/model/PutRecordRequest.h>
#include "aws_kinesis_put_record.h"

using namespace Aws;

SDKOptions options;

Kinesis::KinesisClient *client;

Vector<Kinesis::Model::PutRecordOutcomeCallable> future_outcomes;

void kinesis_init(const char *region, const char *auth_key_id, const char *secure_key, const long timeout) {
    InitAPI(options);

    Client::ClientConfiguration config;

    config.region = region;
    config.requestTimeoutMs = timeout;
    config.connectTimeoutMs = timeout;

    client = New<Kinesis::KinesisClient>("client", Auth::AWSCredentials(auth_key_id, secure_key), config);
}

void kinesis_shutdown() {
    Delete(client);

    ShutdownAPI(options);
}

int kinesis_put_record(const char *stream_name, const char *partition_key,
                       const char *data, size_t data_len) {
    Kinesis::Model::PutRecordRequest request;

    request.SetStreamName(stream_name);
    request.SetPartitionKey(partition_key);
    request.SetData(Utils::ByteBuffer((unsigned char*) data, data_len));

    future_outcomes.push_back(client->PutRecordCallable(request));

    return 0;
}

int kinesis_get_result(char *error_message) {
    Kinesis::Model::PutRecordOutcome outcome;

    for(auto future_outcome = future_outcomes.begin(); future_outcome != future_outcomes.end(); ) {
        std::future_status status = future_outcome->wait_for(std::chrono::microseconds(100));
        if(status == std::future_status::ready || status == std::future_status::deferred) {
            outcome = future_outcome->get();
            future_outcomes.erase(future_outcome);
        } else {
            ++future_outcome;
        }
    }

    if(!outcome.IsSuccess()) {
        outcome.GetError().GetMessage().copy(error_message, ERROR_LINE_MAX);
        return 1;
    }

    return 0;
}