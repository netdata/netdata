// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef LIBNDINC_H
#define LIBNDINC_H 1

# ifdef __cplusplus
extern "C" {
# endif

#include "os.h"
#include "storage_number/storage_number.h"
#include "threads/threads.h"
#include "buffer/buffer.h"
#include "locks/locks.h"
#include "circular_buffer/circular_buffer.h"
#include "avl/avl.h"
#include "inlined.h"
#include "clocks/clocks.h"
#include "popen/popen.h"
#include "simple_pattern/simple_pattern.h"

#ifdef ENABLE_HTTPS
# include "socket/security.h"
#endif

#include "socket/socket.h"
#include "config/appconfig.h"
#include "log/log.h"
#include "procfile/procfile.h"
#include "dictionary/dictionary.h"

#if !defined(__cplusplus) && defined(HAVE_LIBBPF)
#include "ebpf/ebpf.h"
#endif

#include "eval/eval.h"
#include "statistical/statistical.h"
#include "adaptive_resortable_list/adaptive_resortable_list.h"
#include "url/url.h"
#include "json/json.h"
#include "health/health.h"
#include "string/utf8.h"

# ifdef __cplusplus
}
# endif

#endif // LIBNDINC_H
