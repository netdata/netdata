// SPDX-License-Identifier: GPL-3.0-or-later

/** @file logsmanagement.c
 *  @brief This is the main file of the Netdata logs management project
 *
 *  The aim of the project is to add the capability to collect, parse and
 *  query logs in the Netdata agent. For more information please refer 
 *  to the project's [README](README.md) file.
 */

#include <uv.h>
#include "daemon/common.h"
#include "db_api.h"
#include "file_info.h"
#include "flb_plugin.h"
#include "functions.h"
#include "helper.h"
#include "libnetdata/required_dummies.h"
#include "logsmanag_config.h"
#include "rrd_api/rrd_api_stats.h"

#if defined(ENABLE_LOGSMANAGEMENT_TESTS)
#include "logsmanagement/unit_test/unit_test.h"
#endif

netdata_mutex_t stdout_mut = NETDATA_MUTEX_INITIALIZER;

bool logsmanagement_should_exit = false;

struct File_infos_arr *p_file_infos_arr = NULL;

static uv_loop_t *main_loop;

static struct {
    uv_signal_t sig;
    const int signum;
} signals[] = {
    // Add here signals that will terminate the plugin
    {.signum = SIGINT},
    {.signum = SIGQUIT},
    {.signum = SIGPIPE},
    {.signum = SIGTERM}
};

static void signal_handler(uv_signal_t *handle, int signum __maybe_unused) {
    UNUSED(handle);

    debug_log("Signal received: %d\n", signum);

    __atomic_store_n(&logsmanagement_should_exit, true, __ATOMIC_RELAXED);

}

static void on_walk_cleanup(uv_handle_t* handle, void* data){
    UNUSED(data);
    if (!uv_is_closing(handle)) 
        uv_close(handle, NULL);
}

/**
 * @brief The main function of the logs management plugin.
 * @details Any static asserts are most likely going to be inluded here. After 
 * any initialisation routines, the default uv_loop_t is executed indefinitely. 
 */
int main(int argc, char **argv) {

    /* Static asserts */
    #pragma GCC diagnostic push
    #pragma GCC diagnostic ignored "-Wunused-local-typedefs" 
    COMPILE_TIME_ASSERT(SAVE_BLOB_TO_DB_MIN <= SAVE_BLOB_TO_DB_MAX);
    COMPILE_TIME_ASSERT(CIRCULAR_BUFF_DEFAULT_MAX_SIZE >= CIRCULAR_BUFF_MAX_SIZE_RANGE_MIN); 
    COMPILE_TIME_ASSERT(CIRCULAR_BUFF_DEFAULT_MAX_SIZE <= CIRCULAR_BUFF_MAX_SIZE_RANGE_MAX);
    #pragma GCC diagnostic pop

    clocks_init();

    program_name = LOGS_MANAGEMENT_PLUGIN_STR;

    nd_log_initialize_for_external_plugins(program_name);

    // netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    // if(verify_netdata_host_prefix(true) == -1) exit(1);

    int g_update_every = 0;
    for(int i = 1; i < argc ; i++) {
        if(isdigit(*argv[i]) && !g_update_every && str2i(argv[i]) > 0 && str2i(argv[i]) < 86400) {
            g_update_every = str2i(argv[i]);
            debug_log("new update_every received: %d", g_update_every);
        }
        else if(!strcmp("--unittest", argv[i])) {
#if defined(ENABLE_LOGSMANAGEMENT_TESTS)
            exit(logs_management_unittest());
#else
            collector_error("%s was not built with unit test support.", program_name);
#endif
        }
        else if(!strcmp("version", argv[i]) || 
                !strcmp("-version", argv[i]) || 
                !strcmp("--version", argv[i]) || 
                !strcmp("-v", argv[i]) || 
                !strcmp("-V", argv[i])) {
            printf(VERSION"\n");
            exit(0);
        }
        else if(!strcmp("-h", argv[i]) || 
                !strcmp("--help", argv[i])) {
            fprintf(stderr,
                    "\n"
                    " netdata %s %s\n"
                    " Copyright (C) 2023 Netdata Inc.\n"
                    " Released under GNU General Public License v3 or later.\n"
                    " All rights reserved.\n"
                    "\n"
                    " This program is the logs management plugin for netdata.\n"
                    "\n"
                    " Available command line options:\n"
                    "\n"
                    "  --unittest              run unit tests and exit\n"
                    "\n"
                    "  -v\n"
                    "  -V\n"
                    "  --version               print version and exit\n"
                    "\n"
                    "  -h\n"
                    "  --help                  print this message and exit\n"
                    "\n"
                    " For more information:\n"
                    " https://github.com/netdata/netdata/tree/master/collectors/logs-management.plugin\n"
                    "\n",
                    program_name,
                    VERSION
            );
            exit(1);
        }
        else
            collector_error("%s(): ignoring parameter '%s'", __FUNCTION__, argv[i]);
    }

    Flb_socket_config_t *p_forward_in_config = NULL;

    main_loop = mallocz(sizeof(uv_loop_t));
    fatal_assert(uv_loop_init(main_loop) == 0);

    flb_srvc_config_t flb_srvc_config = {
        .flush           = FLB_FLUSH_DEFAULT,
        .http_listen     = FLB_HTTP_LISTEN_DEFAULT,
        .http_port       = FLB_HTTP_PORT_DEFAULT,
        .http_server     = FLB_HTTP_SERVER_DEFAULT,
        .log_path        = "NULL",
        .log_level       = FLB_LOG_LEVEL_DEFAULT,
        .coro_stack_size = FLB_CORO_STACK_SIZE_DEFAULT
    };

    p_file_infos_arr = callocz(1, sizeof(struct File_infos_arr));

    if(logs_manag_config_load(&flb_srvc_config, &p_forward_in_config, g_update_every)) 
        exit(1);    

    if(flb_init(flb_srvc_config, get_stock_config_dir(), g_logs_manag_config.sd_journal_field_prefix)){
        collector_error("flb_init() failed - logs management will be disabled");
        exit(1);
    }

    if(flb_add_fwd_input(p_forward_in_config))
        collector_error("flb_add_fwd_input() failed - logs management forward input will be disabled");

    /* Initialize logs management for each configuration section  */
    config_file_load(main_loop, p_forward_in_config, &flb_srvc_config, &stdout_mut);

    if(p_file_infos_arr->count == 0){
        collector_info("No valid configuration could be found for any log source - logs management will be disabled");
        exit(1);
    }

    /* Run Fluent Bit engine
     * NOTE: flb_run() ideally would be executed after db_init(), but in case of
     * a db_init() failure, it is easier to call flb_stop_and_cleanup() rather 
     * than the other way round (i.e. cleaning up after db_init(), if flb_run() 
     * fails). */
    if(flb_run()){
        collector_error("flb_run() failed - logs management will be disabled");
        exit(1);
    }

    if(db_init()){
        collector_error("db_init() failed - logs management will be disabled");
        exit(1);
    }

    uv_thread_t *p_stats_charts_thread_id = NULL;
    const char *const netdata_internals_monitoring = getenv("NETDATA_INTERNALS_MONITORING");
    if( netdata_internals_monitoring && 
        *netdata_internals_monitoring && 
        strcmp(netdata_internals_monitoring, "YES") == 0){

        p_stats_charts_thread_id = mallocz(sizeof(uv_thread_t));
        fatal_assert(0 == uv_thread_create(p_stats_charts_thread_id, stats_charts_init, &stdout_mut));
    }
    
#if defined(__STDC_VERSION__)
    debug_log( "__STDC_VERSION__: %ld", __STDC_VERSION__);
#else
    debug_log( "__STDC_VERSION__ undefined");
#endif // defined(__STDC_VERSION__)
    debug_log( "libuv version: %s", uv_version_string());
    debug_log( "LZ4 version: %s", LZ4_versionString());
    debug_log( "SQLITE version: " SQLITE_VERSION);

    for(int i = 0; i < (int) (sizeof(signals) / sizeof(signals[0])); i++){
        uv_signal_init(main_loop, &signals[i].sig);
        uv_signal_start(&signals[i].sig, signal_handler, signals[i].signum);
    } 

    struct functions_evloop_globals *wg = logsmanagement_func_facets_init(&logsmanagement_should_exit);

    collector_info("%s setup completed successfully", program_name);

    /* Run uvlib loop. */
    while(!__atomic_load_n(&logsmanagement_should_exit, __ATOMIC_RELAXED))
        uv_run(main_loop, UV_RUN_ONCE);

    /* If there are valid log sources, there should always be valid handles */
    collector_info("uv_run(main_loop, ...); no handles or requests - cleaning up...");
    
    nd_log_limits_unlimited();

    // TODO: Clean up stats charts memory
    if(p_stats_charts_thread_id){
        uv_thread_join(p_stats_charts_thread_id);
        freez(p_stats_charts_thread_id);
    }

    uv_stop(main_loop);

    flb_terminate();

    flb_free_fwd_input_out_cb();

    p_file_info_destroy_all();

    uv_walk(main_loop, on_walk_cleanup, NULL);
    while(0 != uv_run(main_loop, UV_RUN_ONCE));
    if(uv_loop_close(main_loop))
        m_assert(0, "uv_loop_close() result not 0");
    freez(main_loop);

    functions_evloop_cancel_threads(wg);

    collector_info("logs management clean up done - exiting");

    exit(0);
}
