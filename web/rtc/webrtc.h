// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEBRTC_H
#define NETDATA_WEBRTC_H

#include "../../libnetdata/libnetdata.h"

# ifdef __cplusplus
extern "C" {
# endif

int webrtc_new_connection(const char *sdp, BUFFER *wb, char **candidates, size_t *candidates_max);
void webrtc_close_all_connections();
void webrtc_initialize();

# ifdef __cplusplus
}
# endif

#endif //NETDATA_WEBRTC_H
