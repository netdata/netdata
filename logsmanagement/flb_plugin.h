/** @file flb_plugin.h
 *  @brief Header of flb_plugin.c
 *
 *  @author Dimitris Pantazis
 */

#ifndef FLB_PLUGIN_H_
#define FLB_PLUGIN_H_

#include "file_info.h"
#include <uv.h>

int flb_init(void);
int flb_run(void);
void flb_stop_and_cleanup(void);
void flb_tmp_buff_cpy_timer_cb(uv_timer_t *handle);
int flb_add_input(struct File_info *const p_file_info);

#define SYSTEMD_DEFAULT_PATH "systemd_default"
#define DOCKER_EV_DEFAULT_PATH "docker_ev_default"

#endif // FLB_PLUGIN_H_
