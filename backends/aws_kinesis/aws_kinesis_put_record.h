// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BACKEND_KINESIS_PUT_RECORD_H
#define NETDATA_BACKEND_KINESIS_PUT_RECORD_H

#define ERROR_LINE_MAX 1023

#ifdef __cplusplus
typedef Aws::SDKOptions kinesis_options;
typedef Aws::Kinesis::KinesisClient kinesis_client;
#else
typedef struct kinesis_options kinesis_options;
typedef struct kinesis_client kinesis_client;
#endif

#ifdef __cplusplus
extern "C" {
#endif

void kinesis_init(kinesis_options **options, kinesis_client **client,
                  const char *region, const char *auth_key_id, const char *secure_key);

void kinesis_shutdown(kinesis_options *options, kinesis_client *client);

int kinesis_put_record(const kinesis_client *client,
                       const char *stream_name, const char *partition_key,
                       const char *data, size_t data_len, char *error_message);

#ifdef __cplusplus
}
#endif

#endif //NETDATA_BACKEND_KINESIS_PUT_RECORD_H
