#ifndef ACLK_COMMON_H
#define ACLK_COMMON_H

#include "libnetdata/libnetdata.h"

typedef enum aclk_proxy_type {
    SOCKS5,
    HTTP,
} ACLK_PROXY_TYPE;

#define ACLK_PROXY_PROTO_ADDR_SEPARATOR "://"

#define ACLK_PROXY_PROTO_SOCKS5  "socks5" ACLK_PROXY_PROTO_ADDR_SEPARATOR
#define ACLK_PROXY_PROTO_SOCKS5H "socks5h" ACLK_PROXY_PROTO_ADDR_SEPARATOR

ACLK_PROXY_TYPE aclk_verify_proxy(const char *string);

#endif //ACLK_COMMON_H
