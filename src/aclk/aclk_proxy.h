#ifndef ACLK_PROXY_H
#define ACLK_PROXY_H

#include "aclk.h"

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
const char *aclk_get_proxy(ACLK_PROXY_TYPE *type, bool for_logging);
const char *aclk_get_proxy_source(void);
void aclk_proxy_get_display(char *buf, size_t buflen, const char *proxy, ACLK_PROXY_TYPE type);
void aclk_proxy_get_full_display(char *buf, size_t buflen);

static inline const char *aclk_proxy_type_to_url(ACLK_PROXY_TYPE type) {
    switch (type) {
        case PROXY_TYPE_HTTP: return "http://";
        case PROXY_TYPE_SOCKS5: return "socks5://";
        default: return "";
    }
}

#endif /* ACLK_PROXY_H */
