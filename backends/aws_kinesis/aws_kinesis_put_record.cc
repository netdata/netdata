// SPDX-License-Identifier: GPL-3.0-or-later

#include <aws/core/Aws.h>
#include <aws/core/client/ClientConfiguration.h>
#include <aws/core/auth/AWSCredentials.h>
#include <aws/core/utils/Outcome.h>
#include <aws/kinesis/KinesisClient.h>
#include <aws/kinesis/model/PutRecordRequest.h>
#include "aws_kinesis_put_record.h"

using namespace Aws;

static SDKOptions options;

static Kinesis::KinesisClient *client;

struct request_outcome {
    Kinesis::Model::PutRecordOutcomeCallable future_outcome;
    size_t data_len;
};

static Vector<request_outcome> request_outcomes;

void backends_kinesis_init(const char *region, const char *access_key_id, const char *secret_key, const long timeout) {
    InitAPI(options);

    Client::ClientConfiguration config;

    config.region = region;
    config.requestTimeoutMs = timeout;
    config.connectTimeoutMs = timeout;

    if(access_key_id && *access_key_id && secret_key && *secret_key) {
        client = New<Kinesis::KinesisClient>("client", Auth::AWSCredentials(access_key_id, secret_key), config);
    } else {
        client = New<Kinesis::KinesisClient>("client", config);
    }
}

void backends_kinesis_shutdown() {
    Delete(client);

    ShutdownAPI(options);
}

int backends_kinesis_put_record(const char *stream_name, const char *partition_key,
                       const char *data, size_t data_len) {
    Kinesis::Model::PutRecordRequest request;

    request.SetStreamName(stream_name);
    request.SetPartitionKey(partition_key);
    request.SetData(Utils::ByteBuffer((unsigned char*) data, data_len));

    request_outcomes.push_back({client->PutRecordCallable(request), data_len});

    return 0;
}

int backends_kinesis_get_result(char *error_message, size_t *sent_bytes, size_t *lost_bytes) {
    Kinesis::Model::PutRecordOutcome outcome;
    *sent_bytes = 0;
    *lost_bytes = 0;

    for(auto request_outcome = request_outcomes.begin(); request_outcome != request_outcomes.end(); ) {
        std::future_status status = request_outcome->future_outcome.wait_for(std::chrono::microseconds(100));

        if(status == std::future_status::ready || status == std::future_status::deferred) {
            outcome = request_outcome->future_outcome.get();
            *sent_bytes += request_outcome->data_len;

            if(!outcome.IsSuccess()) {
                *lost_bytes += request_outcome->data_len;
                outcome.GetError().GetMessage().copy(error_message, ERROR_LINE_MAX);
            }

            request_outcomes.erase(request_outcome);
        } else {
            ++request_outcome;
        }
    }

    if(*lost_bytes) {
        return 1;
    }

    return 0;
}