/** @file unit_test.h
 *  @brief Includes unit tests for the logs management project
 *
 *  @author Dimitris Pantazis
 */

#include "unit_test.h"
#include <stdlib.h>
#include <stdio.h>
#include "../circular_buffer.h"
#include "../compression.h"
#include "../parser.h"
#include "../query.h"

#define SEVERAL_LOG_RECORDS "\
127.0.0.1 - - [30/Jun/2022:16:43:51 +0300] \"GET / HTTP/1.0\" 200 11192 \"-\" \"ApacheBench/2.3\"\n\
192.168.2.1 - - [30/Jun/2022:16:43:51 +0300] \"PUT / HTTP/1.0\" 400 11192 \"-\" \"ApacheBench/2.3\"\n"

static int test_compression_decompression() {
    int errors = 0;
    fprintf(stderr, "%s():\n", __FUNCTION__);

    Circ_buff_item_t item;
    item.text_size = sizeof(SEVERAL_LOG_RECORDS);
    fprintf(stderr, "Testing LZ4_compressBound()...\n");
    size_t required_compressed_space  = LZ4_compressBound(item.text_size);
    if(!required_compressed_space){
        fprintf(stderr, "- Error while using LZ4_compressBound()\n");
        return ++errors;
    }

    item.data_max_size = item.text_size + required_compressed_space;
    item.data = mallocz(item.data_max_size);
    memcpy(item.data, SEVERAL_LOG_RECORDS, sizeof(SEVERAL_LOG_RECORDS));

    fprintf(stderr, "Testing LZ4_compress_fast()...\n");    
    item.text_compressed = item.data + item.text_size;

    item.text_compressed_size = LZ4_compress_fast(  item.data, item.text_compressed, 
                                                    item.text_size, required_compressed_space, 1);
    if(!item.text_compressed_size){
        fprintf(stderr, "- Error while using LZ4_compress_fast()\n");
        return ++errors;                      
    }

    char *decompressed_text = mallocz(item.text_size);
    if(decompress_text(&item, decompressed_text) <= 0){
        fprintf(stderr, "- Error in decompress_text()\n");
        return ++errors;
    }

    if(memcmp(item.data, decompressed_text, item.text_size)){
        fprintf(stderr, "- Error, original and decompressed data not the same\n");
        ++errors;
    }

    fprintf(stderr, "%s", errors ? "" : "OK\n");
    return errors;
}

const char * const parse_configs_to_test[] = {
    "127.0.0.1 - - [30/Jun/2022:16:43:51 +0300] \"GET / HTTP/1.0\" 200 11228 \"-\" \"ApacheBench/2.3\"\n",      /* Apache csvCombined 1 */
    "::1 - - [01/Sep/2022:19:04:42 +0100] \"GET / HTTP/1.1\" 200 3477 \"-\" \"Mozilla/5.0 (Windows NT 10.0; \
Win64; x64; rv:103.0) Gecko/20100101 Firefox/103.0\"\n",                                                          /* Apache csvCombined 2 */
    "XPS-wsl.localdomain:80 ::1 - - [30/Jun/2022:20:59:29 +0300] \"GET / HTTP/1.1\" 200 3477 \"-\" \"Mozilla\
/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/103.0.5060.53 Safari/537.36\
 Edg/103.0.1264.37\"\n",                                                                                        /* Apache csvVhostCombined */
    "127.0.0.1 - - [30/Jun/2022:16:43:51 +0300] \"GET / HTTP/1.0\" 200 11228\n",                                /* Apache csvCommon */
    "XPS-wsl.localdomain:80 127.0.0.1 - - [30/Jun/2022:16:43:51 +0300] \"GET / HTTP/1.0\" 200 11228\n",         /* Apache csvVhostCommon 1 */
    "XPS-wsl.localdomain:80 2001:0db8:85a3:0000:0000:8a2e:0370:7334 - - [30/Jun/2022:16:43:51 +0300] \"GET /\
 HTTP/1.0\" 200 11228\n",                                                                                       /* Apache csvVhostCommon 2 */
    "47.29.201.179 - - [28/Feb/2019:13:17:10 +0000] \"GET /?p=1 HTTP/2.0\" 200 5316 \"https://dot.com/?p=1\"\
 \"Mozilla/5.0 (Windows NT 6.1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/72.0.3626.119 Safari/537.36\"\n", /* Nginx csvCombined */
};
const web_log_line_field_t parse_config_expected[][15] = {
    {REQ_CLIENT     , CUSTOM    , CUSTOM, TIME  , TIME, REQ , RESP_CODE, RESP_SIZE, CUSTOM   , CUSTOM ,     -1, -1, -1, -1, -1}, /* Apache csvCombined 1 */
    {REQ_CLIENT     , CUSTOM    , CUSTOM, TIME  , TIME, REQ , RESP_CODE, RESP_SIZE, CUSTOM   , CUSTOM ,     -1, -1, -1, -1, -1}, /* Apache csvCombined 2 */
    {VHOST_WITH_PORT, REQ_CLIENT, CUSTOM, CUSTOM, TIME, TIME, REQ      , RESP_CODE, RESP_SIZE, CUSTOM , CUSTOM, -1, -1, -1, -1}, /* Apache csvVhostCombined */
    {REQ_CLIENT     , CUSTOM    , CUSTOM, TIME  , TIME, REQ , RESP_CODE, RESP_SIZE, -1       , -1     ,     -1, -1, -1, -1, -1}, /* Apache csvCommon */
    {VHOST_WITH_PORT, REQ_CLIENT, CUSTOM, CUSTOM, TIME, TIME, REQ      , RESP_CODE, RESP_SIZE, -1     ,     -1, -1, -1, -1, -1}, /* Apache csvVhostCommon 1 */
    {VHOST_WITH_PORT, REQ_CLIENT, CUSTOM, CUSTOM, TIME, TIME, REQ      , RESP_CODE, RESP_SIZE, -1     ,     -1, -1, -1, -1, -1}, /* Apache csvVhostCommon 2 */
    {REQ_CLIENT     , CUSTOM    , CUSTOM, TIME  , TIME, REQ,  RESP_CODE, RESP_SIZE, CUSTOM   , CUSTOM ,     -1, -1, -1, -1, -1}, /* Nginx csvCombined */
}; 
int *parse_config_expected_num_fields = NULL;
const char parse_config_delim = ' ';

static int test_auto_detect_web_log_parser_config() {
    int errors = 0;
    fprintf(stderr, "%s():\n", __FUNCTION__);

    for(int i = 0; i < (int) (sizeof(parse_configs_to_test) / sizeof(parse_configs_to_test[0])); i++){
        parse_config_expected_num_fields = reallocz(parse_config_expected_num_fields, (i + 1) * sizeof(int));
        parse_config_expected_num_fields[i] = 0;
        for(int j = 0; (int) parse_config_expected[i][j] != -1; j++){
            parse_config_expected_num_fields[i]++;
        }
    }

    for(int i = 0; i < (int) (sizeof(parse_configs_to_test) / sizeof(parse_configs_to_test[0])); i++){
        char *line = strdupz(parse_configs_to_test[i]);
        line[strlen(line) - 1] = 0;
        Web_log_parser_config_t *wblp_conf = auto_detect_web_log_parser_config(line, parse_config_delim);
        if(!wblp_conf){
            fprintf(stderr, "- Error during auto_detect_web_log_parser_config() (NULL wblp_conf) for:\n%s log record \n", line);
            ++errors;
        } else if(wblp_conf->num_fields != parse_config_expected_num_fields[i]){
            fprintf(stderr, "- Error during auto_detect_web_log_parser_config() (number of fields mismatch) for:\n%s log record \n", line);
            fprintf(stderr, "Expected %d fields but auto-detected %d\n", parse_config_expected_num_fields[i], wblp_conf->num_fields);
            ++errors;
        } else {
            for(int j = 0; (int) parse_config_expected[i][j] != -1; j++){
                if(wblp_conf->fields[j] != parse_config_expected[i][j]){
                    fprintf(stderr, "- Error during auto_detect_web_log_parser_config() (field type mismatch) for:\n%s log record \n", line);
                    ++errors;
                    break;
                }
            }
        }

        freez(line);
        if(wblp_conf) freez(wblp_conf->fields);
        freez(wblp_conf);
    }

    fprintf(stderr, "%s", errors ? "" : "OK\n");
    return errors;
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

    fprintf(stderr, "%s", errors ? "" : "OK\n");
    return errors;
}

int test_logs_management(int argc, char *argv[]){
    (void) argc;
    (void) argv;
    int errors = 0;

    fprintf(stderr, "\n\n======================================================\n");
    fprintf(stderr, "         ** Starting logs management tests **\n");
    fprintf(stderr, "======================================================\n");
    fprintf(stderr, "------------------------------------------------------\n");
    errors += test_compression_decompression();
    fprintf(stderr, "------------------------------------------------------\n");
    errors += test_auto_detect_web_log_parser_config();
    fprintf(stderr, "------------------------------------------------------\n");
    errors += test_sanitise_string();
    fprintf(stderr, "------------------------------------------------------\n");
    fprintf(stderr, "Total errors: %d\n", errors);
    fprintf(stderr, "======================================================\n");
    fprintf(stderr, "         ** Finished logs management tests **\n");
    fprintf(stderr, "======================================================\n");

    return errors;
}