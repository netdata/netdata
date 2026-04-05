// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETIPC_NETDATA_H
#define NETDATA_NETIPC_NETDATA_H 1

#include "libnetdata/libnetdata.h"

#if defined(OS_LINUX) || defined(OS_WINDOWS)
#include "netipc/netipc_service.h"
#include "netipc/netipc_protocol.h"

// derive a netipc auth token from the current NETDATA_INVOCATION_ID
uint64_t netipc_auth_token(void);

#endif // OS_LINUX || OS_WINDOWS

#endif // NETDATA_NETIPC_NETDATA_H
