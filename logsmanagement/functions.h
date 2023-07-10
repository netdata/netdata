// SPDX-License-Identifier: GPL-3.0-or-later

/** @file functions.h
 *  @brief Header of functions.c 
 */

#ifndef FUNCTIONS_H_
#define FUNCTIONS_H_

#include "../database/rrdfunctions.h"

#define FUNCTION_LOGSMANAGEMENT_HELP_SHORT "Query of logs management engine running on this node"

int logsmanagement_function_execute_cb( BUFFER *dest_wb, int timeout, 
                                        const char *function, void *collector_data, 
                                        void (*callback)(BUFFER *wb, int code, void *callback_data), 
                                        void *callback_data);

#endif // FUNCTIONS_H_