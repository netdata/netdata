// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_FUNCTION_STREAMING_H
#define NETDATA_FUNCTION_STREAMING_H

#include "daemon/common.h"

#define RRDFUNCTIONS_STREAMING_HELP "Streaming status for parents and children."

int function_streaming(BUFFER *wb, const char *function);

#endif //NETDATA_FUNCTION_STREAMING_H
