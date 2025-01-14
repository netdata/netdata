// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ND_LOG_FATAL_H
#define NETDATA_ND_LOG_FATAL_H

#include "libnetdata/common.h"

void netdata_logger_fatal( const char *file, const char *function, unsigned long line, const char *fmt, ... ) NORETURN PRINTFLIKE(4, 5);

#define fatal(args...)   netdata_logger_fatal(__FILE__, __FUNCTION__, __LINE__, ##args)
#define fatal_assert(expr) ((expr) ? (void)(0) : netdata_logger_fatal(__FILE__, __FUNCTION__, __LINE__, "Assertion `%s' failed", #expr))

#ifdef NETDATA_INTERNAL_CHECKS
#define internal_fatal(condition, args...) do { if(unlikely(condition)) netdata_logger_fatal(__FILE__, __FUNCTION__, __LINE__, ##args); } while(0)
#else
#define internal_fatal(args...) debug_dummy()
#endif

#endif //NETDATA_ND_LOG_FATAL_H
