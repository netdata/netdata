// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEBRTC_H
#define NETDATA_WEBRTC_H

#include "../../libnetdata/libnetdata.h"
#include "../server/web_client.h"

#define DESCRIPTION_ENTRIES_MAX 100
#define CANDIDATES_ENTRIES_MAX 100
#define DATACHANNEL_ENTRIES_MAX 100

# ifdef __cplusplus
extern "C" {
# endif

int webrtc_answer_to_offer(const char *sdp, BUFFER *wb, char **candidates, size_t *candidates_max);
void webrtc_initialize();

# ifdef __cplusplus
}
# endif

#endif //NETDATA_WEBRTC_H
