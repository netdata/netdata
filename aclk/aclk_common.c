#include "aclk_common.h"

struct {
    ACLK_PROXY_TYPE type;
    const char *url_str;
} supported_proxy_types[] = {
    { .type = PROXY_TYPE_SOCKS5,   .url_str = "socks5" ACLK_PROXY_PROTO_ADDR_SEPARATOR  },
    { .type = PROXY_TYPE_SOCKS5,   .url_str = "socks5h" ACLK_PROXY_PROTO_ADDR_SEPARATOR },
    { .type = PROXY_TYPE_UNKNOWN,  .url_str = NULL                                      },
};

static inline ACLK_PROXY_TYPE aclk_find_proxy(const char *string)
{
    int i = 0;
    while( supported_proxy_types[i].url_str ) {
        if(!strncmp(supported_proxy_types[i].url_str, string, strlen(supported_proxy_types[i].url_str)))
            return supported_proxy_types[i].type;
        i++;
    }
    return PROXY_TYPE_UNKNOWN;
}

ACLK_PROXY_TYPE aclk_verify_proxy(const char *string)
{
    if(!string)
        return PROXY_TYPE_UNKNOWN;

    while(*string == 0x20)
        string++;

    if(!*string)
        return PROXY_TYPE_UNKNOWN;

    return aclk_find_proxy(string);
}
