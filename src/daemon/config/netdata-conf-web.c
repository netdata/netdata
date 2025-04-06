// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-web.h"
#include "daemon/static_threads.h"

size_t netdata_conf_web_query_threads(void) {
    // See https://github.com/netdata/netdata/issues/11081#issuecomment-831998240 for more details
    if (OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110) {
        inicfg_set_number(&netdata_config, CONFIG_SECTION_WEB, "web server threads", 1);
        netdata_log_info("You are running an OpenSSL older than 1.1.0, web server will not enable multithreading.");
        return 1;
    }

    size_t cpus = MIN(netdata_conf_cpus(), 256); // max 256 cores
    size_t threads = cpus * (netdata_conf_is_parent() ? 2 : 1);
    threads = MAX(threads, 6);

    threads = inicfg_get_number(&netdata_config, CONFIG_SECTION_WEB, "web server threads", threads);
    if(threads < 1) {
        netdata_log_error("[" CONFIG_SECTION_WEB "].web server threads in netdata.conf needs to be at least 1. Overwriting it.");
        threads = 1;
        inicfg_set_number(&netdata_config, CONFIG_SECTION_WEB, "web server threads", threads);
    }
    return threads;
}

static int make_dns_decision(const char *section_name, const char *config_name, const char *default_value, SIMPLE_PATTERN *p) {
    const char *value = inicfg_get(&netdata_config, section_name,config_name,default_value);

    if(!strcmp("yes",value))
        return 1;

    if(!strcmp("no",value))
        return 0;

    if(strcmp("heuristic",value) != 0)
        netdata_log_error("Invalid configuration option '%s' for '%s'/'%s'. Valid options are 'yes', 'no' and 'heuristic'. Proceeding with 'heuristic'",
                          value, section_name, config_name);

    return simple_pattern_is_potential_name(p);
}

extern struct netdata_static_thread *static_threads;
void web_server_threading_selection(void) {
    FUNCTION_RUN_ONCE();

    web_server_mode = web_server_mode_id(inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "mode", web_server_mode_name(web_server_mode)));

    int static_threaded = (web_server_mode == WEB_SERVER_MODE_STATIC_THREADED);

    int i;
    for (i = 0; static_threads[i].name; i++) {
        if (static_threads[i].start_routine == socket_listen_main_static_threaded)
            static_threads[i].enabled = static_threaded;
    }
}

void netdata_conf_section_web(void) {
    FUNCTION_RUN_ONCE();

    web_client_timeout =
        (int)inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_WEB, "disconnect idle clients after", web_client_timeout);

    web_client_first_request_timeout =
        (int)inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_WEB, "timeout for first request", web_client_first_request_timeout);

    web_client_streaming_rate_t =
        inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_WEB, "accept a streaming request every", web_client_streaming_rate_t);

    respect_web_browser_do_not_track_policy =
        inicfg_get_boolean(&netdata_config, CONFIG_SECTION_WEB, "respect do not track policy", respect_web_browser_do_not_track_policy);
    web_x_frame_options = inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "x-frame-options response header", "");
    if(!*web_x_frame_options)
        web_x_frame_options = NULL;

    web_allow_connections_from =
        simple_pattern_create(inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "allow connections from", "localhost *"),
                              NULL, SIMPLE_PATTERN_EXACT, true);
    web_allow_connections_dns  =
        make_dns_decision(CONFIG_SECTION_WEB, "allow connections by dns", "heuristic", web_allow_connections_from);
    web_allow_dashboard_from   =
        simple_pattern_create(inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "allow dashboard from", "localhost *"),
                              NULL, SIMPLE_PATTERN_EXACT, true);
    web_allow_dashboard_dns    =
        make_dns_decision(CONFIG_SECTION_WEB, "allow dashboard by dns", "heuristic", web_allow_dashboard_from);
    web_allow_badges_from      =
        simple_pattern_create(inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "allow badges from", "*"), NULL, SIMPLE_PATTERN_EXACT,
                              true);
    web_allow_badges_dns       =
        make_dns_decision(CONFIG_SECTION_WEB, "allow badges by dns", "heuristic", web_allow_badges_from);
    web_allow_registry_from    =
        simple_pattern_create(inicfg_get(&netdata_config, CONFIG_SECTION_REGISTRY, "allow from", "*"), NULL, SIMPLE_PATTERN_EXACT,
                              true);
    web_allow_registry_dns     = make_dns_decision(CONFIG_SECTION_REGISTRY, "allow by dns", "heuristic",
                                               web_allow_registry_from);
    web_allow_streaming_from   = simple_pattern_create(inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "allow streaming from", "*"),
                                                     NULL, SIMPLE_PATTERN_EXACT, true);
    web_allow_streaming_dns    = make_dns_decision(CONFIG_SECTION_WEB, "allow streaming by dns", "heuristic",
                                                web_allow_streaming_from);
    // Note the default is not heuristic, the wildcards could match DNS but the intent is ip-addresses.
    web_allow_netdataconf_from = simple_pattern_create(inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "allow netdata.conf from",
                                                                  "localhost fd* 10.* 192.168.* 172.16.* 172.17.* 172.18.*"
                                                                  " 172.19.* 172.20.* 172.21.* 172.22.* 172.23.* 172.24.*"
                                                                  " 172.25.* 172.26.* 172.27.* 172.28.* 172.29.* 172.30.*"
                                                                  " 172.31.* UNKNOWN"), NULL, SIMPLE_PATTERN_EXACT,
                                                       true);
    web_allow_netdataconf_dns  =
        make_dns_decision(CONFIG_SECTION_WEB, "allow netdata.conf by dns", "no", web_allow_netdataconf_from);
    web_allow_mgmt_from        =
        simple_pattern_create(inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "allow management from", "localhost"),
                              NULL, SIMPLE_PATTERN_EXACT, true);
    web_allow_mgmt_dns         =
        make_dns_decision(CONFIG_SECTION_WEB, "allow management by dns","heuristic",web_allow_mgmt_from);

    web_enable_gzip = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_WEB, "enable gzip compression", web_enable_gzip);

    const char *s = inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "gzip compression strategy", "default");
    if(!strcmp(s, "default"))
        web_gzip_strategy = Z_DEFAULT_STRATEGY;
    else if(!strcmp(s, "filtered"))
        web_gzip_strategy = Z_FILTERED;
    else if(!strcmp(s, "huffman only"))
        web_gzip_strategy = Z_HUFFMAN_ONLY;
    else if(!strcmp(s, "rle"))
        web_gzip_strategy = Z_RLE;
    else if(!strcmp(s, "fixed"))
        web_gzip_strategy = Z_FIXED;
    else {
        netdata_log_error("Invalid compression strategy '%s'. Valid strategies are 'default', 'filtered', 'huffman only', 'rle' and 'fixed'. Proceeding with 'default'.", s);
        web_gzip_strategy = Z_DEFAULT_STRATEGY;
    }

    web_gzip_level = (int)inicfg_get_number(&netdata_config, CONFIG_SECTION_WEB, "gzip compression level", 3);
    if(web_gzip_level < 1) {
        netdata_log_error("Invalid compression level %d. Valid levels are 1 (fastest) to 9 (best ratio). Proceeding with level 1 (fastest compression).", web_gzip_level);
        web_gzip_level = 1;
    }
    else if(web_gzip_level > 9) {
        netdata_log_error("Invalid compression level %d. Valid levels are 1 (fastest) to 9 (best ratio). Proceeding with level 9 (best compression).", web_gzip_level);
        web_gzip_level = 9;
    }
}

void netdata_conf_web_security_init(void) {
    FUNCTION_RUN_ONCE();

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/ssl/key.pem", netdata_configured_user_config_dir);
    netdata_ssl_security_key = inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "ssl key", filename);

    snprintfz(filename, FILENAME_MAX, "%s/ssl/cert.pem", netdata_configured_user_config_dir);
    netdata_ssl_security_cert = inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "ssl certificate", filename);

    tls_version    = inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "tls version",  "1.3");
    tls_ciphers    = inicfg_get(&netdata_config, CONFIG_SECTION_WEB, "tls ciphers",  "none");
}

