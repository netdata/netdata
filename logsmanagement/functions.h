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

int logsmanagement_function_facets( BUFFER *wb, int timeout, const char *function, void *collector_data, 
                                    rrd_function_result_callback_t result_cb, void *result_cb_data,
                                    rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                                    rrd_function_register_canceller_cb_t register_cancel_cb, void *register_cancel_db_data);

#endif // FUNCTIONS_H_