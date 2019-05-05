// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEB_HEALTH_SVG_H
#define NETDATA_WEB_HEALTH_SVG_H 1

#include "libnetdata/libnetdata.h"
#include "web/server/web_client.h"
#include "health/health.h"

#define HEALTH_CMDAPI_CMD_SILENCEALL "SILENCE ALL"
#define HEALTH_CMDAPI_CMD_DISABLEALL "DISABLE ALL"
#define HEALTH_CMDAPI_CMD_SILENCE "SILENCE"
#define HEALTH_CMDAPI_CMD_DISABLE "DISABLE"
#define HEALTH_CMDAPI_CMD_RESET "RESET"

#define HEALTH_CMDAPI_MSG_AUTHERROR "Auth Error\n"
#define HEALTH_CMDAPI_MSG_SILENCEALL "All alarm notifications are silenced\n"
#define HEALTH_CMDAPI_MSG_DISABLEALL "All health checks are disabled\n"
#define HEALTH_CMDAPI_MSG_RESET "All health checks and notifications are enabled\n"
#define HEALTH_CMDAPI_MSG_DISABLE "Health checks disabled for alarms matching the selectors\n"
#define HEALTH_CMDAPI_MSG_SILENCE "Alarm notifications silenced for alarms matching the selectors\n"
#define HEALTH_CMDAPI_MSG_ADDED "Alarm selector added\n"
#define HEALTH_CMDAPI_MSG_INVALID_KEY "Invalid key. Ignoring it.\n"
#define HEALTH_CMDAPI_MSG_STYPEWARNING "WARNING: Added alarm selector to silence/disable alarms without a SILENCE or DISABLE command.\n"
#define HEALTH_CMDAPI_MSG_NOSELECTORWARNING "WARNING: SILENCE or DISABLE command is ineffective without defining any alarm selectors.\n"

extern int web_client_api_request_v1_mgmt_health(RRDHOST *host, struct web_client *w, char *url);

#include "web/api/web_api_v1.h"

#endif /* NETDATA_WEB_HEALTH_SVG_H */
