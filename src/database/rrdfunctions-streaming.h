// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDFUNCTIONS_STREAMING_H
#define NETDATA_RRDFUNCTIONS_STREAMING_H

#include "rrd.h"

#define RRDFUNCTIONS_STREAMING_HELP "Streaming status for parents and children."

int rrdhost_function_streaming(BUFFER *wb, const char *function);

#endif //NETDATA_RRDFUNCTIONS_STREAMING_H
