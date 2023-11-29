// SPDX-License-Identifier: GPL-3.0-or-later

/** @file flb_plugin.h
 *  @brief Header of flb_plugin.c
 */

#ifndef FLB_PLUGIN_H_
#define FLB_PLUGIN_H_

#include "file_info.h"
#include <uv.h>

#define LOG_PATH_AUTO "auto"
#define KMSG_DEFAULT_PATH "/dev/kmsg"
#define SYSTEMD_DEFAULT_PATH "SD_JOURNAL_LOCAL_ONLY"
#define DOCKER_EV_DEFAULT_PATH "/var/run/docker.sock"

typedef struct {
    char *flush,
         *http_listen, *http_port, *http_server,
         *log_path, *log_level,
         *coro_stack_size;
} flb_srvc_config_t ;

int flb_init(flb_srvc_config_t flb_srvc_config, const char *const stock_config_dir);
int flb_run(void);
void flb_terminate(void);
void flb_complete_item_timer_timeout_cb(uv_timer_t *handle);
int flb_add_input(struct File_info *const p_file_info);
int flb_add_fwd_input(Flb_socket_config_t *const forward_in_config);
void flb_free_fwd_input_out_cb(void);

#endif // FLB_PLUGIN_H_
