#include "aclk_common.h"

struct {
    ACLK_PROXY_TYPE type;
    const char *url_str;
} supported_proxy_types[] = {
    { .type = SOCKS5,   ACLK_PROXY_PROTO_SOCKS5  },
    { .type = SOCKS5,   ACLK_PROXY_PROTO_SOCKS5H },
    { .type = -1,       NULL                     },
};

static inline int aclk_find_proxy(const char *string)
{
    int i = 0;
    while( supported_proxy_types[i].type >= 0 ) {
        if(!strncmp(supported_proxy_types[i].url_str, string, strlen(supported_proxy_types[i].url_str)))
            return i;
        i++;
    }
    return -1;
}

ACLK_PROXY_TYPE aclk_verify_proxy(const char *string)
{
    int i = 0;

    if(!string)
        return -1;

    while(*string == 0x20)
        string++;

    if(!*string)
        return -1;

    i = aclk_find_proxy(string);
    if(i > 0)
        return supported_proxy_types[i].type;

    return -1;
}
