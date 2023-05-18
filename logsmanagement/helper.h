/** @file helper.h
 *  @brief Includes helper functions for the Logs Project
 *
 *  @author Dimitris Pantazis
 */

#ifndef HELPER_H_
#define HELPER_H_

#include "libnetdata/libnetdata.h"
#include <assert.h>
#include <stdarg.h>
#include <string.h>
#include <sys/time.h>

#define LOGS_MANAG_STR_HELPER(x) #x
#define LOGS_MANAG_STR(x) LOGS_MANAG_STR_HELPER(x)

#define MAX_LOGS_MANAG_LOG_MSG_LEN 256

typedef enum { LOGS_MANAG_ERROR,
               LOGS_MANAG_WARNING,
               LOGS_MANAG_INFO,
               LOGS_MANAG_DEBUG } Log_level;

#ifndef m_assert
#if LOGS_MANAG_DEBUG_LEV                                             // Disable m_assert if production release
#define m_assert(expr, msg) assert(((void)(msg), (expr))) /**< Custom assert function that prints out failure message */
#else
#define m_assert(expr, msg) \
    do {                    \
    } while (0)
#endif  // LOGS_MANAG_DEBUG_LEV
#endif  // m_assert

/* Test if a timestamp is within a valid range 
 * 1649175852000 equals Tuesday, 5 April 2022 16:24:12, 
 * 2532788652000 equals Tuesday, 5 April 2050 16:24:12
 */
#define TEST_MS_TIMESTAMP_VALID(x) (((x) > 1649175852000 && (x) < 2532788652000)? 1:0)

#define TIMESTAMP_MS_STR_SIZE sizeof(1649175852000)

// Portable thread local, see https://stackoverflow.com/questions/18298280/how-to-declare-a-variable-as-thread-local-portably
#ifndef thread_local
#if __STDC_VERSION__ >= 201112 && !defined __STDC_NO_THREADS__
#define thread_local _Thread_local
#elif   defined _WIN32 && ( \
        defined _MSC_VER || \
        defined __ICL || \
        defined __DMC__ || \
        defined __BORLANDC__ )
#define thread_local __declspec(thread) 
/* note that ICC (linux) and Clang are covered by __GNUC__ */
#elif   defined __GNUC__ || \
        defined __SUNPRO_C || \
        defined __xlC__
#define thread_local __thread
#else
#error "Cannot define thread_local"
#endif
#endif // thread_local

#ifdef ENABLE_LOGSMANAGEMENT_TESTS
#define UNIT_STATIC
#else 
#define UNIT_STATIC static
#endif // ENABLE_LOGSMANAGEMENT_TESTS 

#ifndef COMPILE_TIME_ASSERT // https://stackoverflow.com/questions/3385515/static-assert-in-c
#define STATIC_ASSERT(COND,MSG) typedef char static_assertion_##MSG[(!!(COND))*2-1]
// token pasting madness:
#define COMPILE_TIME_ASSERT3(X,L) STATIC_ASSERT(X,static_assertion_at_line_##L)
#define COMPILE_TIME_ASSERT2(X,L) COMPILE_TIME_ASSERT3(X,L)
#define COMPILE_TIME_ASSERT(X)    COMPILE_TIME_ASSERT2(X,__LINE__)
#endif  // COMPILE_TIME_ASSERT

#define BIT_SET(a, b) ((a) |= (1ULL << (b)))
#define BIT_CLEAR(a, b) ((a) &= ~(1ULL << (b)))
#define BIT_CHECK(a, b) (!!((a) & (1ULL << (b))))  // '!!' to make sure this returns 0 or 1

// Include for sleep_ms()
#ifdef _WIN32
#include <Windows.h>
#else
#include <unistd.h>
#endif  // _WIN32

extern uv_loop_t *main_loop; 

/**
 * @brief Extract file_basename from full file path
 * @param path String containing the full path.
 * @return Pointer to the file_basename string
 */
static inline char *get_basename(const char *const path) {
    if(!path) return NULL;
    char *s = strrchr(path, '/');
    if (!s)
        return strdupz(path);
    else
        return strdupz(s + 1);
}


typedef enum {
    STR2XX_SUCCESS = 0,
    STR2XX_OVERFLOW,
    STR2XX_UNDERFLOW,
    STR2XX_INCONVERTIBLE
} str2xx_errno;

/* Convert string s to int out.
 * https://stackoverflow.com/questions/7021725/how-to-convert-a-string-to-integer-in-c
 *
 * @param[out] out The converted int. Cannot be NULL.
 * @param[in] s Input string to be converted.
 *
 *     The format is the same as strtol,
 *     except that the following are inconvertible:
 *     - empty string
 *     - leading whitespace
 *     - any trailing characters that are not part of the number
 *     Cannot be NULL.
 *
 * @param[in] base Base to interpret string in. Same range as strtol (2 to 36).
 * @return Indicates if the operation succeeded, or why it failed.
 */
static inline str2xx_errno str2int(int *out, char *s, int base) {
    char *end;
    if (unlikely(s[0] == '\0' || isspace(s[0]))){
        // debug(D_LOGS_MANAG, "str2int error: STR2XX_INCONVERTIBLE 1");
        // m_assert(0, "str2int error: STR2XX_INCONVERTIBLE");
        return STR2XX_INCONVERTIBLE;
    }
    errno = 0;
    long l = strtol(s, &end, base);
    /* Both checks are needed because INT_MAX == LONG_MAX is possible. */
    if (unlikely(l > INT_MAX || (errno == ERANGE && l == LONG_MAX))){
        // debug(D_LOGS_MANAG, "str2int error: STR2XX_OVERFLOW");
        // m_assert(0, "str2int error: STR2XX_OVERFLOW");
        return STR2XX_OVERFLOW;
    }
    if (unlikely(l < INT_MIN || (errno == ERANGE && l == LONG_MIN))){
        // debug(D_LOGS_MANAG, "str2int error: STR2XX_UNDERFLOW");
        // m_assert(0, "str2int error: STR2XX_UNDERFLOW");
        return STR2XX_UNDERFLOW;
    }
    if (unlikely(*end != '\0')){
        // debug(D_LOGS_MANAG, "str2int error: STR2XX_INCONVERTIBLE 2");
        // m_assert(0, "str2int error: STR2XX_INCONVERTIBLE 2");
        return STR2XX_INCONVERTIBLE;
    }
    *out = l;
    return STR2XX_SUCCESS;
}

static inline str2xx_errno str2float(float *out, char *s) {
    char *end;
    if (unlikely(s[0] == '\0' || isspace(s[0]))){ 
        // debug(D_LOGS_MANAG, "str2float error: STR2XX_INCONVERTIBLE 1\n");
        // m_assert(0, "str2float error: STR2XX_INCONVERTIBLE");
        return STR2XX_INCONVERTIBLE;
    }
    errno = 0;
    float f = strtof(s, &end);
    /* Both checks are needed because INT_MAX == LONG_MAX is possible. */
    if (unlikely((errno == ERANGE && f == HUGE_VALF))){
        // debug(D_LOGS_MANAG, "str2float error: STR2XX_OVERFLOW\n");
        // m_assert(0, "str2float error: STR2XX_OVERFLOW");
        return STR2XX_OVERFLOW;
    }
    if (unlikely((errno == ERANGE && f == -HUGE_VALF))){
        // debug(D_LOGS_MANAG, "str2float error: STR2XX_UNDERFLOW\n");
        // m_assert(0, "str2float error: STR2XX_UNDERFLOW");
        return STR2XX_UNDERFLOW;
    }
    if (unlikely((*end != '\0'))){
        // debug(D_LOGS_MANAG, "str2float error: STR2XX_INCONVERTIBLE 2\n");
        // m_assert(0, "str2float error: STR2XX_INCONVERTIBLE");
        return STR2XX_INCONVERTIBLE;
    }
    *out = f;
    return STR2XX_SUCCESS;
}

/**
 * @brief Read last line of *filename, up to max_line_width characters.
 * @details This function should be used carefully as it is not the most
 * efficient one. But it is a quick-n-dirty way of reading the last line
 * of a file.
 * @param[in] filename File to be read.
 * @param[in] max_line_width Positive int indicating the max line width to be read. 
 * If a line is longer than that, it will be truncated. If zero or negative, a
 * default value will be used instead.
 * @return Pointer to a string holding the line that was read, or NULL if error.
 */
static inline char *read_last_line(const char *filename, int max_line_width){
    const int default_max_line_width = 4 * 1024;
    uv_fs_t stat_req, open_req, read_req;
    int64_t start_pos, end_pos;
    uv_file file_handle;
    uv_buf_t uvBuf;
    char *buff;
    int rc, line_pos, found_ln, bytes_read;

    if(max_line_width <= 0) max_line_width = default_max_line_width;

    rc = uv_fs_stat(NULL, &stat_req, filename, NULL);
    if (unlikely(rc)) {
        error("uv_fs_stat() error for %s: (%d) %s\n", filename, rc, uv_strerror(rc));
        m_assert(!rc, "uv_fs_stat() failed during read_last_line()");
        uv_fs_req_cleanup(&stat_req);
        return NULL;
    }
    uv_stat_t *statbuf = uv_fs_get_statbuf(&stat_req);
    end_pos = statbuf->st_size;
    uv_fs_req_cleanup(&stat_req);

    if(end_pos == 0) return NULL;
    start_pos = end_pos - max_line_width;
    if(start_pos < 0) start_pos = 0;

    rc = uv_fs_open(NULL, &open_req, filename, O_RDONLY, 0, NULL);
    if (unlikely(rc < 0)) {
        error("uv_fs_open() error: %s (%d) %s\n",filename, rc, uv_strerror(rc));
        uv_fs_req_cleanup(&open_req);
        return NULL;
    } 
    file_handle = open_req.result;  // open_req->result of a uv_fs_t is the file descriptor in case of the uv_fs_open
    uv_fs_req_cleanup(&open_req);

    buff = callocz(1, (size_t) (end_pos - start_pos + 1) * sizeof(char));
    uvBuf = uv_buf_init(buff, (unsigned int) (end_pos - start_pos + 1));
    rc = uv_fs_read(NULL, &read_req, file_handle, &uvBuf, 1, start_pos, NULL);
    uv_fs_req_cleanup(&read_req);
    if (unlikely(rc < 0)){ 
        error("uv_fs_read() error for %s (%d) %s\n", filename, rc, uv_strerror(rc));
        return NULL;
    }

    buff[rc] = '\0';

    bytes_read = rc;
    line_pos = found_ln = 0;

    for(int i = bytes_read - 2; i >= 0; i--){
        char ch = buff[i];
        if (ch == '\n'){
            found_ln = 1;
            line_pos = i;
            break;
        }
    }
    if(found_ln){
        char *line = callocz(1, (size_t) (bytes_read - line_pos - 1) * sizeof(char));
        memcpy(line, &buff[line_pos + 1], (size_t) (bytes_read - line_pos - 2));
        freez(buff);
        return line;
    }
    if(start_pos == 0){
        return buff;
    }

    // Should not get here - error line too long
    freez(buff);
    return NULL;
}

#endif  // HELPER_H_
