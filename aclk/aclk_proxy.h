#ifndef ACLK_PROXY_H
#define ACLK_PROXY_H

#include <config.h>

#define ACLK_PROXY_PROTO_ADDR_SEPARATOR "://"

typedef enum aclk_proxy_type {
    PROXY_TYPE_UNKNOWN = 0,
    PROXY_TYPE_SOCKS5,
    PROXY_TYPE_HTTP,
    PROXY_DISABLED,
    PROXY_NOT_SET,
} ACLK_PROXY_TYPE;

ACLK_PROXY_TYPE aclk_verify_proxy(const char *string);
const char *aclk_lws_wss_get_proxy_setting(ACLK_PROXY_TYPE *type);
void safe_log_proxy_censor(char *proxy);
const char *aclk_get_proxy(ACLK_PROXY_TYPE *type);

#endif /* ACLK_PROXY_H */
