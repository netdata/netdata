#include "aclk_common.h"

#include "daemon/common.h"

#ifdef ENABLE_ACLK
#include <libwebsockets.h>
#endif

netdata_mutex_t legacy_aclk_shared_state_mutex = NETDATA_MUTEX_INITIALIZER;

struct legacy_aclk_shared_state legacy_aclk_shared_state = {
    .version_neg = 0,
    .version_neg_wait_till = 0
};

int aclk_decode_base_url(char *url, char **aclk_hostname, int *aclk_port)
{
    int pos = 0;
    if (!strncmp("https://", url, 8)) {
        pos = 8;
    } else if (!strncmp("http://", url, 7)) {
        error("Cannot connect ACLK over %s -> unencrypted link is not supported", url);
        return 1;
    }
    int host_end = pos;
    while (url[host_end] != 0 && url[host_end] != '/' && url[host_end] != ':')
        host_end++;
    if (url[host_end] == 0) {
        *aclk_hostname = strdupz(url + pos);
        *aclk_port = 443;
        info("Setting ACLK target host=%s port=%d from %s", *aclk_hostname, *aclk_port, url);
        return 0;
    }
    if (url[host_end] == ':') {
        *aclk_hostname = callocz(host_end - pos + 1, 1);
        strncpy(*aclk_hostname, url + pos, host_end - pos);
        int port_end = host_end + 1;
        while (url[port_end] >= '0' && url[port_end] <= '9')
            port_end++;
        if (port_end - host_end > 6) {
            error("Port specified in %s is invalid", url);
            return 0;
        }
        *aclk_port = atoi(&url[host_end+1]);
    }
    if (url[host_end] == '/') {
        *aclk_port = 443;
        *aclk_hostname = callocz(1, host_end - pos + 1);
        strncpy(*aclk_hostname, url+pos, host_end - pos);
    }
    info("Setting ACLK target host=%s port=%d from %s", *aclk_hostname, *aclk_port, url);
    return 0;
}
