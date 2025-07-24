// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"
#include "netdata-conf-ssl.h"
#include <curl/curl.h>

static bool is_ca_bundle_valid(const char *ca_path) {
    if (!ca_path || !*ca_path)
        return false;

    FILE *fp = fopen(ca_path, "r");
    if (!fp)
        return false;

    ERR_clear_error();

    int valid_certs = 0;
    X509 *cert = NULL;
    while ((cert = PEM_read_X509(fp, NULL, NULL, NULL)) != NULL) {
        ASN1_TIME *not_after = X509_get_notAfter(cert);
        int day, sec;

        if (ASN1_TIME_diff(&day, &sec, NULL, not_after) == 1) {
            if (day > 0 || (day == 0 && sec > 0)) {
                valid_certs++;

                // we found 1 valid, stop reading the file
                X509_free(cert);
                break;
            }
        }

        X509_free(cert);
    }

    fclose(fp);

    ERR_clear_error();
    return valid_certs > 0;
}

const char *detect_libcurl_default_ca() {
#if LIBCURL_VERSION_NUM >= 0x074600  // 7.70.0 (CURLVERSION_SEVENTH)
    curl_version_info_data *info = curl_version_info(CURLVERSION_NOW);
    if (info) {
        // Check built-in CA bundle
        if (info->cainfo &&
            access(info->cainfo, R_OK) == 0 &&
            is_ca_bundle_valid(info->cainfo))
            return info->cainfo;
    }
#endif
    return NULL;
}

static inline const char *detect_ca_path(void) {
    static const char *paths[] = {
        "/opt/netdata/etc/ssl/certs/ca-certificates.crt",   // Netdata static build (needs to come first for consistency with standalone cURL in static builds)
        "/etc/ssl/certs/ca-certificates.crt",               // Debian, Ubuntu, Arch
        "/etc/ssl/certs/ca-bundle.crt",                     // Rocky Linux (via symlinks)
        "/etc/pki/tls/certs/ca-bundle.crt",                 // RHEL, CentOS, Fedora
        "/etc/ssl/ca-bundle.pem",                           // OpenSUSE
        "/etc/ssl/cert.pem",                                // Alpine
        "/opt/netdata/share/ssl/certs/ca-certificates.crt", // Netdata static build - fallback
        NULL
    };

    for (int i = 0; paths[i] != NULL; i++) {
        if (access(paths[i], R_OK) == 0 &&
            is_ca_bundle_valid(paths[i]))
            return paths[i];
    }

    return NULL;
}

void netdata_conf_ssl(void) {
    FUNCTION_RUN_ONCE();

    netdata_ssl_initialize_openssl();

#if 0
    const char *p = getenv("CURL_CA_BUNDLE");
    if(!p || !*p) p = getenv("SSL_CERT_FILE");
    if(!p || !*p) {
        p = X509_get_default_cert_file();
        if(!p || !*p || !is_ca_bundle_valid(p))
            p = NULL;
    }
    if(!p || !*p) p = detect_libcurl_default_ca();
    if(!p || !*p) p = detect_ca_path();
    setenv("CURL_CA_BUNDLE", inicfg_get(&netdata_config, CONFIG_SECTION_ENV_VARS, "CURL_CA_BUNDLE", p ? p : ""), 1);
    setenv("SSL_CERT_FILE", inicfg_get(&netdata_config, CONFIG_SECTION_ENV_VARS, "SSL_CERT_FILE", p ? p : ""), 1);
#endif

}
