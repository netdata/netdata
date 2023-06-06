// SPDX-License-Identifier: GPL-3.0-or-later

/** @file query_test.c
 *  @brief This is the file containing tests for the query API.
 */

#include "query.h"
#include <inttypes.h>
#include <stdlib.h>
#include <uv.h>
#include "logsmanagement_conf.h"
#include "helper.h"
#include "query_test.h"

static uv_loop_t query_thread_uv_loop;
static uv_pipe_t query_data_pipe;

static void pipe_read_cb(uv_stream_t *client, ssize_t nread, const uv_buf_t *buf) {
    if (nread < 0) {
        uv_close((uv_handle_t *)client, NULL);
        return;
    }
    debug(D_LOGS_MANAG, "Read through pipe: %.*s\n", (int) nread, buf->base);

    // Deserialise streamed string
    char *pEnd;
    int log_files_no = strtol(strtok(buf->base, ","), &pEnd, 10);
    logs_query_params_t *query_params = calloc(1, log_files_no * sizeof(logs_query_params_t));
    uv_thread_t *test_execute_query_thread_id = malloc(log_files_no * sizeof(uv_thread_t));
    for (int i = 0; i < log_files_no; i++) {
        query_params[i].start_timestamp = strtoll(strtok(NULL, ","), &pEnd, 10);
        query_params[i].end_timestamp = strtoll(strtok(NULL, ","), &pEnd, 10);
        query_params[i].filename[0] = malloc(100 * sizeof(char));
        query_params[i].filename[0] = strtok(NULL, ",");
        query_params[i].keyword = strtok(NULL, ",");
        size_t buff_size = (size_t)strtoll(strtok(NULL, ","), &pEnd, 10);
        // debug(D_LOGS_MANAG, "size_of_buff in pipe_read_cb(): %zd\n", buff_size);
        query_params[i].results_buff = buffer_create(buff_size, NULL);


        int rc = uv_thread_create(&test_execute_query_thread_id[i], test_execute_query_thread, &query_params[i]);
        if (unlikely(rc)) debug(D_LOGS_MANAG, "Creation of thread failed: %s\n", uv_strerror(rc));
        m_assert(!rc, "Creation of thread failed");
    }

}

static void alloc_cb(uv_handle_t *handle, size_t suggested_size, uv_buf_t *buf) {
    UNUSED(handle);
    buf->base = malloc(suggested_size);
    buf->len = suggested_size;
}

static void connection_cb(uv_stream_t *server, int status) {

    if (status == -1) {
        debug(D_LOGS_MANAG, "uv_listen connection_cb error\n");
        m_assert(0, "uv_listen connection_cb error!");
    }

    debug(D_LOGS_MANAG, "Received connection on " LOGS_MANAGEMENT_STRESS_TEST_PIPENAME "\n");

    uv_pipe_t *client = (uv_pipe_t *)malloc(sizeof(uv_pipe_t));
    uv_pipe_init(&query_thread_uv_loop, client, 0);
    if (uv_accept(server, (uv_stream_t *)client) == 0) {
        if (uv_read_start((uv_stream_t *)client, alloc_cb, pipe_read_cb) != 0) {
            debug(D_LOGS_MANAG, "uv_read_start() error");
            uv_close((uv_handle_t *)&client, NULL);
            m_assert(0, "uv_read_start() error");
        }
    } else {
        uv_close((uv_handle_t *)client, NULL);
    }
}

void remove_pipe(int sig) {
    UNUSED(sig);
    uv_fs_t req;
    uv_fs_unlink(&query_thread_uv_loop, &req, LOGS_MANAGEMENT_STRESS_TEST_PIPENAME, NULL);
    uv_fs_req_cleanup(&req);
    // exit(0);
}

void test_execute_query_thread(void *args) {
    logs_query_params_t query_params = *((logs_query_params_t *)args);
    int rc = 0;
    uv_file file_handle;
    char *buf = NULL;
    uv_fs_t read_req;
    uv_buf_t uv_buf;
    int64_t file_offset = 0;
    size_t results_size_max = query_params.results_buff->size;
    msec_t final_timestamp = query_params.end_timestamp;

    uv_loop_t thread_loop;
    uv_loop_init(&thread_loop);

    // Open log source to use for validation
    uv_fs_t open_req;
    rc = uv_fs_open(&thread_loop, &open_req, query_params.filename[0], O_RDONLY, 0, NULL);
    if (unlikely(rc < 0)) {
        debug(D_LOGS_MANAG, "file_open() error: %s (%d) %s\n", query_params.filename[0], rc, uv_strerror(rc));
        m_assert(rc >= 0, "uv_fs_open() failed");
    } 
    debug(D_LOGS_MANAG, "Opened file: %s\n", query_params.filename[0]);
    file_handle = open_req.result;  // open_req->result of a uv_fs_t is the file descriptor in case of the uv_fs_open
    uv_fs_req_cleanup(&open_req);

    // Run queries and compare results with log file data
    const msec_t start_time = now_realtime_msec();
    msec_t query_start_time, query_total_time = 0;
    while (1) {
        query_start_time = now_realtime_msec();
        (void) execute_logs_manag_query(&query_params);
        query_total_time += (now_realtime_msec() - query_start_time);
        if (query_params.results_buff->len == 0)
            break;

        buf = realloc(buf, query_params.results_buff->len);
        uv_buf = uv_buf_init(buf, query_params.results_buff->len);
        rc = uv_fs_read(&thread_loop, &read_req, file_handle, &uv_buf, 1, file_offset, NULL);
        if (rc < 0) debug(D_LOGS_MANAG, "uv_fs_read() error for %s\n", query_params.filename[0]);
        m_assert(rc >= 0, "uv_fs_read() failed");

        // printf("\n============Comparison results============\n");
        // printf( "%.*s", 
        //         (int) query_params.results_buff->len, query_params.results_buff->buffer);
        // printf("\n============\n");
        // printf( "%.*s", 
        //         (int) query_params.results_buff->len, buf);
        // fflush(stdout);

        /* Do not compare the last char in memcmp, as it can be either '\n' or '\0' */
        rc = memcmp(buf, query_params.results_buff->buffer, query_params.results_buff->len - 1);
        if (rc) debug(D_LOGS_MANAG, "Mismatch between DB and log file data in %s\n", query_params.filename[0]);
        m_assert(!rc, "Mismatch between DB and log file data!");

        file_offset += query_params.results_buff->len;
        debug(D_LOGS_MANAG, "Query file offset %" PRId64 " for %s\n", file_offset, query_params.filename[0]);
        uv_fs_req_cleanup(&read_req);

        // Simulate real query which would do buffer_create() and buffer_free() everytime 
        buffer_free(query_params.results_buff); 
        query_params.results_buff = buffer_create(results_size_max, NULL);
        query_params.start_timestamp = query_params.end_timestamp + 1;
        query_params.end_timestamp = final_timestamp;
    }

#if 1
    // Log filesize should be the same as byte of data read back from the database
    uv_fs_t stat_req;
    rc = uv_fs_stat(&thread_loop, &stat_req, query_params.filename[0], NULL);
    if (rc) {
        debug(D_LOGS_MANAG, "uv_fs_stat() error for %s: (%d) %s\n", query_params.filename[0], rc, uv_strerror(rc));
        m_assert(!rc, "uv_fs_stat() failed");
    } else {
        // Request succeeded; get filesize
        uv_stat_t *statbuf = uv_fs_get_statbuf(&stat_req);
        // debug(D_LOGS_MANAG, "Size of %s: %lldKB\n", query_params.filename, (long long)statbuf->st_size / 1000);
        if (statbuf->st_size != (uint64_t) file_offset){
            debug(D_LOGS_MANAG, "Mismatch between log filesize (%" PRIu64 ") and data size returned from query (%" PRIu64 ") for: %s\n",
                        statbuf->st_size, (uint64_t) file_offset, query_params.filename[0]);
            m_assert(statbuf->st_size == (uint64_t) file_offset, "Mismatch between log filesize and data size in DB!");
        }
        debug(D_LOGS_MANAG, "Log filesize and data size from query match for %s\n", query_params.filename[0]);
    }
    uv_fs_req_cleanup(&stat_req);
#endif

    const msec_t end_time = now_realtime_msec();
    debug(D_LOGS_MANAG, "==============================\n"
                        "Stress test queries for '%s' completed with success!\n"
                        "Total duration: %llums to retrieve and compare %" PRId64 "KB.\n"
                        "Query execution total duration: %llums\n"
                        "==============================",
          query_params.filename[0], end_time - start_time, file_offset / 1000, query_total_time);

    uv_run(&thread_loop, UV_RUN_DEFAULT);
}

void run_stress_test_queries_thread(void *args) {
    UNUSED(args);

    int rc = 0;
    rc = uv_loop_init(&query_thread_uv_loop);
    if (unlikely(rc)) fatal("Failed to initialise query_thread_uv_loop\n");

    if ((rc = uv_pipe_init(&query_thread_uv_loop, &query_data_pipe, 0))) {
        debug(D_LOGS_MANAG, "uv_pipe_init(): %s\n", uv_strerror(rc));
        m_assert(0, "uv_pipe_init() failed");
    }
    // signal(SIGINT, remove_pipe);
    if ((rc = uv_pipe_bind(&query_data_pipe, LOGS_MANAGEMENT_STRESS_TEST_PIPENAME))) {
        debug(D_LOGS_MANAG, "uv_pipe_bind() error %s. Trying again.\n", uv_err_name(rc));
        // Try removing pipe and binding again
        remove_pipe(0);  // Delete pipe if it exists
        // uv_close((uv_handle_t *)&query_data_pipe, NULL);
        rc = uv_pipe_bind(&query_data_pipe, LOGS_MANAGEMENT_STRESS_TEST_PIPENAME);
        debug(D_LOGS_MANAG, "uv_pipe_bind() error %s\n", uv_err_name(rc));
        m_assert(!rc, "uv_pipe_bind() error!");
    }
    if ((rc = uv_listen((uv_stream_t *)&query_data_pipe, 1, connection_cb))) {
        debug(D_LOGS_MANAG, "uv_pipe_listen() error %s\n", uv_err_name(rc));
        m_assert(!rc, "uv_pipe_listen() error!");
    }
    uv_run(&query_thread_uv_loop, UV_RUN_DEFAULT);
    uv_close((uv_handle_t *)&query_data_pipe, NULL);
}
