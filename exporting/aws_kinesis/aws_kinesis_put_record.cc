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

struct request_outcome {
    Kinesis::Model::PutRecordOutcomeCallable future_outcome;
    size_t data_len;
};

void aws_sdk_init()
{
    InitAPI(options);
}

void aws_sdk_shutdown()
{
    ShutdownAPI(options);
}

void kinesis_init(
    void *kinesis_specific_data_p, const char *region, const char *access_key_id, const char *secret_key,
    const long timeout)
{
    struct aws_kinesis_specific_data *kinesis_specific_data =
        (struct aws_kinesis_specific_data *)kinesis_specific_data_p;

    Client::ClientConfiguration config;

    config.region = region;
    config.requestTimeoutMs = timeout;
    config.connectTimeoutMs = timeout;

    Kinesis::KinesisClient *client;

    if (access_key_id && *access_key_id && secret_key && *secret_key) {
        client = New<Kinesis::KinesisClient>("client", Auth::AWSCredentials(access_key_id, secret_key), config);
    } else {
        client = New<Kinesis::KinesisClient>("client", config);
    }
    kinesis_specific_data->client = (void *)client;

    Vector<request_outcome> *request_outcomes;

    request_outcomes = new Vector<request_outcome>;
    kinesis_specific_data->request_outcomes = (void *)request_outcomes;
}

void kinesis_shutdown(void *client)
{
    Delete((Kinesis::KinesisClient *)client);
}

void kinesis_put_record(
    void *kinesis_specific_data_p, const char *stream_name, const char *partition_key, const char *data,
    size_t data_len)
{
    struct aws_kinesis_specific_data *kinesis_specific_data =
        (struct aws_kinesis_specific_data *)kinesis_specific_data_p;
    Kinesis::Model::PutRecordRequest request;

    request.SetStreamName(stream_name);
    request.SetPartitionKey(partition_key);
    request.SetData(Utils::ByteBuffer((unsigned char *)data, data_len));

    ((Vector<request_outcome> *)(kinesis_specific_data->request_outcomes))->push_back(
        { ((Kinesis::KinesisClient *)(kinesis_specific_data->client))->PutRecordCallable(request), data_len });
}

int kinesis_get_result(void *request_outcomes_p, char *error_message, size_t *sent_bytes, size_t *lost_bytes)
{
    Vector<request_outcome> *request_outcomes = (Vector<request_outcome> *)request_outcomes_p;
    Kinesis::Model::PutRecordOutcome outcome;
    *sent_bytes = 0;
    *lost_bytes = 0;

    for (auto request_outcome = request_outcomes->begin(); request_outcome != request_outcomes->end();) {
        std::future_status status = request_outcome->future_outcome.wait_for(std::chrono::microseconds(100));

        if (status == std::future_status::ready || status == std::future_status::deferred) {
            outcome = request_outcome->future_outcome.get();
            *sent_bytes += request_outcome->data_len;

            if (!outcome.IsSuccess()) {
                *lost_bytes += request_outcome->data_len;
                outcome.GetError().GetMessage().copy(error_message, ERROR_LINE_MAX);
            }

            request_outcomes->erase(request_outcome);
        } else {
            ++request_outcome;
        }
    }

    if (*lost_bytes) {
        return 1;
    }

    return 0;
}