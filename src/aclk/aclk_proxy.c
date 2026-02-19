#include "aclk_proxy.h"

#define ACLK_PROXY_ENV "env"
#define ACLK_PROXY_CONFIG_VAR "proxy"

struct {
    ACLK_PROXY_TYPE type;
    const char *url_str;
} supported_proxy_types[] = {
    { .type = PROXY_TYPE_SOCKS5,   .url_str = "socks5"  ACLK_PROXY_PROTO_ADDR_SEPARATOR },
    { .type = PROXY_TYPE_SOCKS5,   .url_str = "socks5h" ACLK_PROXY_PROTO_ADDR_SEPARATOR },
    { .type = PROXY_TYPE_HTTP,     .url_str = "http"    ACLK_PROXY_PROTO_ADDR_SEPARATOR },
    { .type = PROXY_TYPE_UNKNOWN,  .url_str = NULL                                      },
};

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

    while (*string == 0x20)
        string++;

    if (!*string)
        return PROXY_TYPE_UNKNOWN;

    return aclk_find_proxy(string);
}

// helper function to censor user&password
// for logging purposes
void safe_log_proxy_censor(char *proxy)
{
    if (!proxy)
        return;

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
    netdata_log_error("%s Provided Value:\"%s\"", str, log);
    freez(log);
}

// helper to extract "http://host:port" from a proxy URL, skipping credentials
void aclk_proxy_get_display(char *buf, size_t buflen, const char *proxy, ACLK_PROXY_TYPE type)
{
    const char *at = strrchr(proxy, '@');
    const char *host_start = at ? at + 1 : proxy;
    const char *sep = strstr(proxy, ACLK_PROXY_PROTO_ADDR_SEPARATOR);
    if (!at && sep)
        host_start = sep + strlen(ACLK_PROXY_PROTO_ADDR_SEPARATOR);
    snprintfz(buf, buflen, "%s%s", aclk_proxy_type_to_url(type), host_start);
}

static const char *proxy_source = NULL;

static inline int check_http_environment(const char **proxy)
{
    const char *var = "http_proxy";
    char *tmp = getenv(var);

    if (!tmp || !*tmp) {
        var = "https_proxy";
        tmp = getenv(var);
        if (!tmp || !*tmp)
            return 1;
    }

    if (aclk_verify_proxy(tmp) == PROXY_TYPE_HTTP) {
        *proxy = tmp;
        char display[512];
        aclk_proxy_get_display(display, sizeof(display), tmp, PROXY_TYPE_HTTP);
        char source_buf[256];
        snprintfz(source_buf, sizeof(source_buf), "environment variable '%s'", var);
        freez((void *)proxy_source);
        proxy_source = strdupz(source_buf);
        nd_log(NDLS_DAEMON, NDLP_INFO,
               "ACLK: using HTTP proxy %s (%s, from %s)",
               display, strchr(tmp, '@') ? "with credentials" : "without credentials", proxy_source);
        return 0;
    }

    char buf[1024];
    snprintfz(buf, sizeof(buf),
              "Environment var '%s' defined but of unknown format '%s'. "
              "Supported syntax: 'http://[user:pass@]host:port'.",
              var, tmp);
    safe_log_proxy_error(buf, tmp);

    return 1;
}

const char *aclk_lws_wss_get_proxy_setting(ACLK_PROXY_TYPE *type)
{
    const char *proxy = cloud_config_proxy_get();

    *type = PROXY_DISABLED;

    if (!proxy || !*proxy || strcmp(proxy, "none") == 0) {
        nd_log(NDLS_DAEMON, NDLP_INFO,
               "ACLK: proxy is %s, will connect directly without proxy.",
               (!proxy || !*proxy) ? "not configured" : "set to 'none'");
        freez((void *)proxy_source);
        proxy_source = NULL;
        return proxy;
    }

    if (strcmp(proxy, ACLK_PROXY_ENV) == 0) {
        if (check_http_environment(&proxy) == 0)
            *type = PROXY_TYPE_HTTP;
        else {
            if (cloud_config_proxy_is_explicitly_set())
                nd_log(NDLS_DAEMON, NDLP_WARNING,
                       "ACLK: proxy is explicitly set to 'env' but neither 'http_proxy' nor 'https_proxy'"
                       " environment variables are set. Will connect directly without proxy.");

            freez((void *)proxy_source);
            proxy_source = NULL;
            proxy = NULL;
        }
        return proxy;
    }

    *type = aclk_verify_proxy(proxy);

    if (*type == PROXY_TYPE_UNKNOWN) {
        *type = PROXY_DISABLED;
        safe_log_proxy_error(
            "Config var \"" ACLK_PROXY_CONFIG_VAR
            "\" defined but of unknown format. Supported syntax: \"socks5[h]://[user:pass@]host:ip\".",
            proxy);
        freez((void *)proxy_source);
        proxy_source = NULL;
    }
    else {
        const char *src = cloud_config_proxy_source_get();
        freez((void *)proxy_source);
        proxy_source = src ? strdupz(src) : NULL;
        char display[512];
        aclk_proxy_get_display(display, sizeof(display), proxy, *type);
        nd_log(NDLS_DAEMON, NDLP_INFO,
               "ACLK: using %s proxy %s (%s, from %s)",
               *type == PROXY_TYPE_HTTP ? "HTTP" : "SOCKS5",
               display,
               strchr(proxy, '@') ? "with credentials" : "without credentials",
               proxy_source);
    }

    return proxy;
}

// helper function to read settings only once (static)
// as claiming, challenge/response and ACLK
// read the same thing, no need to parse again
const char *aclk_get_proxy(ACLK_PROXY_TYPE *return_type, bool for_logging)
{
    static const char *proxy = NULL;
    static const char *safe_proxy = NULL;
    static ACLK_PROXY_TYPE proxy_type = PROXY_NOT_SET;

    if (proxy_type == PROXY_NOT_SET) {
        proxy = aclk_lws_wss_get_proxy_setting(&proxy_type);
        char *log = NULL;
        if (proxy) {
            log = strdupz(proxy);
            safe_log_proxy_censor(log);
        }
        safe_proxy = log;
    }

    if (return_type)
        *return_type = proxy_type;
    return for_logging ? safe_proxy : proxy;
}

const char *aclk_get_proxy_source(void) {
    return proxy_source;
}

void aclk_proxy_get_full_display(char *buf, size_t buflen) {
    ACLK_PROXY_TYPE proxy_type;
    const char *proxy_str = aclk_get_proxy(&proxy_type, false);

    if (proxy_type == PROXY_DISABLED || proxy_type == PROXY_NOT_SET || !proxy_str) {
        snprintfz(buf, buflen, "none");
        return;
    }

    char host_display[256];
    aclk_proxy_get_display(host_display, sizeof(host_display), proxy_str, proxy_type);

    const char *source = aclk_get_proxy_source();
    snprintfz(buf, buflen, "%s (%s, from %s)",
              host_display,
              strchr(proxy_str, '@') ? "with credentials" : "without credentials",
              source ? source : "unknown");
}
