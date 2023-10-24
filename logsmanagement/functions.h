// SPDX-License-Identifier: GPL-3.0-or-later

/** @file functions.h
 *  @brief Header of functions.c 
 */

#ifndef FUNCTIONS_H_
#define FUNCTIONS_H_

#include "../database/rrdfunctions.h"

#define LOGS_MANAG_FUNC_NAME "logsmanagement"
#define FUNCTION_LOGSMANAGEMENT_HELP_SHORT "Query of logs management engine running on this node"

int logsmanagement_function_execute_cb( BUFFER *dest_wb, int timeout, 
                                        const char *function, void *collector_data, 
                                        void (*callback)(BUFFER *wb, int code, void *callback_data), 
                                        void *callback_data);

void logsmanagement_func_facets_init(bool *p_logsmanagement_should_exit);

#endif // FUNCTIONS_H_