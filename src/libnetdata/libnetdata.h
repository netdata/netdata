// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LIB_H
#define NETDATA_LIB_H 1

# ifdef __cplusplus
extern "C" {
# endif

#include "common.h"
#include "signals/signals.h"
#include "memory/alignment.h"
#include "memory/nd-mallocz.h"
#include "memory/nd-mmap.h"
#include "libnetdata/exit/exit_initiated.h"
#include "log/nd_log-fatal.h"
#include "atomics/atomics.h"

// NETDATA_TRACE_ALLOCATIONS does not work under musl libc, so don't enable it
//#if defined(NETDATA_INTERNAL_CHECKS) && !defined(NETDATA_TRACE_ALLOCATIONS)
//#define NETDATA_TRACE_ALLOCATIONS 1
//#endif

#include "libjudy/judy-malloc.h"
#include "locks/benchmark.h"
#include "locks/benchmark-rw.h"
#include "object-state/object-state.h"
#include "storage-point.h"
#include "paths/paths.h"

int  vsnprintfz(char *dst, size_t n, const char *fmt, va_list args);
int  snprintfz(char *dst, size_t n, const char *fmt, ...) PRINTFLIKE(3, 4);

void json_escape_string(char *dst, const char *src, size_t size);
void json_fix_string(char *s);


extern struct rlimit rlimit_nofile;

char *fgets_trim_len(char *buf, size_t buf_size, FILE *fp, size_t *len);

int verify_netdata_host_prefix(bool log_msg);

char *read_by_filename(const char *filename, long *file_size);
char *find_and_replace(const char *src, const char *find, const char *replace, const char *where);

#define BITS_IN_A_KILOBIT     1000
#define KILOBITS_IN_A_MEGABIT 1000

#include "bitmap/bitmap64.h"

#define COMPRESSION_MAX_CHUNK 0x4000
#define COMPRESSION_MAX_OVERHEAD 128
#define COMPRESSION_MAX_MSG_SIZE (COMPRESSION_MAX_CHUNK - COMPRESSION_MAX_OVERHEAD - 1)
#define PLUGINSD_LINE_MAX (COMPRESSION_MAX_MSG_SIZE - 768)

bool run_command_and_copy_output_to_stdout(const char *command, int max_line_length);
struct web_buffer *run_command_and_get_output_to_buffer(const char *command, int max_line_length);

extern const char *netdata_configured_host_prefix;

// safe includes before O/S specific functions
#include "template-enum.h"
#include "libjudy/vendored/Judy.h"
#include "libjudy/judyl-typed.h"

#include "string/string.h"
#include "buffer/buffer.h"

#include "uuid/uuid.h"
#include "uuid/uuidmap.h"
#include "http/content_type.h"
#include "http/http_access.h"

#include "inlined.h"
#include "parsers/parsers.h"

#include "threads/threads.h"
#include "locks/locks.h"
#include "locks/spinlock.h"
#include "locks/rw-spinlock.h"
#include "completion/completion.h"
#include "libnetdata/locks/waitq.h"
#include "clocks/clocks.h"
#include "simple_pattern/simple_pattern.h"
#include "libnetdata/log/nd_log.h"

#include "socket/security.h"    // must be before windows.h

// this may include windows.h
#include "os/os.h"

#include "socket/socket.h"
#include "socket/nd-sock.h"
#include "socket/nd-poll.h"
#include "socket/listen-sockets.h"
#include "socket/poll-events.h"
#include "socket/connect-to.h"
#include "socket/socket-peers.h"
#include "avl/avl.h"

#include "line_splitter/line_splitter.h"
#include "c_rhash/c_rhash.h"
#include "ringbuffer/ringbuffer.h"
#include "circular_buffer/circular_buffer.h"
#include "buffered_reader/buffered_reader.h"
#include "datetime/iso8601.h"
#include "datetime/rfc3339.h"
#include "datetime/rfc7231.h"
#include "sanitizers/sanitizers.h"

#include "config/config.h"
#include "spawn_server/spawn_server.h"
#include "spawn_server/spawn_popen.h"
#include "procfile/procfile.h"
#include "dictionary/dictionary.h"
#include "dictionary/thread-cache.h"

#include "log/systemd-journal-helpers.h"

#include "eval/eval.h"
#include "statistical/statistical.h"
#include "adaptive_resortable_list/adaptive_resortable_list.h"
#include "url/url.h"
#include "json/json.h"
#include "json/json-c-parser-inline.h"
#include "string/utf8.h"
#include "libnetdata/aral/aral.h"
#include "onewayalloc/onewayalloc.h"
#include "worker_utilization/worker_utilization.h"
#include "yaml.h"
#include "http/http_defs.h"
#include "gorilla/gorilla.h"
#include "facets/facets.h"
#include "functions_evloop/functions_evloop.h"
#include "query_progress/progress.h"

static ALWAYS_INLINE PPvoid_t JudyLFirstThenNext(Pcvoid_t PArray, Word_t * PIndex, bool *first) {
    if(unlikely(*first)) {
        *first = false;
        return JudyLFirst(PArray, PIndex, PJE0);
    }

    return JudyLNext(PArray, PIndex, PJE0);
}

static ALWAYS_INLINE PPvoid_t JudyLLastThenPrev(Pcvoid_t PArray, Word_t * PIndex, bool *first) {
    if(unlikely(*first)) {
        *first = false;
        return JudyLLast(PArray, PIndex, PJE0);
    }

    return JudyLPrev(PArray, PIndex, PJE0);
}

typedef enum {
    TIMING_STEP_INTERNAL = 0,

    TIMING_STEP_BEGIN2_PREPARE,
    TIMING_STEP_BEGIN2_FIND_CHART,
    TIMING_STEP_BEGIN2_PARSE,
    TIMING_STEP_BEGIN2_ML,
    TIMING_STEP_BEGIN2_PROPAGATE,
    TIMING_STEP_BEGIN2_STORE,

    TIMING_STEP_SET2_PREPARE,
    TIMING_STEP_SET2_LOOKUP_DIMENSION,
    TIMING_STEP_SET2_PARSE,
    TIMING_STEP_SET2_ML,
    TIMING_STEP_SET2_PROPAGATE,
    TIMING_STEP_RRDSET_STORE_METRIC,
    TIMING_STEP_DBENGINE_FIRST_CHECK,
    TIMING_STEP_DBENGINE_CHECK_DATA,
    TIMING_STEP_DBENGINE_PACK,
    TIMING_STEP_DBENGINE_PAGE_FIN,
    TIMING_STEP_DBENGINE_MRG_UPDATE,
    TIMING_STEP_DBENGINE_PAGE_ALLOC,
    TIMING_STEP_DBENGINE_CREATE_NEW_PAGE,
    TIMING_STEP_DBENGINE_FLUSH_PAGE,
    TIMING_STEP_SET2_STORE,

    TIMING_STEP_END2_PREPARE,
    TIMING_STEP_END2_PUSH_V1,
    TIMING_STEP_END2_ML,
    TIMING_STEP_END2_RRDSET,
    TIMING_STEP_END2_PROPAGATE,
    TIMING_STEP_END2_STORE,

    TIMING_STEP_FREEIPMI_CTX_CREATE,
    TIMING_STEP_FREEIPMI_DSR_CACHE_DIR,
    TIMING_STEP_FREEIPMI_SENSOR_CONFIG_FILE,
    TIMING_STEP_FREEIPMI_SENSOR_READINGS_BY_X,
    TIMING_STEP_FREEIPMI_READ_record_id,
    TIMING_STEP_FREEIPMI_READ_sensor_number,
    TIMING_STEP_FREEIPMI_READ_sensor_type,
    TIMING_STEP_FREEIPMI_READ_sensor_name,
    TIMING_STEP_FREEIPMI_READ_sensor_state,
    TIMING_STEP_FREEIPMI_READ_sensor_units,
    TIMING_STEP_FREEIPMI_READ_sensor_bitmask_type,
    TIMING_STEP_FREEIPMI_READ_sensor_bitmask,
    TIMING_STEP_FREEIPMI_READ_sensor_bitmask_strings,
    TIMING_STEP_FREEIPMI_READ_sensor_reading_type,
    TIMING_STEP_FREEIPMI_READ_sensor_reading,
    TIMING_STEP_FREEIPMI_READ_event_reading_type_code,
    TIMING_STEP_FREEIPMI_READ_record_type,
    TIMING_STEP_FREEIPMI_READ_record_type_class,
    TIMING_STEP_FREEIPMI_READ_sel_state,
    TIMING_STEP_FREEIPMI_READ_event_direction,
    TIMING_STEP_FREEIPMI_READ_event_type_code,
    TIMING_STEP_FREEIPMI_READ_event_offset_type,
    TIMING_STEP_FREEIPMI_READ_event_offset,
    TIMING_STEP_FREEIPMI_READ_event_offset_string,
    TIMING_STEP_FREEIPMI_READ_manufacturer_id,

    TIMING_STEP_DBENGINE_EVICT_LOCK,
    TIMING_STEP_DBENGINE_EVICT_SELECT,
    TIMING_STEP_DBENGINE_EVICT_SELECT_PAGE,
    TIMING_STEP_DBENGINE_EVICT_RELOCATE_PAGE,
    TIMING_STEP_DBENGINE_EVICT_SORT,
    TIMING_STEP_DBENGINE_EVICT_DEINDEX,
    TIMING_STEP_DBENGINE_EVICT_DEINDEX_PAGE,
    TIMING_STEP_DBENGINE_EVICT_FINISHED,
    TIMING_STEP_DBENGINE_EVICT_FREE_LOOP,
    TIMING_STEP_DBENGINE_EVICT_FREE_PAGE,
    TIMING_STEP_DBENGINE_EVICT_FREE_ATOMICS,
    TIMING_STEP_DBENGINE_EVICT_FREE_CB,
    TIMING_STEP_DBENGINE_EVICT_FREE_ATOMICS2,
    TIMING_STEP_DBENGINE_EVICT_FREE_ARAL,
    TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_DATA,
    TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_ARAL,
    TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_TIER1_ARAL,
    TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_GLIVE,
    TIMING_STEP_DBENGINE_EVICT_FREE_MAIN_PGD_GWORKER,
    TIMING_STEP_DBENGINE_EVICT_FREE_OPEN,
    TIMING_STEP_DBENGINE_EVICT_FREE_EXTENT,

    // terminator
    TIMING_STEP_MAX,
} TIMING_STEP;

typedef enum {
    TIMING_ACTION_INIT,
    TIMING_ACTION_STEP,
    TIMING_ACTION_FINISH,
} TIMING_ACTION;

#ifdef NETDATA_TIMING_REPORT
#define timing_init() timing_action(TIMING_ACTION_INIT, TIMING_STEP_INTERNAL)
#define timing_step(step) timing_action(TIMING_ACTION_STEP, step)
#define timing_report() timing_action(TIMING_ACTION_FINISH, TIMING_STEP_INTERNAL)

#define timing_dbengine_evict_init() timing_action(TIMING_ACTION_INIT, TIMING_STEP_INTERNAL)
#define timing_dbengine_evict_step(step) timing_action(TIMING_ACTION_STEP, step)
#define timing_dbengine_evict_report() timing_action(TIMING_ACTION_FINISH, TIMING_STEP_INTERNAL)
#else
#define timing_init() debug_dummy()
#define timing_step(step) debug_dummy()
#define timing_report() debug_dummy()

#define timing_dbengine_evict_init() debug_dummy()
#define timing_dbengine_evict_step(step) debug_dummy()
#define timing_dbengine_evict_report() debug_dummy()
#endif


void timing_action(TIMING_ACTION action, TIMING_STEP step);

int hash256_string(const unsigned char *string, size_t size, char *hash);

extern bool unittest_running;

bool rrdr_relative_window_to_absolute(time_t *after, time_t *before, time_t now);
bool rrdr_relative_window_to_absolute_query(time_t *after, time_t *before, time_t *now_ptr, bool unittest);

int netdata_base64_decode(unsigned char *out, const unsigned char *in, int in_len);
int netdata_base64_encode(unsigned char *encoded, const unsigned char *input, size_t input_size);

// --------------------------------------------------------------------------------------------------------------------

static inline void freez_charp(char **p) {
    freez(*p);
}

static inline void freez_const_charp(const char **p) {
    freez((void *)*p);
}

#define CLEAN_CONST_CHAR_P _cleanup_(freez_const_charp) const char
#define CLEAN_CHAR_P _cleanup_(freez_charp) char

// --------------------------------------------------------------------------------------------------------------------
// automatic cleanup function, instead of pthread pop/push

// volatile: Tells the compiler that the variable defined might be accessed in unexpected ways
// (e.g., by the cleanup function). This prevents it from being optimized out.
#define CLEANUP_FUNCTION_REGISTER(func) volatile void * __attribute__((cleanup(func)))

static inline void *CLEANUP_FUNCTION_GET_PTR(void *pptr) {
    void *ret;
    void **p = (void **)pptr;
    if(p) {
        ret = *p;
        *p = NULL; // use it only once - this will prevent using it again

        if(!ret)
            nd_log(NDLS_DAEMON, NDLP_ERR, "cleanup function called multiple times!");
    }
    else {
        nd_log(NDLS_DAEMON, NDLP_ERR, "cleanup function called with NULL pptr!");
        ret = NULL;
    }

    return ret;
}

// --------------------------------------------------------------------------------------------------------------------

# ifdef __cplusplus
}
# endif

#endif // NETDATA_LIB_H
