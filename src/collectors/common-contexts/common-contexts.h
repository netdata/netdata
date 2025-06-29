// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COMMON_CONTEXTS_H
#define NETDATA_COMMON_CONTEXTS_H

#include "../../libnetdata/libnetdata.h"
#include "../../database/rrd.h"

#ifndef _COMMON_PLUGIN_NAME
#error You need to set _COMMON_PLUGIN_NAME before including common-contexts.h
#endif

#ifndef _COMMON_PLUGIN_MODULE_NAME
#error You need to set _COMMON_PLUGIN_MODULE_NAME before including common-contexts.h
#endif

#define _COMMON_CONFIG_SECTION "plugin:" _COMMON_PLUGIN_NAME ":" _COMMON_PLUGIN_MODULE_NAME

typedef void (*instance_labels_cb_t)(RRDSET *st, void *data);

#include "system-io.h"
#include "system-ram.h"
#include "system-interrupts.h"
#include "system-processes.h"
#include "system-ipc.h"
#include "mem-swap.h"
#include "mem-pgfaults.h"
#include "mem-available.h"
#include "disk-io.h"
#include "disk-ops.h"
#include "disk-qops.h"
#include "disk-util.h"
#include "disk-busy.h"
#include "disk-iotime.h"
#include "disk-await.h"
#include "disk-svctm.h"
#include "disk-avgsz.h"
#include "power-supply.h"

#endif //NETDATA_COMMON_CONTEXTS_H
