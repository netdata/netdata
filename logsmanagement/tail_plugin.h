/** @file tail_plugin.h
 *  @brief Header of tail_plugin.c 
 *
 *  @author Dimitris Pantazis
 */

#ifndef TAIL_PLUGIN_H_
#define TAIL_PLUGIN_H_

#include "file_info.h"

int tail_plugin_add_input(struct File_info *const p_file_info);
int tail_plugin_init(struct File_infos_arr *const p_file_infos_arr);

#endif  // TAIL_PLUGIN_H_