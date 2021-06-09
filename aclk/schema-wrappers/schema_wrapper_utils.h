// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef SCHEMA_WRAPPER_UTILS_H
#define SCHEMA_WRAPPER_UTILS_H

#if GOOGLE_PROTOBUF_VERSION < 3001000
#define PROTO_COMPAT_MSG_SIZE(msg) (size_t)msg.ByteSize();
#else
#define PROTO_COMPAT_MSG_SIZE(msg) msg.ByteSizeLong();
#endif

#endif /* SCHEMA_WRAPPER_UTILS_H */
