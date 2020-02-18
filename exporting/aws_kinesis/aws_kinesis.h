// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_KINESIS_H
#define NETDATA_EXPORTING_KINESIS_H

#include "exporting/exporting_engine.h"
#include "exporting/json/json.h"
#include "aws_kinesis_put_record.h"

#define KINESIS_PARTITION_KEY_MAX 256
#define KINESIS_RECORD_MAX 1024 * 1024

int init_aws_kinesis_instance(struct instance *instance);
void aws_kinesis_connector_worker(void *instance_p);

#endif //NETDATA_EXPORTING_KINESIS_H
