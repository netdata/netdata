// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BACKEND_KINESIS_PUT_RECORD_H
#define NETDATA_BACKEND_KINESIS_PUT_RECORD_H

#ifdef __cplusplus
extern "C" {
#endif

int put_record(char *region, char *auth_key_id, char *secure_key, char *stream_name, char *partition_key, char *data, size_t data_len);

#ifdef __cplusplus
}
#endif

#endif //NETDATA_BACKEND_KINESIS_PUT_RECORD_H
