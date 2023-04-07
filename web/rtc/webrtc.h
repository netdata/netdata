// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WEBRTC_H
#define NETDATA_WEBRTC_H

#include "../../libnetdata/libnetdata.h"

#define DESCRIPTION_ENTRIES_MAX 100
#define CANDIDATES_ENTRIES_MAX 100
#define DATACHANNEL_ENTRIES_MAX 100

# ifdef __cplusplus
extern "C" {
# endif

struct webrtc_answer {
    size_t description_id;
    char *description[DESCRIPTION_ENTRIES_MAX];

    size_t candidates_id;
    char *candidates[CANDIDATES_ENTRIES_MAX];
};

void *webrtc_answer_to_offer(const char *sdp);
struct webrtc_answer *webrtc_get_answer(void *webrtc_conn);
void webrtc_close(void *webrtc_conn);
void webrtc_initialize();

# ifdef __cplusplus
}
# endif

#endif //NETDATA_WEBRTC_H
