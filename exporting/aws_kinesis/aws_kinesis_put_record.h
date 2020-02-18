// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_KINESIS_PUT_RECORD_H
#define NETDATA_EXPORTING_KINESIS_PUT_RECORD_H

#define ERROR_LINE_MAX 1023

#ifdef __cplusplus
extern "C" {
#endif

void aws_sdk_init();
void aws_sdk_shutdown();

void kinesis_init(const char *region, const char *access_key_id, const char *secret_key, const long timeout);
void kinesis_shutdown();

int kinesis_put_record(const char *stream_name, const char *partition_key,
                       const char *data, size_t data_len);

int kinesis_get_result(char *error_message, size_t *sent_bytes, size_t *lost_bytes);

#ifdef __cplusplus
}
#endif

#endif //NETDATA_EXPORTING_KINESIS_PUT_RECORD_H
