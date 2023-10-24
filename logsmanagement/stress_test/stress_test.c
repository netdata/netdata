// SPDX-License-Identifier: GPL-3.0-or-later

/** @file stress_test.c
 *  @brief Black-box stress testing of Netdata Logs Management
 */

#include <assert.h>
#include <inttypes.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/time.h>
#include <time.h>
#include <unistd.h>
#include <uv.h>
#include "../defaults.h"

#include "stress_test.h"

#define SIMULATED_LOGS_DIR "/tmp/netdata_log_management_stress_test_data"
#define LOG_ROTATION_CMD "logrotate --force logrotate.conf -s /tmp/netdata_log_management_stress_test_data/logrotate_status"
#define CSV_DELIMITER " "
#define USE_LTSV_FORMAT 0

#define MS_IN_S 1000
#define US_IN_S 1000000

#define NO_OF_FIELDS 10

#ifdef _WIN32
# define PIPENAME "\\\\?\\pipe\\netdata-logs-stress-test"
#else
# define PIPENAME "/tmp/netdata-logs-stress-test"
#endif // _WIN32

uv_process_t child_req;
uv_process_options_t options;
size_t max_msg_len;
static int log_files_no;
static volatile int log_rotated = 0;

static char **all_fields_arr[NO_OF_FIELDS];
static int all_fields_arr_sizes[NO_OF_FIELDS];

static char *vhosts_ports[] = {
    "testhost.host:17",
    "invalidhost&%$:80",
    "testhost12.host:80",
    "testhost57.host:19999",
    "testhost111.host:77777",
    NULL
};

static char *vhosts[] = {
    "testhost.host",
    "invalidhost&%$",
    "testhost12.host",
    "testhost57.host",
    "testhost111.host",
    NULL
};

static char *ports[] = {
    "17",
    "80",
    "123",
    "8080",
    "19999",
    "77777",
    NULL
};

static char *req_clients[] = {
    "192.168.15.14",
    "192.168.2.1",
    "188.133.132.15",
    "156.134.132.15",
    "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
    "8501:0ab8:85a3:0000:0000:4a5d:0370:5213",
    "garbageAddress",
    NULL
};

static char *req_methods[] = {
    "GET",
    "POST",
    "UPDATE",
    "DELETE",
    "PATCH",
    "PUT",
    "INVALIDMETHOD",
    NULL
};

static char *resp_codes[] = {
    "5",
    "200",
    "202",
    "404",
    "410",
    "1027",
    NULL
};

static char *req_protos[] = {
    "HTTP/1",
    "HTTP/1.0",
    "HTTP/2",
    "HTTP/3",
    NULL
};

static char *req_sizes[] = {
    "236",
    "635",
    "954",
    "-",
    NULL
};

static char *resp_sizes[] = {
    "128",
    "452",
    "1056",
    "-",
    NULL
};

static char *ssl_protos[] = {
    "TLSv1",
    "TLSv1.1",
    "TLSv1.2",
    "TLSv1.3",
    "SSLv3",
    "-",
    NULL
};

static char *ssl_ciphers[] = {
    "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
    "TLS_PSK_WITH_AES_128_CCM_8",
    "ECDHE-RSA-AES128-GCM-SHA256",
    "TLS_RSA_WITH_DES_CBC_SHA",
    "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256",
    "invalid_SSL_cipher_suite",
    "invalidSSLCipher",
    NULL
};



// "host:testhost.host\tport:80\treq_client:192.168.15.14\treq_method:\"GET\"\tresp_code:202\treq_proto:HTTP/1\treq_size:635\tresp_size:-\tssl_proto:TLSv1\tssl_cipher:TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",


// TODO: Include query.h instead of copy-pasting
typedef struct db_query_params {
    msec_t start_timestamp;
    msec_t end_timestamp;
    char *filename;
    char *keyword;
    char *results;
    size_t results_size;
} logs_query_params_t;

size_t get_local_time(char *buf, size_t max_buf_size){
    time_t rawtime;
    struct tm *info;
    time( &rawtime );
#if USE_LTSV_FORMAT
    return strftime (buf, max_buf_size, "time:[%d/%b/%Y:%H:%M:%S %z]",localtime( &rawtime ));
#else
    return strftime (buf, max_buf_size, "[%d/%b/%Y:%H:%M:%S %z]",localtime( &rawtime ));
#endif
}

static void produce_logs(void *arg) {
    msec_t runtime;
    msec_t start_time = now_realtime_msec();
    int log_no = *((int *)arg);
    int rc = 0;
    long int msgs_written = 0;
    uv_file file_handle;
    uv_buf_t uv_buf;
    char *buf = malloc(max_msg_len + 100);

    size_t buf_size;
    uv_fs_t write_req;

    uv_loop_t loop;
    uv_loop_init(&loop);

    char log_filename[100];
    sprintf(log_filename, "%s/%d.log", SIMULATED_LOGS_DIR, log_no);

    uv_fs_t open_req;
    rc = uv_fs_open(&loop, &open_req, log_filename, O_WRONLY | O_CREAT | O_TRUNC, 0777, NULL);
    if (rc < 0) {
        fprintf(stderr, "[STRESS_TEST] file_open() error: %s (%d) %s\n", log_filename, rc, uv_strerror(rc));
    } else {
        fprintf(stderr, "[STRESS_TEST] Opened file: %s\n", log_filename);
        file_handle = open_req.result;  // open_req->result of a uv_fs_t is the file descriptor in case of the uv_fs_open
    }
    uv_fs_req_cleanup(&open_req);

    sleep(DELAY_OPEN_TO_WRITE_SEC);

    fprintf(stderr, "[STRESS_TEST] Start logging: %s\n", log_filename);

    int applied_close_open = 0;
    while (msgs_written < TOTAL_MSGS_PER_SOURCE) {

        size_t msg_timestamp_len = 50;
        msg_timestamp_len = get_local_time(buf, msg_timestamp_len);
        buf_size = msg_timestamp_len;

        for(int i = 0; i < NO_OF_FIELDS; i++){
            strcpy(&buf[buf_size++], CSV_DELIMITER);
            int arr_item_off = rand() % all_fields_arr_sizes[i];
            size_t arr_item_len = strlen(all_fields_arr[i][arr_item_off]);
            memcpy(&buf[buf_size], all_fields_arr[i][arr_item_off], arr_item_len);
            buf_size += arr_item_len;
        }

        buf[buf_size] = '\n';

        uv_buf = uv_buf_init(buf, buf_size + 1);
        uv_fs_write(&loop, &write_req, file_handle, &uv_buf, 1, -1, NULL);
        msgs_written++;
        if(!(msgs_written % 1000000)) fprintf(stderr, "[STRESS_TEST] Wrote %" PRId64 " messages to %s\n", msgs_written, log_filename);
        if(log_rotated && !applied_close_open) {
            uv_fs_t close_req;
            rc = uv_fs_close(&loop, &close_req, file_handle, NULL);
            if(rc) {
                fprintf(stderr, "[STRESS_TEST] file_close() error: %s (%d) %s\n", log_filename, rc, uv_strerror(rc));
                assert(0);
            }
            uv_fs_req_cleanup(&close_req);

            rc = uv_fs_open(&loop, &open_req, log_filename, O_WRONLY | O_CREAT | O_TRUNC , 0777, NULL);
            if (rc < 0) {
                fprintf(stderr, "[STRESS_TEST] file_open() error: %s (%d) %s\n", log_filename, rc, uv_strerror(rc));
                assert(0);
            } else {
                fprintf(stderr, "[STRESS_TEST] Rotated file: %s\n", log_filename);
                file_handle = open_req.result;  // open_req->result of a uv_fs_t is the file descriptor in case of the uv_fs_open
            }
            uv_fs_req_cleanup(&open_req);

            applied_close_open = 1;
            fflush(stderr);
        }
#if DELAY_BETWEEN_MSG_WRITE /**< Sleep delay (in us) in between consequent messages writes to a file **/
        usleep(DELAY_BETWEEN_MSG_WRITE);
#endif
    }

    runtime = now_realtime_msec() - start_time - DELAY_OPEN_TO_WRITE_SEC * MS_IN_S;
    fprintf(stderr, "[STRESS_TEST] It took %" PRIu64 "ms to write %" PRId64 " log records in %s (%" PRId64 "k msgs/s))\n. ",
            runtime, msgs_written, log_filename, msgs_written / runtime);
}

static void log_rotate(void *arg){
    uv_sleep((DELAY_OPEN_TO_WRITE_SEC + LOG_ROTATE_AFTER_SEC) * MS_IN_S);
    assert(system(LOG_ROTATION_CMD) != -1);
    log_rotated = 1;
    fprintf(stderr, "[STRESS_TEST] Rotate log sources\n");
    fflush(stderr);
}

static void connect_cb(uv_connect_t* req, int status){
    int rc = 0;
    if(status < 0){
        fprintf(stderr, "[STRESS_TEST] Failed to connect to pipe!\n");
        exit(-1);
    }
    else
        fprintf(stderr, "[STRESS_TEST] Connection to pipe successful!\n");

    uv_write_t write_req; 
    write_req.data = req->handle;
    
    // Serialise logs_query_params_t
    char *buf = calloc(100 * log_files_no, sizeof(char));
    sprintf(buf, "%d", log_files_no);
    for(int i = 0; i < log_files_no ; i++){
        sprintf(&buf[strlen(buf)], ",0,2147483646000," SIMULATED_LOGS_DIR "/%d.log,%s,%zu", i, " ", (size_t) MAX_LOG_MSG_SIZE);
    }
    fprintf(stderr, "[STRESS_TEST] Serialised DB query params: %s\n", buf);

    // Write to pipe
    uv_buf_t uv_buf = uv_buf_init(buf, strlen(buf));
    rc = uv_write(&write_req, (uv_stream_t *) req->handle, &uv_buf, 1, NULL);
    if (rc) {
        fprintf(stderr, "[STRESS_TEST] uv_write() error: %s\n", uv_strerror(rc));
        uv_close((uv_handle_t *) req->handle, NULL);
        exit(-1);
    }

#if 1
    uv_shutdown_t shutdown_req;
    rc = uv_shutdown(&shutdown_req, (uv_stream_t *) req->handle, NULL);
    if (rc) {
        fprintf(stderr, "[STRESS_TEST] uv_shutdown() error: %s\n", uv_strerror(rc));
        uv_close((uv_handle_t *) req->handle, NULL);
        exit(-1);
    }
#endif 
    
}

int main(int argc, const char *argv[]) {
    fprintf(stdout, "*****************************************************************************\n"
                    "%-15s %40s\n",
                    "* [STRESS_TEST] Starting stress_test", "*");

    srand(time(NULL));

    all_fields_arr[0] = vhosts;
    all_fields_arr[1] = ports;
    all_fields_arr[2] = req_clients;
    all_fields_arr[3] = req_methods;
    all_fields_arr[4] = resp_codes;
    all_fields_arr[5] = req_protos;
    all_fields_arr[6] = req_sizes;
    all_fields_arr[7] = resp_sizes;
    all_fields_arr[8] = ssl_protos;
    all_fields_arr[9] = ssl_ciphers;

    for (int i = 0; i < NO_OF_FIELDS; i++){
        char **arr = all_fields_arr[i];
        int arr_size = 0;
        size_t max_item_len = 0;
        while(arr[arr_size] != NULL){
            size_t item_len = strlen(arr[arr_size]);
            if(item_len > max_item_len) max_item_len = item_len;
            arr_size++;
        }
        max_msg_len += max_item_len;
        all_fields_arr_sizes[i] = arr_size;
    }

    
    char *ptr;
    log_files_no = NUM_LOG_SOURCES;
    fprintf(stdout, "*****************************************************************************\n"
                    "%-15s%42s %-10u%9s\n"
                    "%-15s%42s %-10u%9s\n" 
                    "%-15s%42s %-10u%9s\n"
                    "%-15s%42s %-10u%9s\n"
                    "%-15s%42s %-10u%9s\n"
                    "%-15s%42s %-10u%9s\n"
                    "*****************************************************************************\n",
                    "* [STRESS_TEST]", "Number of log sources to simulate:", log_files_no, "file *",
                    "* [STRESS_TEST]", "Total log records to produce per source:", TOTAL_MSGS_PER_SOURCE, "records *",
                    "* [STRESS_TEST]", "Delay between log record write to file:", DELAY_BETWEEN_MSG_WRITE, "us *",
                    "* [STRESS_TEST]", "Log sources to rotate via create after:", LOG_ROTATE_AFTER_SEC, "s *",
                    "* [STRESS_TEST]", "Queries to be executed after:", QUERIES_DELAY,  "s *", 
                    "* [STRESS_TEST]", "Delay until start writing logs:", DELAY_OPEN_TO_WRITE_SEC,  "s *");

    /* Start threads that produce log messages */
    uv_thread_t *log_producer_threads = malloc(log_files_no * sizeof(uv_thread_t));
    int *log_producer_thread_no = malloc(log_files_no * sizeof(int));
    for (int i = 0; i < log_files_no; i++) {
        fprintf(stderr, "[STRESS_TEST] Starting up log producer for %d.log\n", i);
        log_producer_thread_no[i] = i;
        assert(!uv_thread_create(&log_producer_threads[i], produce_logs, &log_producer_thread_no[i]));
    }

    uv_thread_t *log_rotate_thread = malloc(sizeof(uv_thread_t));
    assert(!uv_thread_create(log_rotate_thread, log_rotate, NULL));

    for (int j = 0; j < log_files_no; j++) {
        uv_thread_join(&log_producer_threads[j]);
    }

    sleep(QUERIES_DELAY); // Give netdata-logs more than LOG_FILE_READ_INTERVAL to ensure the entire log file has been read.

    uv_pipe_t query_data_pipe;
    uv_pipe_init(uv_default_loop(), &query_data_pipe, 1);
    uv_connect_t connect_req;
    uv_pipe_connect(&connect_req, &query_data_pipe, PIPENAME, connect_cb);

    uv_run(uv_default_loop(), UV_RUN_DEFAULT);

    uv_close((uv_handle_t *) &query_data_pipe, NULL);
}
