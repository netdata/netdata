#ifndef ACLK_COMMON_H
#define ACLK_COMMON_H

#include "libnetdata/libnetdata.h"

typedef enum aclk_proxy_type {
    PROXY_TYPE_UNKNOWN = 0,
    PROXY_TYPE_SOCKS5,
    PROXY_TYPE_HTTP,
    PROXY_DISABLED,
    PROXY_NOT_SET,
} ACLK_PROXY_TYPE;

#define ACLK_PROXY_PROTO_ADDR_SEPARATOR "://"
#define ACLK_PROXY_ENV "env"
#define ACLK_PROXY_CONFIG_VAR "proxy"

ACLK_PROXY_TYPE aclk_verify_proxy(const char *string);

#endif //ACLK_COMMON_H
