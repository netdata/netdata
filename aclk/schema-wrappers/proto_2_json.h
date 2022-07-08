// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef PROTO_2_JSON_H
#define PROTO_2_JSON_H

#include <sys/types.h>

#ifdef __cplusplus
extern "C" {
#endif

char *protomsg_to_json(const void *protobin, size_t len, const char *msgname);

#ifdef __cplusplus
}
#endif

#endif /* PROTO_2_JSON_H */
