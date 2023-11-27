// SPDX-License-Identifier: GPL-3.0-or-later

/** @file   logsmanag_config.h
 *  @brief  Header of logsmanag_config.c
 */

#include "file_info.h"
#include "flb_plugin.h"

char *get_user_config_dir(void);

char *get_stock_config_dir(void);

char *get_log_dir(void);

char *get_cache_dir(void);

void p_file_info_destroy_all(void);

#define LOGS_MANAG_CONFIG_LOAD_ERROR_OK                  0
#define LOGS_MANAG_CONFIG_LOAD_ERROR_NO_STOCK_CONFIG    -1
#define LOGS_MANAG_CONFIG_LOAD_ERROR_P_FLB_SRVC_NULL    -2

int logs_manag_config_load( flb_srvc_config_t *p_flb_srvc_config, 
                            Flb_socket_config_t **forward_in_config_p,
                            int g_update_every);

void config_file_load(  uv_loop_t *main_loop,
                        Flb_socket_config_t *p_forward_in_config, 
                        flb_srvc_config_t *p_flb_srvc_config,
                        netdata_mutex_t *stdout_mut);