// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEBRTC_H
#define NETDATA_WEBRTC_H

#include "libnetdata/libnetdata.h"

int webrtc_new_connection(const char *sdp, BUFFER *wb);
void webrtc_close_all_connections();
void webrtc_initialize();

#endif //NETDATA_WEBRTC_H
