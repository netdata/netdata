// SPDX-License-Identifier: GPL-3.0-or-later

/** @file unit_test.h
 *  @brief Includes unit tests for the Logs Management project
 */

#include "unit_test.h"
#include <stdlib.h>
#include <stdio.h>

#ifndef __USE_XOPEN_EXTENDED
#define __USE_XOPEN_EXTENDED
#endif

#include <ftw.h>
#include <unistd.h>
#include "../circular_buffer.h"
#include "../helper.h"
#include "../logsmanag_config.h"
#include "../parser.h"
#include "../query.h"
#include "../db_api.h"

static int old_stdout = STDOUT_FILENO;
static int old_stderr = STDERR_FILENO;

#define SUPRESS_STDX(stream_no)                                                     \
{                                                                                   \
    if(stream_no == STDOUT_FILENO)                                                  \
        old_stdout = dup(old_stdout);                                               \
    else                                                                            \
        old_stderr = dup(old_stderr);                                               \
    if(!freopen("/dev/null", "w", stream_no == STDOUT_FILENO ? stdout : stderr))    \
        exit(-1);                                                                   \
}

#define UNSUPRESS_STDX(stream_no)                                                   \
{                                                                                   \
    fclose(stream_no == STDOUT_FILENO ? stdout : stderr);                           \
    if(stream_no == STDOUT_FILENO)                                                  \
        stdout = fdopen(old_stdout, "w");                                           \
    else                                                                            \
        stderr = fdopen(old_stderr, "w");                                           \
}

#define SUPRESS_STDOUT()    SUPRESS_STDX(STDOUT_FILENO)
#define SUPRESS_STDERR()    SUPRESS_STDX(STDERR_FILENO)
#define UNSUPRESS_STDOUT()  UNSUPRESS_STDX(STDOUT_FILENO)
#define UNSUPRESS_STDERR()  UNSUPRESS_STDX(STDERR_FILENO)

#define LOG_RECORDS_PARTIAL "\
127.0.0.1 - - [30/Jun/2022:16:43:51 +0300] \"GET / HTTP/1.0\" 200 11192 \"-\" \"ApacheBench/2.3\"\n\
192.168.2.1 - - [30/Jun/2022:16:43:51 +0300] \"PUT / HTTP/1.0\" 400 11192 \"-\" \"ApacheBench/2.3\"\n\
255.91.204.202 - mann1475 [30/Jun/2023:21:05:09 +0000] \"POST /vertical/turn-key/engineer/e-enable HTTP/1.0\" 401 11411\n\
91.126.60.234 - ritchie4302 [30/Jun/2023:21:05:09 +0000] \"PATCH /empower/interfaces/deploy HTTP/2.0\" 404 29063\n\
120.134.242.160 - runte5364 [30/Jun/2023:21:05:09 +0000] \"GET /visualize/enterprise/optimize/embrace HTTP/1.0\" 400 10637\n\
61.134.57.25 - - [30/Jun/2023:21:05:09 +0000] \"HEAD /metrics/optimize/bandwidth HTTP/1.1\" 200 26713\n\
18.90.118.50 - - [30/Jun/2023:21:05:09 +0000] \"PATCH /methodologies/extend HTTP/2.0\" 205 15708\n\
21.174.251.223 - zulauf8852 [30/Jun/2023:21:05:09 +0000] \"POST /proactive HTTP/2.0\" 100 9456\n\
20.217.190.46 - - [30/Jun/2023:21:05:09 +0000] \"GET /mesh/frictionless HTTP/1.1\" 301 3153\n\
130.43.250.80 - hintz5738 [30/Jun/2023:21:05:09 +0000] \"PATCH /e-markets/supply-chains/mindshare HTTP/2.0\" 401 13039\n\
222.36.95.121 - pouros3514 [30/Jun/2023:21:05:09 +0000] \"DELETE /e-commerce/scale/customized/best-of-breed HTTP/1.0\" 406 8304\n\
133.117.9.29 - hoeger7673 [30/Jun/2023:21:05:09 +0000] \"PUT /extensible/maximize/visualize/bricks-and-clicks HTTP/1.0\" 403 17067\n\
65.145.39.136 - heathcote3368 [30/Jun/2023:21:05:09 +0000] \"DELETE /technologies/iterate/viral HTTP/1.1\" 501 29982\n\
153.132.199.122 - murray8217 [30/Jun/2023:21:05:09 +0000] \"PUT /orchestrate/visionary/visualize HTTP/1.1\" 500 12705\n\
140.149.178.196 - hickle8613 [30/Jun/2023:21:05:09 +0000] \"PATCH /drive/front-end/infomediaries/maximize HTTP/1.1\" 406 20179\n\
237.31.189.207 - - [30/Jun/2023:21:05:09 +0000] \"GET /bleeding-edge/recontextualize HTTP/1.1\" 406 24815\n\
210.217.232.107 - - [30/Jun/2023:21:05:09 +0000] \"POST /redefine/next-generation/relationships/intuitive HTTP/2.0\" 205 14028\n\
121.2.189.119 - marvin5528 [30/Jun/2023:21:05:09 +0000] \"PUT /sexy/innovative HTTP/2.0\" 204 10689\n\
120.13.121.164 - jakubowski1027 [30/Jun/2023:21:05:09 +0000] \"PUT /sexy/initiatives/morph/eyeballs HTTP/1.0\" 502 22287\n\
28.229.107.175 - wilderman8830 [30/Jun/2023:21:05:09 +0000] \"PATCH /visionary/best-of-breed HTTP/1.1\" 503 6010\n\
210.147.186.50 - - [30/Jun/2023:21:05:09 +0000] \"PUT /paradigms HTTP/2.0\" 501 18054\n\
185.157.236.127 - - [30/Jun/2023:21:05:09 +0000] \"GET /maximize HTTP/1.0\" 400 13650\n\
236.90.19.165 - - [30/Jun/2023:21:23:34 +0000] \"GET /next-generation/user-centric/24%2f365 HTTP/1.0\" 400 5212\n\
233.182.111.100 - torphy3512 [30/Jun/2023:21:23:34 +0000] \"PUT /seamless/incentivize HTTP/1.0\" 304 27750\n\
80.185.129.193 - - [30/Jun/2023:21:23:34 +0000] \"HEAD /strategic HTTP/1.1\" 502 6146\n\
182.145.92.52 - - [30/Jun/2023:21:23:34 +0000] \"PUT /dot-com/grow/networks HTTP/1.0\" 301 1763\n\
46.14.122.16 - - [30/Jun/2023:21:23:34 +0000] \"HEAD /deliverables HTTP/1.0\" 301 7608\n\
162.111.143.158 - bruen3883 [30/Jun/2023:21:23:34 +0000] \"POST /extensible HTTP/2.0\" 403 22752\n\
201.13.111.255 - hilpert8768 [30/Jun/2023:21:23:34 +0000] \"PATCH /applications/engage/frictionless/content HTTP/1.0\" 406 24866\n\
76.90.243.15 - - [30/Jun/2023:21:23:34 +0000] \"PATCH /24%2f7/seamless/target/enable HTTP/1.1\" 503 8176\n\
187.79.114.48 - - [30/Jun/2023:21:23:34 +0000] \"GET /synergistic HTTP/1.0\" 503 14251\n\
59.52.178.62 - kirlin3704 [30/Jun/2023:21:23:34 +0000] \"POST /web-readiness/grow/evolve HTTP/1.0\" 501 13305\n\
27.46.78.167 - - [30/Jun/2023:21:23:34 +0000] \"PATCH /interfaces/schemas HTTP/2.0\" 100 4860\n\
191.9.15.43 - goodwin7310 [30/Jun/2023:21:23:34 +0000] \"POST /engage/innovate/web-readiness/roi HTTP/2.0\" 404 4225\n\
195.153.126.148 - klein8350 [30/Jun/2023:21:23:34 +0000] \"DELETE /killer/synthesize HTTP/1.0\" 204 15134\n\
162.207.64.184 - mayert4426 [30/Jun/2023:21:23:34 +0000] \"HEAD /intuitive/vertical/incentivize HTTP/1.0\" 204 23666\n\
185.96.7.205 - - [30/Jun/2023:21:23:34 +0000] \"DELETE /communities/deliver/user-centric HTTP/1.0\" 416 18210\n\
187.180.105.55 - - [30/Jun/2023:21:23:34 +0000] \"POST /customized HTTP/2.0\" 200 1396\n\
216.82.243.54 - kunze7200 [30/Jun/2023:21:23:34 +0000] \"PUT /e-tailers/evolve/leverage/engage HTTP/2.0\" 504 1665\n\
170.128.69.228 - - [30/Jun/2023:21:23:34 +0000] \"DELETE /matrix/open-source/proactive HTTP/1.0\" 301 18326\n\
253.200.84.66 - steuber5220 [30/Jun/2023:21:23:34 +0000] \"POST /benchmark/experiences HTTP/1.1\" 504 18944\n\
28.240.40.161 - - [30/Jun/2023:21:23:34 +0000] \"PATCH /initiatives HTTP/1.0\" 500 6500\n\
134.163.236.75 - - [30/Jun/2023:21:23:34 +0000] \"HEAD /platforms/recontextualize HTTP/1.0\" 203 22188\n\
241.64.230.66 - - [30/Jun/2023:21:23:34 +0000] \"GET /cutting-edge/methodologies/b2c/cross-media HTTP/1.1\" 403 20698\n\
210.216.183.157 - okuneva6218 [30/Jun/2023:21:23:34 +0000] \"POST /generate/incentivize HTTP/2.0\" 403 25900\n\
164.219.134.242 - - [30/Jun/2023:21:23:34 +0000] \"HEAD /efficient/killer/whiteboard HTTP/2.0\" 501 22081\n\
173.156.54.99 - harvey6165 [30/Jun/2023:21:23:34 +0000] \"HEAD /dynamic/cutting-edge/sexy/user-centric HTTP/2.0\" 200 2995\n\
215.242.74.14 - - [30/Jun/2023:21:23:34 +0000] \"PUT /roi HTTP/1.0\" 204 9674\n\
133.77.49.187 - lockman3141 [30/Jun/2023:21:23:34 +0000] \"PUT /mindshare/transition HTTP/2.0\" 503 2726\n\
159.77.190.255 - - [30/Jun/2023:21:23:34 +0000] \"DELETE /world-class/bricks-and-clicks HTTP/1.1\" 501 21712\n\
65.6.237.113 - - [30/Jun/2023:21:23:34 +0000] \"PATCH /e-enable HTTP/2.0\" 405 11865\n\
194.76.211.16 - champlin6280 [30/Jun/2023:21:23:34 +0000] \"PUT /applications/redefine/eyeballs/mindshare HTTP/1.0\" 302 27679\n\
96.206.219.202 - - [30/Jun/2023:21:23:34 +0000] \"PUT /solutions/mindshare/vortals/transition HTTP/1.0\" 403 7385\n\
255.80.116.201 - hintz8162 [30/Jun/2023:21:23:34 +0000] \"POST /frictionless/e-commerce HTTP/1.0\" 302 9235\n\
89.66.165.183 - smith2655 [30/Jun/2023:21:23:34 +0000] \"HEAD /markets/synergize HTTP/2.0\" 501 28055\n\
39.210.168.14 - - [30/Jun/2023:21:23:34 +0000] \"GET /integrate/killer/end-to-end/infrastructures HTTP/1.0\" 302 11311\n\
173.99.112.210 - - [30/Jun/2023:21:23:34 +0000] \"GET /interfaces HTTP/2.0\" 503 1471\n\
108.4.157.6 - morissette1161 [30/Jun/2023:21:23:34 +0000] \"POST /mesh/convergence HTTP/1.1\" 403 18708\n\
174.160.107.162 - - [30/Jun/2023:21:23:34 +0000] \"POST /vortals/monetize/utilize/synergistic HTTP/1.1\" 302 13252\n\
188.8.105.56 - beatty6880 [30/Jun/2023:21:23:34 +0000] \"POST /web+services/innovate/generate/leverage HTTP/1.1\" 301 29856\n\
115.179.64.255 - - [30/Jun/2023:21:23:34 +0000] \"PATCH /transform/transparent/b2c/holistic HTTP/1.1\" 406 10208\n\
48.104.215.32 - - [30/Jun/2023:21:23:34 +0000] \"DELETE /drive/clicks-and-mortar HTTP/1.0\" 501 13752\n\
75.212.115.12 - pfannerstill5140 [30/Jun/2023:21:23:34 +0000] \"PATCH /leading-edge/mesh/methodologies HTTP/1.0\" 503 4946\n\
52.75.2.117 - osinski2030 [30/Jun/2023:21:23:34 +0000] \"PUT /incentivize/recontextualize HTTP/1.1\" 301 8785\n"

#define LOG_RECORD_WITHOUT_NEW_LINE \
"82.39.169.93 - streich5722 [30/Jun/2023:21:23:34 +0000] \"GET /action-items/leading-edge/reinvent/maximize HTTP/1.1\" 500 1228"

#define LOG_RECORDS_WITHOUT_TERMINATING_NEW_LINE \
        LOG_RECORDS_PARTIAL \
        LOG_RECORD_WITHOUT_NEW_LINE

#define LOG_RECORD_WITH_NEW_LINE \
"131.128.33.109 - turcotte6735 [30/Jun/2023:21:23:34 +0000] \"PUT /distributed/strategize HTTP/1.1\" 401 16471\n"

#define LOG_RECORDS_WITH_TERMINATING_NEW_LINE \
        LOG_RECORDS_PARTIAL \
        LOG_RECORD_WITH_NEW_LINE

static int test_compression_decompression() {
    int errors = 0;
    fprintf(stderr, "%s():\n", __FUNCTION__);

    Circ_buff_item_t item;
    item.text_size = sizeof(LOG_RECORDS_WITH_TERMINATING_NEW_LINE);
    fprintf(stderr, "Testing LZ4_compressBound()...\n");
    size_t required_compressed_space  = LZ4_compressBound(item.text_size);
    if(!required_compressed_space){
        fprintf(stderr, "- Error while using LZ4_compressBound()\n");
        return ++errors;
    }

    item.data_max_size = item.text_size + required_compressed_space;
    item.data = mallocz(item.data_max_size);
    memcpy(item.data, LOG_RECORDS_WITH_TERMINATING_NEW_LINE, sizeof(LOG_RECORDS_WITH_TERMINATING_NEW_LINE));

    fprintf(stderr, "Testing LZ4_compress_fast()...\n");    
    item.text_compressed = item.data + item.text_size;

    item.text_compressed_size = LZ4_compress_fast(  item.data, item.text_compressed, 
                                                    item.text_size, required_compressed_space, 1);
    if(!item.text_compressed_size){
        fprintf(stderr, "- Error while using LZ4_compress_fast()\n");
        return ++errors;                      
    }

    char *decompressed_text = mallocz(item.text_size);

    if(LZ4_decompress_safe( item.text_compressed, 
                            decompressed_text, 
                            item.text_compressed_size, 
                            item.text_size) < 0){
        fprintf(stderr, "- Error in decompress_text()\n");
        return ++errors;
    }

    if(memcmp(item.data, decompressed_text, item.text_size)){
        fprintf(stderr, "- Error, original and decompressed data not the same\n");
        ++errors;
    }
    freez(decompressed_text);

    fprintf(stderr, "%s\n", errors ? "FAIL" : "OK");
    return errors;
}

static int test_read_last_line() {
    int errors = 0;
    fprintf(stderr, "%s():\n", __FUNCTION__);

    #if defined(_WIN32) || defined(_WIN64)
    char tmpname[MAX_PATH] = "/tmp/tmp.XXXXXX";
    #else
    char tmpname[] = "/tmp/tmp.XXXXXX";
    #endif
    (void) umask(0022);

    int fd = mkstemp(tmpname);
    if (fd == -1){
        fprintf(stderr, "mkstemp() Failed with error %s\n", strerror(errno));
        exit(EXIT_FAILURE);
    }

    FILE *tmpfp = fdopen(fd, "r+");
    if (tmpfp == NULL) {
        close(fd);
        unlink(tmpname);
        exit(EXIT_FAILURE);
    }

    if(fprintf(tmpfp, "%s", LOG_RECORDS_WITHOUT_TERMINATING_NEW_LINE) <= 0){
        close(fd);
        unlink(tmpname);
        exit(EXIT_FAILURE);
    }
    fflush(tmpfp);

    fprintf(stderr, "Testing read of LOG_RECORD_WITHOUT_NEW_LINE...\n");  
    errors += strcmp(LOG_RECORD_WITHOUT_NEW_LINE, read_last_line(tmpname, 0)) ? 1 : 0;

    if(fprintf(tmpfp, "\n%s", LOG_RECORD_WITH_NEW_LINE) <= 0){
        close(fd);
        unlink(tmpname);
        exit(EXIT_FAILURE);
    }
    fflush(tmpfp);

    fprintf(stderr, "Testing read of LOG_RECORD_WITH_NEW_LINE...\n");
    errors += strcmp(LOG_RECORD_WITH_NEW_LINE, read_last_line(tmpname, 0)) ? 1 : 0;

    unlink(tmpname);
    close(fd);
    fclose(tmpfp);

    fprintf(stderr, "%s\n", errors ? "FAIL" : "OK");
    return errors;
}

const char * const parse_configs_to_test[] = {
    /* [1] Apache csvCombined 1 */
    "127.0.0.1 - - [15/Oct/2020:04:43:51 -0700] \"GET / HTTP/1.0\" 200 11228 \"-\" \"ApacheBench/2.3\"",

    /* [2] Apache csvCombined 2 - extra white space */
    "::1 - - [01/Sep/2022:19:04:42 +0100] \"GET   /   HTTP/1.1\" 200 3477 \"-\" \"Mozilla/5.0 (Windows NT 10.0; \
Win64; x64; rv:103.0)    Gecko/20100101 Firefox/103.0\"",

    /* [3] Apache csvCombined 3 - with new line */
    "209.202.252.202 - rosenbaum7551 [20/Jun/2023:14:42:27 +0000] \"PUT /harness/networks/initiatives/engineer HTTP/2.0\"\
    403 42410 \"https://www.senioriterate.name/streamline/exploit\" \"Opera/10.54 (Macintosh; Intel Mac OS X 10_7_6;\
 en-US) Presto/2.12.334 Version/10.00\"\n",

    /* [4] Apache csvCombined 4 - invalid request field */
    "::1 - - [13/Jul/2023:21:00:56 +0100] \"-\" 408 - \"-\" \"-\"",

    /* [5] Apache csvVhostCombined */
    "XPS-wsl.localdomain:80 ::1 - - [30/Jun/2022:20:59:29 +0300] \"GET / HTTP/1.1\" 200 3477 \"-\" \"Mozilla\
/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/103.0.5060.53 Safari/537.36\
 Edg/103.0.1264.37\"",

    /* [6] Apache csvCommon 1 */                                                                                        
    "127.0.0.1 - - [30/Jun/2022:16:43:51 +0300] \"GET / HTTP/1.0\" 200 11228",

    /* [7] Apache csvCommon 2 - with carriage return */                                                                                        
    "180.89.137.89 - barrows1527 [05/Jun/2023:17:46:08 +0000]\
 \"DELETE /b2c/viral/innovative/reintermediate HTTP/1.0\" 416 99\r",

    /* [8] Apache csvCommon 3 - with new line */
    "212.113.230.101 - - [20/Jun/2023:14:29:49 +0000] \"PATCH /strategic HTTP/1.1\" 404 1217\n",

    /* [9] Apache csvVhostCommon 1 */                  
    "XPS-wsl.localdomain:80 127.0.0.1 - - [30/Jun/2022:16:43:51 +0300] \"GET / HTTP/1.0\" 200 11228",

    /* [10] Apache csvVhostCommon 2 - with new line and extra white space */      
    "XPS-wsl.localdomain:80    2001:0db8:85a3:0000:0000:8a2e:0370:7334 -   - [30/Jun/2022:16:43:51 +0300] \"GET /\
 HTTP/1.0\" 200 11228\n",

    /* [11] Nginx csvCombined */
    "47.29.201.179 - - [28/Feb/2019:13:17:10 +0000] \"GET /?p=1 HTTP/2.0\" 200 5316 \"https://dot.com/?p=1\"\
 \"Mozilla/5.0 (Windows NT 6.1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/72.0.3626.119 Safari/537.36\"", 
};
const web_log_line_field_t parse_config_expected[][15] = {
    /* [1]  */ {REQ_CLIENT     , CUSTOM    , CUSTOM, TIME  , TIME, REQ , RESP_CODE, RESP_SIZE, CUSTOM   , CUSTOM ,     -1, -1, -1, -1, -1}, /* Apache csvCombined 1 */
    /* [2]  */ {REQ_CLIENT     , CUSTOM    , CUSTOM, TIME  , TIME, REQ , RESP_CODE, RESP_SIZE, CUSTOM   , CUSTOM ,     -1, -1, -1, -1, -1}, /* Apache csvCombined 2 */
    /* [3]  */ {REQ_CLIENT     , CUSTOM    , CUSTOM, TIME  , TIME, REQ , RESP_CODE, RESP_SIZE, CUSTOM   , CUSTOM ,     -1, -1, -1, -1, -1}, /* Apache csvCombined 3 */
    /* [4]  */ {REQ_CLIENT     , CUSTOM    , CUSTOM, TIME  , TIME, REQ , RESP_CODE, RESP_SIZE, CUSTOM   , CUSTOM ,     -1, -1, -1, -1, -1}, /* Apache csvCombined 4 */
    /* [5]  */ {VHOST_WITH_PORT, REQ_CLIENT, CUSTOM, CUSTOM, TIME, TIME, REQ      , RESP_CODE, RESP_SIZE, CUSTOM , CUSTOM, -1, -1, -1, -1}, /* Apache csvVhostCombined */
    /* [6]  */ {REQ_CLIENT     , CUSTOM    , CUSTOM, TIME  , TIME, REQ , RESP_CODE, RESP_SIZE, -1       , -1     ,     -1, -1, -1, -1, -1}, /* Apache csvCommon 1 */
    /* [7]  */ {REQ_CLIENT     , CUSTOM    , CUSTOM, TIME  , TIME, REQ , RESP_CODE, RESP_SIZE, -1       , -1     ,     -1, -1, -1, -1, -1}, /* Apache csvCommon 2 */
    /* [8]  */ {REQ_CLIENT     , CUSTOM    , CUSTOM, TIME  , TIME, REQ , RESP_CODE, RESP_SIZE, -1       , -1     ,     -1, -1, -1, -1, -1}, /* Apache csvCommon 3 */
    /* [9]  */ {VHOST_WITH_PORT, REQ_CLIENT, CUSTOM, CUSTOM, TIME, TIME, REQ      , RESP_CODE, RESP_SIZE, -1     ,     -1, -1, -1, -1, -1}, /* Apache csvVhostCommon 1 */
    /* [10]  */ {VHOST_WITH_PORT, REQ_CLIENT, CUSTOM, CUSTOM, TIME, TIME, REQ      , RESP_CODE, RESP_SIZE, -1     ,     -1, -1, -1, -1, -1}, /* Apache csvVhostCommon 2 */
    /* [11] */ {REQ_CLIENT     , CUSTOM    , CUSTOM, TIME  , TIME, REQ,  RESP_CODE, RESP_SIZE, CUSTOM   , CUSTOM ,     -1, -1, -1, -1, -1}, /* Nginx csvCombined */
};
static const char parse_config_delim = ' ';
static int *parse_config_expected_num_fields = NULL;

static void setup_parse_config_expected_num_fields() {
    fprintf(stderr, "%s():\n", __FUNCTION__);

    for(int i = 0; i < (int) (sizeof(parse_configs_to_test) / sizeof(parse_configs_to_test[0])); i++){
        parse_config_expected_num_fields = reallocz(parse_config_expected_num_fields, (i + 1) * sizeof(int));
        parse_config_expected_num_fields[i] = 0;
        for(int j = 0; (int) parse_config_expected[i][j] != -1; j++){
            parse_config_expected_num_fields[i]++;
        }
    }

    fprintf(stderr, "OK\n");
}

static int test_count_fields() {
    int errors = 0;
    fprintf(stderr, "%s():\n", __FUNCTION__);

    for(int i = 0; i < (int) (sizeof(parse_configs_to_test) / sizeof(parse_configs_to_test[0])); i++){
        if(count_fields(parse_configs_to_test[i], parse_config_delim) != parse_config_expected_num_fields[i]){
            fprintf(stderr, "- Error (count_fields() result incorrect) for:\n%s", parse_configs_to_test[i]);
            ++errors;
        }
    }

    fprintf(stderr, "%s\n", errors ? "FAIL" : "OK");
    return errors;
}

static int test_auto_detect_web_log_parser_config() {
    int errors = 0;
    fprintf(stderr, "%s():\n", __FUNCTION__);

    for(int i = 0; i < (int) (sizeof(parse_configs_to_test) / sizeof(parse_configs_to_test[0])); i++){
        size_t line_sz = strlen(parse_configs_to_test[i]) + 1;
        char *line = strdupz(parse_configs_to_test[i]);
        if(line[line_sz - 2] != '\n' && line[line_sz - 2] != '\r'){
            line = reallocz(line, ++line_sz); // +1 to add '\n' char
            line[line_sz - 1] = '\0';
            line[line_sz - 2] = '\n';
        }
        Web_log_parser_config_t *wblp_conf = auto_detect_web_log_parser_config(line, parse_config_delim);
        if(!wblp_conf){
            fprintf(stderr, "- Error (NULL wblp_conf) for:\n%s", line);
            ++errors;
        } else if(wblp_conf->num_fields != parse_config_expected_num_fields[i]){
            fprintf(stderr, "- Error (number of fields mismatch) for:\n%s", line);
            fprintf(stderr, "Expected %d fields but auto-detected %d\n", parse_config_expected_num_fields[i], wblp_conf->num_fields);
            ++errors;
        } else {
            for(int j = 0; (int) parse_config_expected[i][j] != -1; j++){
                if(wblp_conf->fields[j] != parse_config_expected[i][j]){
                    fprintf(stderr, "- Error (field type mismatch) for:\n%s", line);
                    ++errors;
                    break;
                }
            }
        }

        freez(line);
        if(wblp_conf) freez(wblp_conf->fields);
        freez(wblp_conf);
    }

    fprintf(stderr, "%s\n", errors ? "FAIL" : "OK");
    return errors;
}

Log_line_parsed_t log_line_parsed_expected[] = {
    /* --------------------------------------
    char vhost[VHOST_MAX_LEN];
    int  port;
    char req_scheme[REQ_SCHEME_MAX_LEN];
    char req_client[REQ_CLIENT_MAX_LEN];
    char req_method[REQ_METHOD_MAX_LEN];
    char req_URL[REQ_URL_MAX_LEN];
    char req_proto[REQ_PROTO_MAX_LEN];
    int req_size;
    int req_proc_time;
    int resp_code;
    int resp_size;
    int ups_resp_time;
    char ssl_proto[SSL_PROTO_MAX_LEN];
    char ssl_cipher[SSL_CIPHER_SUITE_MAX_LEN];
    int64_t timestamp;
    int parsing_errors; 
    ------------------------------------------ */
    /* [1]  */ {"",                    0,  "", "127.0.0.1",                                "GET",    "/",                                      "1.0", 0, 0, 200, 11228, 0, "", "", 1602762231, 0}, 
    /* [2]  */ {"",                    0,  "", "::1",                                      "GET",    "/",                                      "1.1", 0, 0, 200, 3477 , 0, "", "", 1662055482, 0},
    /* [3]  */ {"",                    0,  "", "209.202.252.202",                          "PUT",    "/harness/networks/initiatives/engineer", "2.0", 0, 0, 403, 42410, 0, "", "", 1687272147, 0},
    /* [4]  */ {"",                    0,  "", "::1",                                      "-",      "",                                       "",    0, 0, 408, 0,     0, "", "", 1689278456, 0},
    /* [5]  */ {"XPS-wsl.localdomain", 80, "", "::1",                                      "GET",    "/",                                      "1.1", 0, 0, 200, 3477 , 0, "", "", 1656611969, 0},
    /* [6]  */ {"",                    0,  "", "127.0.0.1",                                "GET",    "/",                                      "1.0", 0, 0, 200, 11228, 0, "", "", 1656596631, 0},
    /* [7]  */ {"",                    0,  "", "180.89.137.89",                            "DELETE", "/b2c/viral/innovative/reintermediate",   "1.0", 0, 0, 416, 99   , 0, "", "", 1685987168, 0},
    /* [8]  */ {"",                    0,  "", "212.113.230.101",                          "PATCH",  "/strategic",                             "1.1", 0, 0, 404, 1217 , 0, "", "", 1687271389, 0},
    /* [9]  */ {"XPS-wsl.localdomain", 80, "", "127.0.0.1",                                "GET",    "/",                                      "1.0", 0, 0, 200, 11228, 0, "", "", 1656596631, 0},
    /* [10] */ {"XPS-wsl.localdomain", 80, "", "2001:0db8:85a3:0000:0000:8a2e:0370:7334",  "GET",    "/",                                      "1.0", 0, 0, 200, 11228, 0, "", "", 1656596631, 0},
    /* [11] */ {"",                    0,  "", "47.29.201.179",                            "GET",    "/?p=1",                                  "2.0", 0, 0, 200, 5316 , 0, "", "", 1551359830, 0}
};
static int test_parse_web_log_line(){
    int errors = 0;
    fprintf(stderr, "%s():\n", __FUNCTION__);

    Web_log_parser_config_t *wblp_conf = callocz(1, sizeof(Web_log_parser_config_t));
   
    wblp_conf->delimiter = parse_config_delim;
    wblp_conf->verify_parsed_logs = 1;

    for(int i = 0; i < (int) (sizeof(parse_configs_to_test) / sizeof(parse_configs_to_test[0])); i++){
        wblp_conf->num_fields = parse_config_expected_num_fields[i];
        wblp_conf->fields = (web_log_line_field_t *) parse_config_expected[i];

        Log_line_parsed_t log_line_parsed = (Log_line_parsed_t) {0};
        parse_web_log_line( wblp_conf, 
                            (char *) parse_configs_to_test[i], 
                            strlen(parse_configs_to_test[i]), 
                            &log_line_parsed);
        
        if(strcmp(log_line_parsed_expected[i].vhost, log_line_parsed.vhost))
            fprintf(stderr, "- Error (parsed vhost:%s != expected vhost:%s) for:\n%s",
                log_line_parsed.vhost, log_line_parsed_expected[i].vhost, parse_configs_to_test[i]), ++errors;
        if(log_line_parsed_expected[i].port != log_line_parsed.port)
            fprintf(stderr, "- Error (parsed port:%d != expected port:%d) for:\n%s", 
                log_line_parsed.port, log_line_parsed_expected[i].port, parse_configs_to_test[i]), ++errors;
        if(strcmp(log_line_parsed_expected[i].req_scheme, log_line_parsed.req_scheme))
            fprintf(stderr, "- Error (parsed req_scheme:%s != expected req_scheme:%s) for:\n%s", 
                log_line_parsed.req_scheme, log_line_parsed_expected[i].req_scheme, parse_configs_to_test[i]), ++errors;
        if(strcmp(log_line_parsed_expected[i].req_client, log_line_parsed.req_client))
            fprintf(stderr, "- Error (parsed req_client:%s != expected req_client:%s) for:\n%s", 
                log_line_parsed.req_client, log_line_parsed_expected[i].req_client, parse_configs_to_test[i]), ++errors;
        if(strcmp(log_line_parsed_expected[i].req_method, log_line_parsed.req_method))
            fprintf(stderr, "- Error (parsed req_method:%s != expected req_method:%s) for:\n%s", 
                log_line_parsed.req_method, log_line_parsed_expected[i].req_method, parse_configs_to_test[i]), ++errors;
        if(strcmp(log_line_parsed_expected[i].req_URL, log_line_parsed.req_URL))
            fprintf(stderr, "- Error (parsed req_URL:%s != expected req_URL:%s) for:\n%s", 
                log_line_parsed.req_URL, log_line_parsed_expected[i].req_URL, parse_configs_to_test[i]), ++errors;
        if(strcmp(log_line_parsed_expected[i].req_proto, log_line_parsed.req_proto))
            fprintf(stderr, "- Error (parsed req_proto:%s != expected req_proto:%s) for:\n%s", 
                log_line_parsed.req_proto, log_line_parsed_expected[i].req_proto, parse_configs_to_test[i]), ++errors;
        if(log_line_parsed_expected[i].req_size != log_line_parsed.req_size)
            fprintf(stderr, "- Error (parsed req_size:%d != expected req_size:%d) for:\n%s", 
                log_line_parsed.req_size, log_line_parsed_expected[i].req_size, parse_configs_to_test[i]), ++errors;
        if(log_line_parsed_expected[i].req_proc_time != log_line_parsed.req_proc_time)
            fprintf(stderr, "- Error (parsed req_proc_time:%d != expected req_proc_time:%d) for:\n%s", 
                log_line_parsed.req_proc_time, log_line_parsed_expected[i].req_proc_time, parse_configs_to_test[i]), ++errors;
        if(log_line_parsed_expected[i].resp_code != log_line_parsed.resp_code)
            fprintf(stderr, "- Error (parsed resp_code:%d != expected resp_code:%d) for:\n%s", 
                log_line_parsed.resp_code, log_line_parsed_expected[i].resp_code, parse_configs_to_test[i]), ++errors;
        if(log_line_parsed_expected[i].resp_size != log_line_parsed.resp_size)
            fprintf(stderr, "- Error (parsed resp_size:%d != expected resp_size:%d) for:\n%s", 
                log_line_parsed.resp_size, log_line_parsed_expected[i].resp_size, parse_configs_to_test[i]), ++errors;
        if(log_line_parsed_expected[i].ups_resp_time != log_line_parsed.ups_resp_time)
            fprintf(stderr, "- Error (parsed ups_resp_time:%d != expected ups_resp_time:%d) for:\n%s", 
                log_line_parsed.ups_resp_time, log_line_parsed_expected[i].ups_resp_time, parse_configs_to_test[i]), ++errors;
        if(strcmp(log_line_parsed_expected[i].ssl_proto, log_line_parsed.ssl_proto))
            fprintf(stderr, "- Error (parsed ssl_proto:%s != expected ssl_proto:%s) for:\n%s", 
                log_line_parsed.ssl_proto, log_line_parsed_expected[i].ssl_proto, parse_configs_to_test[i]), ++errors;
        if(strcmp(log_line_parsed_expected[i].ssl_cipher, log_line_parsed.ssl_cipher))
            fprintf(stderr, "- Error (parsed ssl_cipher:%s != expected ssl_cipher:%s) for:\n%s", 
                log_line_parsed.ssl_cipher, log_line_parsed_expected[i].ssl_cipher, parse_configs_to_test[i]), ++errors;
        if(log_line_parsed_expected[i].timestamp != log_line_parsed.timestamp)
            fprintf(stderr, "- Error (parsed timestamp:%" PRId64 " != expected timestamp:%" PRId64 ") for:\n%s", 
                log_line_parsed.timestamp, log_line_parsed_expected[i].timestamp, parse_configs_to_test[i]), ++errors;
    }

    freez(wblp_conf);

    fprintf(stderr, "%s\n", errors ? "FAIL" : "OK");
    return errors ;
}

const char * const unsanitised_strings[] = { "[test]", "^test$", "{test}", 
                                      "(test)", "\\test\\", "test*+.?|", "test&£@"};
const char * const expected_sanitised_strings[] = { "\\[test\\]", "\\^test\\$", "\\{test\\}", 
                                             "\\(test\\)", "\\\\test\\\\", "test\\*\\+\\.\\?\\|", "test&£@"};
static int test_sanitise_string(){
    int errors = 0;
    fprintf(stderr, "%s():\n", __FUNCTION__);

    for(int i = 0; i < (int) (sizeof(unsanitised_strings) / sizeof(unsanitised_strings[0])); i++){
        char *sanitised = sanitise_string((char *) unsanitised_strings[i]);
        if(strcmp(expected_sanitised_strings[i], sanitised)){
            fprintf(stderr, "- Error during sanitise_string() for:%s\n", unsanitised_strings[i]);
            ++errors;
        };
        freez(sanitised);
    }

    fprintf(stderr, "%s\n", errors ? "FAIL" : "OK");
    return errors;
}

char * const regex_src[] = { 
"2022-11-07T11:28:27.427519600Z container create e0c3c6120c29beb393e4b92773c9aa60006747bddabd352b77bf0b4ad23747a7 (image=hello-world, name=xenodochial_lumiere)\n\
2022-11-07T11:28:27.932624500Z container start e0c3c6120c29beb393e4b92773c9aa60006747bddabd352b77bf0b4ad23747a7 (image=hello-world, name=xenodochial_lumiere)\n\
2022-11-07T11:28:27.971060500Z container die e0c3c6120c29beb393e4b92773c9aa60006747bddabd352b77bf0b4ad23747a7 (exitCode=0, image=hello-world, name=xenodochial_lumiere)",
    
"2022-11-07T11:28:27.427519600Z container create e0c3c6120c29beb393e4b92773c9aa60006747bddabd352b77bf0b4ad23747a7 (image=hello-world, name=xenodochial_lumiere)\n\
2022-11-07T11:28:27.932624500Z container start e0c3c6120c29beb393e4b92773c9aa60006747bddabd352b77bf0b4ad23747a7 (image=hello-world, name=xenodochial_lumiere)\n\
2022-11-07T11:28:27.971060500Z container die e0c3c6120c29beb393e4b92773c9aa60006747bddabd352b77bf0b4ad23747a7 (exitCode=0, image=hello-world, name=xenodochial_lumiere)",

"2022-11-07T11:28:27.427519600Z container create e0c3c6120c29beb393e4b92773c9aa60006747bddabd352b77bf0b4ad23747a7 (image=hello-world, name=xenodochial_lumiere)\n\
2022-11-07T11:28:27.932624500Z container start e0c3c6120c29beb393e4b92773c9aa60006747bddabd352b77bf0b4ad23747a7 (image=hello-world, name=xenodochial_lumiere)\n\
2022-11-07T11:28:27.971060500Z container die e0c3c6120c29beb393e4b92773c9aa60006747bddabd352b77bf0b4ad23747a7 (exitCode=0, image=hello-world, name=xenodochial_lumiere)",

"2022-11-07T20:06:36.919980700Z container create bd8d4a3338c3e9ab4ca555c6d869dc980f04f10ebdcd9284321c0afecbec1234 (image=hello-world, name=distracted_sinoussi)\n\
2022-11-07T20:06:36.927728700Z container attach bd8d4a3338c3e9ab4ca555c6d869dc980f04f10ebdcd9284321c0afecbec1234 (image=hello-world, name=distracted_sinoussi)\n\
2022-11-07T20:06:36.958906200Z network connect 178a1988c4173559c721d5e24970eef32aaca41e0e363ff9792c731f917683ed (container=bd8d4a3338c3e9ab4ca555c6d869dc980f04f10ebdcd9284321c0afecbec1234, name=bridge, type=bridge)\n\
2022-11-07T20:06:37.564947300Z container start bd8d4a3338c3e9ab4ca555c6d869dc980f04f10ebdcd9284321c0afecbec1234 (image=hello-world, name=distracted_sinoussi)\n\
2022-11-07T20:06:37.596428500Z container die bd8d4a3338c3e9ab4ca555c6d869dc980f04f10ebdcd9284321c0afecbec1234 (exitCode=0, image=hello-world, name=distracted_sinoussi)\n\
2022-11-07T20:06:38.134325100Z network disconnect 178a1988c4173559c721d5e24970eef32aaca41e0e363ff9792c731f917683ed (container=bd8d4a3338c3e9ab4ca555c6d869dc980f04f10ebdcd9284321c0afecbec1234, name=bridge, type=bridge)",

"Nov  7 21:54:24 X-PC sudo: john : TTY=pts/7 ; PWD=/home/john ; USER=root ; COMMAND=/usr/bin/docker run hello-world\n\
Nov  7 21:54:24 X-PC sudo: pam_unix(sudo:session): session opened for user root by john(uid=0)\n\
Nov  7 21:54:25 X-PC sudo: pam_unix(sudo:session): session closed for user root\n\
Nov  7 21:54:24 X-PC sudo: john : TTY=pts/7 ; PWD=/home/john ; USER=root ; COMMAND=/usr/bin/docker run hello-world\n"
};
const char * const regex_keyword[] = {
    "start",
    "CONTAINER",
    "CONTAINER",
    NULL,
    NULL
};
const char * const regex_pat_str[] = {
    NULL,
    NULL,
    NULL,
    ".*\\bcontainer\\b.*\\bhello-world\\b.*",
    ".*\\bsudo\\b.*\\bCOMMAND=/usr/bin/docker run\\b.*"

};
const int regex_ignore_case[] = {
    1,
    1,
    0,
    1,
    1
};
const int regex_exp_matches[] = {
    1,
    3,
    0,
    4,
    2
};
const char * const regex_exp_dst[] = { 
"2022-11-07T11:28:27.932624500Z container start e0c3c6120c29beb393e4b92773c9aa60006747bddabd352b77bf0b4ad23747a7 (image=hello-world, name=xenodochial_lumiere)\n",

"2022-11-07T11:28:27.427519600Z container create e0c3c6120c29beb393e4b92773c9aa60006747bddabd352b77bf0b4ad23747a7 (image=hello-world, name=xenodochial_lumiere)\n\
2022-11-07T11:28:27.932624500Z container start e0c3c6120c29beb393e4b92773c9aa60006747bddabd352b77bf0b4ad23747a7 (image=hello-world, name=xenodochial_lumiere)\n\
2022-11-07T11:28:27.971060500Z container die e0c3c6120c29beb393e4b92773c9aa60006747bddabd352b77bf0b4ad23747a7 (exitCode=0, image=hello-world, name=xenodochial_lumiere)",

"",

"2022-11-07T20:06:36.919980700Z container create bd8d4a3338c3e9ab4ca555c6d869dc980f04f10ebdcd9284321c0afecbec1234 (image=hello-world, name=distracted_sinoussi)\n\
2022-11-07T20:06:36.927728700Z container attach bd8d4a3338c3e9ab4ca555c6d869dc980f04f10ebdcd9284321c0afecbec1234 (image=hello-world, name=distracted_sinoussi)\n\
2022-11-07T20:06:37.564947300Z container start bd8d4a3338c3e9ab4ca555c6d869dc980f04f10ebdcd9284321c0afecbec1234 (image=hello-world, name=distracted_sinoussi)\n\
2022-11-07T20:06:37.596428500Z container die bd8d4a3338c3e9ab4ca555c6d869dc980f04f10ebdcd9284321c0afecbec1234 (exitCode=0, image=hello-world, name=distracted_sinoussi)",

"Nov  7 21:54:24 X-PC sudo: john : TTY=pts/7 ; PWD=/home/john ; USER=root ; COMMAND=/usr/bin/docker run hello-world\n\
Nov  7 21:54:24 X-PC sudo: john : TTY=pts/7 ; PWD=/home/john ; USER=root ; COMMAND=/usr/bin/docker run hello-world\n"
};
static int test_search_keyword(){
    int errors = 0;
    fprintf(stderr, "%s():\n", __FUNCTION__);

    for(int i = 0; i < (int) (sizeof(regex_src) / sizeof(regex_src[0])); i++){
        regex_t *regex_c = regex_pat_str[i] ? mallocz(sizeof(regex_t)) : NULL;
        if(regex_c && regcomp(  regex_c, regex_pat_str[i], 
                                regex_ignore_case[i] ? REG_EXTENDED | REG_NEWLINE | REG_ICASE : REG_EXTENDED | REG_NEWLINE))
                    fatal("Could not compile regular expression:%s", regex_pat_str[i]);

        size_t regex_src_sz = strlen(regex_src[i]) + 1;
        char *res = callocz(1 , regex_src_sz);
        size_t res_sz;
        int matches = search_keyword( regex_src[i], regex_src_sz, 
                                      res, &res_sz, 
                                      regex_keyword[i], regex_c, 
                                      regex_ignore_case[i]);
        // fprintf(stderr, "\nMatches:%d\nResults:\n%.*s\n", matches, (int) res_sz, res);
        if(regex_exp_matches[i] != matches){
            fprintf(stderr, "- Error in matches returned from search_keyword() for: regex_src[%d]\n", i);
            ++errors;
        };
        if(strncmp(regex_exp_dst[i], res, res_sz - 1)){
            fprintf(stderr, "- Error in strncmp() of results from search_keyword() for: regex_src[%d]\n", i);
            ++errors;
        }
        
        if(regex_c) freez(regex_c);
        freez(res);
    }

    fprintf(stderr, "%s\n", errors ? "FAIL" : "OK");
    return errors;
}

static Flb_socket_config_t *p_forward_in_config = NULL;

static flb_srvc_config_t flb_srvc_config = {
    .flush           = FLB_FLUSH_DEFAULT,
    .http_listen     = FLB_HTTP_LISTEN_DEFAULT,
    .http_port       = FLB_HTTP_PORT_DEFAULT,
    .http_server     = FLB_HTTP_SERVER_DEFAULT,
    .log_path        = "NULL",
    .log_level       = FLB_LOG_LEVEL_DEFAULT,
    .coro_stack_size = FLB_CORO_STACK_SIZE_DEFAULT
};

static flb_srvc_config_t *p_flb_srvc_config = NULL;

static int test_logsmanag_config_funcs(){
    int errors = 0, rc;
    fprintf(stderr, "%s():\n", __FUNCTION__);

    fprintf(stderr, "Testing get_X_dir() functions...\n"); 
    if(NULL == get_user_config_dir()){
        fprintf(stderr, "- Error, get_user_config_dir() returns NULL.\n");
        ++errors;
    }

    if(NULL == get_stock_config_dir()){
        fprintf(stderr, "- Error, get_stock_config_dir() returns NULL.\n");
        ++errors;
    }
    
    if(NULL == get_log_dir()){
        fprintf(stderr, "- Error, get_log_dir() returns NULL.\n");
        ++errors;
    }
    
    if(NULL == get_cache_dir()){
        fprintf(stderr, "- Error, get_cache_dir() returns NULL.\n");
        ++errors;
    }

    fprintf(stderr, "Testing logs_manag_config_load() when p_flb_srvc_config is NULL...\n"); 

    SUPRESS_STDERR();
    rc = logs_manag_config_load(p_flb_srvc_config, &p_forward_in_config, 1);
    UNSUPRESS_STDERR();

    if(LOGS_MANAG_CONFIG_LOAD_ERROR_P_FLB_SRVC_NULL != rc){
        fprintf(stderr, "- Error, logs_manag_config_load() returns %d.\n", rc);
        ++errors;
    } 

    p_flb_srvc_config = &flb_srvc_config;

    fprintf(stderr, "Testing logs_manag_config_load() can load stock config...\n"); 

    SUPRESS_STDERR();
    rc = logs_manag_config_load(&flb_srvc_config, &p_forward_in_config, 1);
    UNSUPRESS_STDERR();

    if( LOGS_MANAG_CONFIG_LOAD_ERROR_OK != rc){
        fprintf(stderr, "- Error, logs_manag_config_load() returns %d.\n", rc);
        ++errors;
    }

    fprintf(stderr, "%s\n", errors ? "FAIL" : "OK");
    return errors;
}

uv_loop_t *main_loop;

static void setup_p_file_infos_arr_and_main_loop() {
    fprintf(stderr, "%s():\n", __FUNCTION__);

    p_file_infos_arr = callocz(1, sizeof(struct File_infos_arr));
    main_loop = mallocz(sizeof(uv_loop_t));
    if(uv_loop_init(main_loop)) 
        exit(EXIT_FAILURE);

    fprintf(stderr, "OK\n");
}

static int test_flb_init(){
    int errors = 0, rc;
    fprintf(stderr, "%s():\n", __FUNCTION__);

    fprintf(stderr, "Testing flb_init() with wrong stock_config_dir...\n"); 

    SUPRESS_STDERR();
    rc = flb_init(flb_srvc_config, "/tmp", "example_prefix_");
    UNSUPRESS_STDERR();
    if(!rc){
        fprintf(stderr, "- Error, flb_init() should fail but it returns %d.\n", rc);
        ++errors;
    }

    fprintf(stderr, "Testing flb_init() with correct stock_config_dir...\n"); 

    rc = flb_init(flb_srvc_config, get_stock_config_dir(), "example_prefix_");
    if(rc){
        fprintf(stderr, "- Error, flb_init() should fail but it returns %d.\n", rc);
        ++errors;
    }

    fprintf(stderr, "%s\n", errors ? "FAIL" : "OK");
    return errors;
}

static int unlink_cb(const char *fpath, const struct stat *sb, int typeflag, struct FTW *ftwbuf){
    UNUSED(sb);
    UNUSED(typeflag);
    UNUSED(ftwbuf);

    return remove(fpath);
}

static int test_db_init(){
    int errors = 0;
    fprintf(stderr, "%s():\n", __FUNCTION__);

    extern netdata_mutex_t stdout_mut;

    SUPRESS_STDOUT();
    SUPRESS_STDERR();
    config_file_load(main_loop, p_forward_in_config, &flb_srvc_config, &stdout_mut);
    UNSUPRESS_STDOUT();
    UNSUPRESS_STDERR();

    fprintf(stderr, "Testing db_init() with main_db_dir == NULL...\n"); 

    SUPRESS_STDERR();
    db_set_main_dir(NULL);
    int rc = db_init();
    UNSUPRESS_STDERR();
    
    if(!rc){
        fprintf(stderr, "- Error, db_init() returns %d even though db_set_main_dir(NULL); was called.\n", rc);
        ++errors;
    }

    char tmpdir[] = "/tmp/tmpdir.XXXXXX";
    char *main_db_dir = mkdtemp (tmpdir);
    fprintf(stderr, "Testing db_init() with main_db_dir == %s...\n", main_db_dir); 

    SUPRESS_STDERR();
    db_set_main_dir(main_db_dir);
    rc = db_init();
    UNSUPRESS_STDERR();

    if(rc){
        fprintf(stderr, "- Error, db_init() returns %d.\n", rc);
        ++errors;
    }

    fprintf(stderr, "Cleaning up %s...\n", main_db_dir);

    if(nftw(main_db_dir, unlink_cb, 64, FTW_DEPTH | FTW_PHYS) == -1){
        fprintf(stderr, "Error while remove path:%s. Will exit...\n", strerror(errno)); 
        exit(EXIT_FAILURE);
    }

    fprintf(stderr, "%s\n", errors ? "FAIL" : "OK");
    return errors;
}

int logs_management_unittest(void){
    int errors = 0;

    fprintf(stderr, "\n\n======================================================\n");
    fprintf(stderr, "         ** Starting logs management tests **\n");
    fprintf(stderr, "======================================================\n");
    fprintf(stderr, "------------------------------------------------------\n");
    errors += test_compression_decompression();
    fprintf(stderr, "------------------------------------------------------\n");
    errors += test_read_last_line();
    fprintf(stderr, "------------------------------------------------------\n");
    setup_parse_config_expected_num_fields();
    fprintf(stderr, "------------------------------------------------------\n");
    errors += test_count_fields();
    fprintf(stderr, "------------------------------------------------------\n");
    errors += test_auto_detect_web_log_parser_config();
    fprintf(stderr, "------------------------------------------------------\n");
    errors += test_parse_web_log_line();
    fprintf(stderr, "------------------------------------------------------\n");
    errors += test_sanitise_string();
    fprintf(stderr, "------------------------------------------------------\n");
    errors += test_search_keyword();
    fprintf(stderr, "------------------------------------------------------\n");
    errors += test_logsmanag_config_funcs();
    fprintf(stderr, "------------------------------------------------------\n");
    setup_p_file_infos_arr_and_main_loop();
    fprintf(stderr, "------------------------------------------------------\n");
    errors += test_flb_init();
    fprintf(stderr, "------------------------------------------------------\n");
    errors += test_db_init();
    fprintf(stderr, "------------------------------------------------------\n");
    fprintf(stderr, "[%s] Total errors: %d\n", errors ? "FAILED" : "SUCCEEDED", errors);
    fprintf(stderr, "======================================================\n");
    fprintf(stderr, "         ** Finished logs management tests **\n");
    fprintf(stderr, "======================================================\n");
    fflush(stderr);

    return errors;
}
