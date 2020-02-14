// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_KINESIS_H
#define NETDATA_EXPORTING_KINESIS_H

#include "exporting/exporting_engine.h"
#include "aws_kinesis_put_record.h"

#define KINESIS_PARTITION_KEY_MAX 256
#define KINESIS_RECORD_MAX 1024 * 1024

#endif //NETDATA_EXPORTING_KINESIS_H
