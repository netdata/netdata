// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_FLIGHT_RECORDER_H
#define ACLK_FLIGHT_RECORDER_H

#include "aclk_events.h"

struct aclk_fl;

int aclk_flight_recorder_init();
void aclk_new_connection_log();

void aclk_log_info(aclk_event_log_t event_id, const char *file, const char *function, const unsigned long line, const char *fmt, ...);
#define aclk_log_event_info(event_id, fmt, args...) aclk_log_info(event_id, __FILE__, __FUNCTION__, __LINE__, fmt, ##args)

void aclk_log_error(aclk_event_log_t event_id, const char *file, const char *function, const unsigned long line, const char *fmt, ...);
#define aclk_log_event_error(event_id, fmt, args...) aclk_log_error(event_id, __FILE__, __FUNCTION__, __LINE__, fmt, ##args)

void aclk_log(aclk_event_log_t event_id, const char *file, const char *function, const unsigned long line, const char *fmt, ...);
#define aclk_log_event(event_id, fmt, args...) aclk_log(event_id, __FILE__, __FUNCTION__, __LINE__, fmt, ##args)

#endif /* ACLK_FLIGHT_RECORDER_H */
