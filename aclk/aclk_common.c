#include "aclk_common.h"

#include "../daemon/common.h"

#ifdef ENABLE_ACLK
#include <libwebsockets.h>
#endif

netdata_mutex_t aclk_shared_state_mutex = NETDATA_MUTEX_INITIALIZER;

int aclk_disable_runtime = 0;
int aclk_kill_link = 0;

struct aclk_shared_state aclk_shared_state = {
    .metadata_submitted = ACLK_METADATA_REQUIRED,
    .agent_state = AGENT_INITIALIZING,
    .last_popcorn_interrupt = 0,
    .version_neg = 0,
    .version_neg_wait_till = 0
};

struct {
    ACLK_PROXY_TYPE type;
    const char *url_str;
} supported_proxy_types[] = {
    { .type = PROXY_TYPE_SOCKS5,   .url_str = "socks5"  ACLK_PROXY_PROTO_ADDR_SEPARATOR },
    { .type = PROXY_TYPE_SOCKS5,   .url_str = "socks5h" ACLK_PROXY_PROTO_ADDR_SEPARATOR },
    { .type = PROXY_TYPE_HTTP,     .url_str = "http"    ACLK_PROXY_PROTO_ADDR_SEPARATOR },
    { .type = PROXY_TYPE_UNKNOWN,  .url_str = NULL                                      },
};

const char *aclk_proxy_type_to_s(ACLK_PROXY_TYPE *type)
{
    switch (*type) {
        case PROXY_DISABLED:
            return "disabled";
        case PROXY_TYPE_HTTP:
            return "HTTP";
        case PROXY_TYPE_SOCKS5:
            return "SOCKS";
        default:
            return "Unknown";
    }
}

static inline ACLK_PROXY_TYPE aclk_find_proxy(const char *string)
{
    int i = 0;
    while (supported_proxy_types[i].url_str) {
        if (!strncmp(supported_proxy_types[i].url_str, string, strlen(supported_proxy_types[i].url_str)))
            return supported_proxy_types[i].type;
        i++;
    }
    return PROXY_TYPE_UNKNOWN;
}

ACLK_PROXY_TYPE aclk_verify_proxy(const char *string)
{
    if (!string)
        return PROXY_TYPE_UNKNOWN;

    while (*string == 0x20 && *string!=0)   // Help coverity (compiler will remove)
        string++;

    if (!*string)
        return PROXY_TYPE_UNKNOWN;

    return aclk_find_proxy(string);
}

// helper function to censor user&password
// for logging purposes
void safe_log_proxy_censor(char *proxy)
{
    size_t length = strlen(proxy);
    char *auth = proxy + length - 1;
    char *cur;

    while ((auth >= proxy) && (*auth != '@'))
        auth--;

    //if not found or @ is first char do nothing
    if (auth <= proxy)
        return;

    cur = strstr(proxy, ACLK_PROXY_PROTO_ADDR_SEPARATOR);
    if (!cur)
        cur = proxy;
    else
        cur += strlen(ACLK_PROXY_PROTO_ADDR_SEPARATOR);

    while (cur < auth) {
        *cur = 'X';
        cur++;
    }
}

static inline void safe_log_proxy_error(char *str, const char *proxy)
{
    char *log = strdupz(proxy);
    safe_log_proxy_censor(log);
    error("%s Provided Value:\"%s\"", str, log);
    freez(log);
}

static inline int check_socks_enviroment(const char **proxy)
{
    char *tmp = getenv("socks_proxy");

    if (!tmp)
        return 1;

    if (aclk_verify_proxy(tmp) == PROXY_TYPE_SOCKS5) {
        *proxy = tmp;
        return 0;
    }

    safe_log_proxy_error(
        "Environment var \"socks_proxy\" defined but of unknown format. Supported syntax: \"socks5[h]://[user:pass@]host:ip\".",
        tmp);
    return 1;
}

static inline int check_http_enviroment(const char **proxy)
{
    char *tmp = getenv("http_proxy");

    if (!tmp)
        return 1;

    if (aclk_verify_proxy(tmp) == PROXY_TYPE_HTTP) {
        *proxy = tmp;
        return 0;
    }

    safe_log_proxy_error(
        "Environment var \"http_proxy\" defined but of unknown format. Supported syntax: \"http[s]://[user:pass@]host:ip\".",
        tmp);
    return 1;
}

const char *aclk_lws_wss_get_proxy_setting(ACLK_PROXY_TYPE *type)
{
    const char *proxy = config_get(CONFIG_SECTION_CLOUD, ACLK_PROXY_CONFIG_VAR, ACLK_PROXY_ENV);
    *type = PROXY_DISABLED;

    if (strcmp(proxy, "none") == 0)
        return proxy;

    if (strcmp(proxy, ACLK_PROXY_ENV) == 0) {
        if (check_socks_enviroment(&proxy) == 0) {
#ifdef LWS_WITH_SOCKS5
            *type = PROXY_TYPE_SOCKS5;
            return proxy;
#else
            safe_log_proxy_error("socks_proxy environment variable set to use SOCKS5 proxy "
                "but Libwebsockets used doesn't have SOCKS5 support built in. "
                "Ignoring and checking for other options.",
                proxy);
#endif
        }
        if (check_http_enviroment(&proxy) == 0)
            *type = PROXY_TYPE_HTTP;
        return proxy;
    }

    *type = aclk_verify_proxy(proxy);
#ifndef LWS_WITH_SOCKS5
    if (*type == PROXY_TYPE_SOCKS5) {
        safe_log_proxy_error(
            "Config var \"" ACLK_PROXY_CONFIG_VAR
            "\" set to use SOCKS5 proxy but Libwebsockets used is built without support for SOCKS proxy. ACLK will be disabled.",
            proxy);
    }
#endif
    if (*type == PROXY_TYPE_UNKNOWN) {
        *type = PROXY_DISABLED;
        safe_log_proxy_error(
            "Config var \"" ACLK_PROXY_CONFIG_VAR
            "\" defined but of unknown format. Supported syntax: \"socks5[h]://[user:pass@]host:ip\".",
            proxy);
    }

    return proxy;
}

// helper function to read settings only once (static)
// as claiming, challenge/response and ACLK
// read the same thing, no need to parse again
const char *aclk_get_proxy(ACLK_PROXY_TYPE *type)
{
    static const char *proxy = NULL;
    static ACLK_PROXY_TYPE proxy_type = PROXY_NOT_SET;

    if (proxy_type == PROXY_NOT_SET)
        proxy = aclk_lws_wss_get_proxy_setting(&proxy_type);

    *type = proxy_type;
    return proxy;
}

int aclk_decode_base_url(char *url, char **aclk_hostname, char **aclk_port)
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
        *aclk_port = strdupz("443");
        info("Setting ACLK target host=%s port=%s from %s", *aclk_hostname, *aclk_port, url);
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
        *aclk_port = callocz(port_end - host_end + 1, 1);
        for (int i = host_end + 1; i < port_end; i++)
            (*aclk_port)[i - host_end - 1] = url[i];
    }
    if (url[host_end] == '/') {
        *aclk_port = strdupz("443");
        *aclk_hostname = callocz(1, host_end - pos + 1);
        strncpy(*aclk_hostname, url+pos, host_end - pos);
    }
    info("Setting ACLK target host=%s port=%s from %s", *aclk_hostname, *aclk_port, url);
    return 0;
}
