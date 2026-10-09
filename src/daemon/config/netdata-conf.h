// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DAEMON_NETDATA_CONF_H
#define NETDATA_DAEMON_NETDATA_CONF_H

#include "libnetdata/libnetdata.h"

extern struct config netdata_config;
bool netdata_conf_load(char *filename, char overwrite_used, const char **user);

#include "netdata-conf-backwards-compatibility.h"
#include "netdata-conf-db.h"
#include "netdata-conf-directories.h"
#include "netdata-conf-global.h"
#include "netdata-conf-profile.h"
#include "netdata-conf-logs.h"
#include "netdata-conf-web.h"
#include "netdata-conf-cloud.h"
#include "netdata-conf-ssl.h"

#endif //NETDATA_DAEMON_NETDATA_CONF_H
