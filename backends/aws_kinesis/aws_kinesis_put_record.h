// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BACKEND_KINESIS_PUT_RECORD_H
#define NETDATA_BACKEND_KINESIS_PUT_RECORD_H

#define ERROR_LINE_MAX 1023

#ifdef __cplusplus
extern "C" {
#endif

void kinesis_init(const char *region, const char *auth_key_id, const char *secure_key, const long timeout);

void kinesis_shutdown();

int kinesis_put_record(const char *stream_name, const char *partition_key,
                       const char *data, size_t data_len);

int kinesis_get_result(char *error_message);

#ifdef __cplusplus
}
#endif

#endif //NETDATA_BACKEND_KINESIS_PUT_RECORD_H
