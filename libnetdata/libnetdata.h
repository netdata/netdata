// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LIB_H
#define NETDATA_LIB_H 1

#ifdef __cplusplus
extern "C" {
#endif

#include "sysinc.h"

#include "inlined.h"
#include "utils.h"
#include "os.h"

#include "adaptive_resortable_list/adaptive_resortable_list.h"
#include "avl/avl.h"
#include "buffer/buffer.h"
#include "circular_buffer/circular_buffer.h"
#include "clocks/clocks.h"
#include "config/appconfig.h"
#include "dictionary/dictionary.h"

#if defined(HAVE_LIBBPF) && !defined(__cplusplus)
#include "ebpf/ebpf.h"
#endif

#include "eval/eval.h"
#include "health/health.h"
#include "json/json.h"
#include "locks/locks.h"
#include "log/log.h"
#include "popen/popen.h"
#include "procfile/procfile.h"
#include "simple_pattern/simple_pattern.h"

#ifdef ENABLE_HTTPS
# include "socket/security.h"
#endif

#include "socket/socket.h"
#include "statistical/statistical.h"
#include "storage_number/storage_number.h"
#include "string/utf8.h"
#include "threads/threads.h"
#include "url/url.h"

# ifdef __cplusplus
}
# endif

#endif // NETDATA_LIB_H
