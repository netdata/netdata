// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_FUNCTION_STREAMING_H
#define NETDATA_FUNCTION_STREAMING_H

#include "database/rrd.h"

#define RRDFUNCTIONS_STREAMING_HELP "Streaming status for parents and children."

int function_streaming(BUFFER *wb, const char *function, BUFFER *payload, const char *source);

#endif //NETDATA_FUNCTION_STREAMING_H
