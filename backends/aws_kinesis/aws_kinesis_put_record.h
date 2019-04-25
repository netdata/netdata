// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BACKEND_KINESIS_PUT_RECORD_H
#define NETDATA_BACKEND_KINESIS_PUT_RECORD_H

#ifdef __cplusplus
extern "C" {
#endif

int put_record(const char *region, const char *auth_key_id, const char *secure_key,
               const char *stream_name, const char *partition_key,
               const char *data, size_t data_len);

#ifdef __cplusplus
}
#endif

#endif //NETDATA_BACKEND_KINESIS_PUT_RECORD_H
