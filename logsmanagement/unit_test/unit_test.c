// SPDX-License-Identifier: GPL-3.0-or-later

/** @file unit_test.h
 *  @brief Includes unit tests for the Logs Management project
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
    errors += test_search_keyword();
    fprintf(stderr, "------------------------------------------------------\n");
    fprintf(stderr, "[%s] Total errors: %d\n", errors ? "FAILED" : "SUCCEEDED", errors);
    fprintf(stderr, "======================================================\n");
    fprintf(stderr, "         ** Finished logs management tests **\n");
    fprintf(stderr, "======================================================\n");
    fflush(stderr);

    return errors;
}
