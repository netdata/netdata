// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_BACKEND_KINESIS_H
#define NETDATA_BACKEND_KINESIS_H

#include "backends/backends.h"
#include "aws_kinesis_put_record.h"

#define KINESIS_PARTITION_KEY_MAX 256
#define KINESIS_RECORD_MAX 1024 * 1024

extern int read_kinesis_conf(const char *path, char **auth_key_id_p, char **secure_key_p, char **stream_name_p);

#endif //NETDATA_BACKEND_KINESIS_H
