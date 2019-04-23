// SPDX-License-Identifier: GPL-3.0-or-later

#include <aws/core/Aws.h>
#include <aws/core/client/ClientConfiguration.h>
#include <aws/core/auth/AWSCredentials.h>
#include <aws/core/utils/Outcome.h>
#include <aws/kinesis/KinesisClient.h>
#include <aws/kinesis/model/PutRecordRequest.h>
#include "aws_kinesis_put_record.h"

using namespace Aws;

int put_record(char *region, char *auth_key_id, char *secure_key, char *stream_name, char *partition_key, char *data, size_t data_len) {
    SDKOptions options;

    InitAPI(options);

    {
        Client::ClientConfiguration config;
        config.region = region; // "us-east-1"

        Kinesis::KinesisClient client(Auth::AWSCredentials(auth_key_id, secure_key), config);

        Kinesis::Model::PutRecordRequest request;

        request.SetStreamName(stream_name);
        request.SetPartitionKey(partition_key);

        // Aws::String data("Event #1");
        request.SetData(Utils::ByteBuffer((unsigned char*) data, data_len));

        Kinesis::Model::PutRecordOutcome outcome = client.PutRecord(request);

        // if(!outcome.IsSuccess()) {
        //     if(outcome.GetError().GetErrorType() == DynamoDBErrors::RESOURCE_IN_USE) {

        //     }
        // }
    }

    ShutdownAPI(options);
    return 0;
}