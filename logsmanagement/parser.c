// SPDX-License-Identifier: GPL-3.0-or-later

/** @file parser.c
 *  @brief API to parse and search logs
 */

#if !defined(_XOPEN_SOURCE) && !defined(__DARWIN__) && !defined(__APPLE__) && !defined(__FreeBSD__)
/* _XOPEN_SOURCE 700 required by strptime (POSIX 2004) and strndup (POSIX 2008)
 * Will need to find a cleaner way of doing this, as currently defining
 * _XOPEN_SOURCE 700 can cause issues on Centos 7, MacOS and FreeBSD too. */
#define _XOPEN_SOURCE 700 
/* _BSD_SOURCE (glibc <= 2.19) and _DEFAULT_SOURCE (glibc >= 2.20) are required 
 * to silence "warning: implicit declaration of function ‘strsep’;" that is 
 * included through libnetdata/inlined.h.  */
#define _BSD_SOURCE
#define _DEFAULT_SOURCE 
#include <time.h>
#endif

#include "parser.h"
#include "helper.h"
#include <stdio.h>
#include <sys/resource.h>
#include <math.h>
#include <string.h>

static regex_t vhost_regex, req_client_regex, cipher_suite_regex;

const char* const csv_auto_format_guess_matrix[] = {
    "$host:$server_port $remote_addr - - [$time_local] \"$request\" $status $body_bytes_sent - - $request_length $request_time $upstream_response_time", // csvVhostCustom4
    "$host:$server_port $remote_addr - - [$time_local] \"$request\" $status $body_bytes_sent - - $request_length $request_time",                         // csvVhostCustom3
    "$host:$server_port $remote_addr - - [$time_local] \"$request\" $status $body_bytes_sent - -",                                                       // csvVhostCombined
    "$host:$server_port $remote_addr - - [$time_local] \"$request\" $status $body_bytes_sent $request_length $request_time $upstream_response_time",     // csvVhostCustom2
    "$host:$server_port $remote_addr - - [$time_local] \"$request\" $status $body_bytes_sent $request_length $request_time",                             // csvVhostCustom1
    "$host:$server_port $remote_addr - - [$time_local] \"$request\" $status $body_bytes_sent",                                                           // csvVhostCommon
    "$remote_addr - - [$time_local] \"$request\" $status $body_bytes_sent - - $request_length $request_time $upstream_response_time",                    // csvCustom4
    "$remote_addr - - [$time_local] \"$request\" $status $body_bytes_sent - - $request_length $request_time",                                            // csvCustom3
    "$remote_addr - - [$time_local] \"$request\" $status $body_bytes_sent - -",                                                                          // csvCombined
    "$remote_addr - - [$time_local] \"$request\" $status $body_bytes_sent $request_length $request_time $upstream_response_time",                        // csvCustom2
    "$remote_addr - - [$time_local] \"$request\" $status $body_bytes_sent $request_length $request_time",                                                // csvCustom1
    "$remote_addr - - [$time_local] \"$request\" $status $body_bytes_sent",                                                                              // csvCommon
    NULL}
;

UNIT_STATIC int count_fields(const char *line, const char delimiter){
    const char *ptr;
    int cnt, fQuote;

    for (cnt = 1, fQuote = 0, ptr = line; *ptr != '\n' && *ptr != '\r' && *ptr != '\0'; ptr++ ){
        if (fQuote) {
            if (*ptr == '\"') {
                if ( ptr[1] == '\"' ) {
                    ptr++;
                    continue;
                }
                fQuote = 0;
            }
            continue;
        }

        if(*ptr == '\"'){
            fQuote = 1;
            continue;
        }
        if(*ptr == delimiter){
            cnt++;
            while(*(ptr+1) == delimiter) ptr++;
            continue;
        }
    }

    if (fQuote) {
        return -1;
    }

    return cnt;
}

/**
 * @brief Parse a delimited string into an array of strings.
 * @details Given a string containing no linebreaks, or containing line breaks
 * which are escaped by "double quotes", extract a NULL-terminated
 * array of strings, one for every delimiter-separated value in the row.
 * @param[in] line The input string to be parsed.
 * @param[in] delimiter The delimiter to be used to split the string.
 * @param[in] num_fields The expected number of fields in \p line. If a negative
 * number is provided, they will be counted.
 * @return A NULL-terminated array of strings with the delimited values in \p line,
 * or NULL in any other case.
 * @todo This function has not been benchmarked or optimised.
 */
static inline char **parse_csv( const char *line, const char delimiter, int num_fields) {
    char **buf, **bptr, *tmp, *tptr;
    const char *ptr;
    int fQuote, fEnd;

    if(num_fields < 0){
        num_fields = count_fields(line, delimiter);

        if ( num_fields == -1 ) {
            return NULL;
        }
    }

    buf = mallocz( sizeof(char*) * (num_fields+1) );

    tmp = mallocz( strlen(line) + 1 );

    bptr = buf;

    for ( ptr = line, fQuote = 0, *tmp = '\0', tptr = tmp, fEnd = 0; ; ptr++ ) {
        if ( fQuote ) {
            if ( !*ptr ) {
                break;
            }

            if ( *ptr == '\"' ) {
                if ( ptr[1] == '\"' ) {
                    *tptr++ = '\"';
                    ptr++;
                    continue;
                }
                fQuote = 0;
            }
            else {
                *tptr++ = *ptr;
            }

            continue;
        }


        if(*ptr == '\"'){
            fQuote = 1;
            continue;
        }
        else if(*ptr == '\0'){
            fEnd = 1;
            *tptr = '\0';
            *bptr = strdupz( tmp );

            if ( !*bptr ) {
                for ( bptr--; bptr >= buf; bptr-- ) {
                    freez( *bptr );
                }
                freez( buf );
                freez( tmp );

                return NULL;
            }

            bptr++;
            tptr = tmp;
            break;
        }
        else if(*ptr == delimiter){
            *tptr = '\0';
            *bptr = strdupz( tmp );

            if ( !*bptr ) {
                for ( bptr--; bptr >= buf; bptr-- ) {
                    freez( *bptr );
                }
                freez( buf );
                freez( tmp );

                return NULL;
            }

            bptr++;
            tptr = tmp;

            continue;
        }
        else{
            *tptr++ = *ptr;
            continue;
        }

        if ( fEnd ) {
            break;
        }
    }

    *bptr = NULL;
    freez( tmp );
    return buf;
}

/**
 * @brief Search a buffer for a keyword (or regular expression)
 * @details Search the source buffer for a keyword (or regular expression) and 
 * copy matches to the destination buffer.
 * @param[in] src The source buffer to be searched
 * @param[in] src_sz Size of \p src
 * @param[in, out] dest The destination buffer where the results will be 
 * written out to. If NULL, the results will just be discarded.
 * @param[out] dest_sz Size of \p dest
 * @param[in] keyword The keyword or pattern to be searched in the src buffer
 * @param[in] regex The precompiled regular expression to be search in the 
 * src buffer. If NULL, \p keyword will be used instead.
 * @param[in] ignore_case Perform case insensitive search if 1.
 * @return Number of matches, or -1 in case of error
 */
int search_keyword( char *src, size_t src_sz __maybe_unused, 
                    char *dest, size_t *dest_sz, 
                    const char *keyword, regex_t *regex,
                    const int ignore_case){
    
    m_assert(src[src_sz - 1] == '\0', "src[src_sz - 1] should be '\0' but it's not");
    m_assert((dest && dest_sz) || (!dest && !dest_sz), "either both dest and dest_sz exist, or none does");

    if(unlikely(dest && !dest_sz))
        return -1;
    
    regex_t regex_compiled;
    
    if(regex) 
        regex_compiled = *regex;
    else{
        char regexString[MAX_REGEX_SIZE];
        const int regex_flags = ignore_case ? REG_EXTENDED | REG_NEWLINE | REG_ICASE : REG_EXTENDED | REG_NEWLINE;
        snprintf(regexString, MAX_REGEX_SIZE, ".*(%s).*", keyword);
        int rc;
        if (unlikely((rc = regcomp(&regex_compiled, regexString, regex_flags)))){
            size_t regcomp_err_str_size = regerror(rc, &regex_compiled, 0, 0);
            char *regcomp_err_str = mallocz(regcomp_err_str_size);
            regerror(rc, &regex_compiled, regcomp_err_str, regcomp_err_str_size);
            fatal("Could not compile regular expression:%.*s, error: %s", (int) MAX_REGEX_SIZE, regexString, regcomp_err_str);
        }
    }

    regmatch_t groupArray[1];
    int matches = 0;
    char *cursor = src;

    if(dest_sz) 
        *dest_sz = 0;

    for ( ; ; matches++){
        if (regexec(&regex_compiled, cursor, 1, groupArray, REG_NOTBOL | REG_NOTEOL)) 
            break;  // No more matches
        if (groupArray[0].rm_so == -1) 
            break;  // No more groups

        size_t match_len = (size_t) (groupArray[0].rm_eo - groupArray[0].rm_so);

        // debug_log( "Match %d [%2d-%2d]:%.*s\n", matches, groupArray[0].rm_so, 
        //         groupArray[0].rm_eo, (int) match_len, cursor + groupArray[0].rm_so);

        if(dest && dest_sz){
            memcpy( &dest[*dest_sz], cursor + groupArray[0].rm_so, match_len);
            *dest_sz += match_len + 1;
            dest[*dest_sz - 1] = '\n';
        }
        
        cursor += groupArray[0].rm_eo;
    }

    if(!regex) 
        regfree(&regex_compiled);

    return matches;
}

/**
 * @brief Extract web log parser configuration from string
 * @param[in] log_format String that describes the log format
 * @param[in] delimiter Delimiter to be used when parsing a CSV log format
 * @return Pointer to struct that contains the extracted log format 
 * configuration or NULL if no fields found in log_format.
 */
Web_log_parser_config_t *read_web_log_parser_config(const char *log_format, const char delimiter){
    int num_fields = count_fields(log_format, delimiter);
    if(num_fields <= 0) return NULL;

    /* If first execution of this function, initialise regexs */
    static int regexs_initialised = 0;

    // TODO: Tests needed for following regexs.
    if(!regexs_initialised){
        assert(regcomp(&vhost_regex, "^[a-zA-Z0-9:.-]+$", REG_NOSUB | REG_EXTENDED) == 0);
        assert(regcomp(&req_client_regex, "^([0-9a-f:.]+|localhost)$", REG_NOSUB | REG_EXTENDED) == 0);
        assert(regcomp(&cipher_suite_regex, "^[A-Z0-9_-]+$", REG_NOSUB | REG_EXTENDED) == 0);
        regexs_initialised = 1;
    }

    Web_log_parser_config_t *wblp_config = callocz(1, sizeof(Web_log_parser_config_t));
    wblp_config->num_fields = num_fields;
    wblp_config->delimiter = delimiter;
    
    char **parsed_format = parse_csv(log_format, delimiter, num_fields); // parsed_format is NULL-terminated
    wblp_config->fields = callocz(num_fields, sizeof(web_log_line_field_t));
    unsigned int fields_off = 0;

    for(int i = 0; i < num_fields; i++ ){

        if(strcmp(parsed_format[i], "$host:$server_port") == 0 || 
           strcmp(parsed_format[i], "%v:%p") == 0) {
            wblp_config->fields[fields_off++] = VHOST_WITH_PORT;
            continue;
        }

        if(strcmp(parsed_format[i], "$host") == 0 || 
           strcmp(parsed_format[i], "$http_host") == 0 ||
           strcmp(parsed_format[i], "%v") == 0) {
            wblp_config->fields[fields_off++] = VHOST;
            continue;
        }

        if(strcmp(parsed_format[i], "$server_port") == 0 || 
           strcmp(parsed_format[i], "%p") == 0) {
            wblp_config->fields[fields_off++] = PORT;
            continue;
        }

        if(strcmp(parsed_format[i], "$scheme") == 0) {
            wblp_config->fields[fields_off++] = REQ_SCHEME;
            continue;
        }

        if(strcmp(parsed_format[i], "$remote_addr") == 0 || 
           strcmp(parsed_format[i], "%a") == 0 ||
           strcmp(parsed_format[i], "%h") == 0) {
            wblp_config->fields[fields_off++] = REQ_CLIENT;
            continue;
        }

        if(strcmp(parsed_format[i], "$request") == 0 || 
           strcmp(parsed_format[i], "%r") == 0) {
            wblp_config->fields[fields_off++] = REQ;
            continue;
        }

        if(strcmp(parsed_format[i], "$request_method") == 0 || 
           strcmp(parsed_format[i], "%m") == 0) {
            wblp_config->fields[fields_off++] = REQ_METHOD;
            continue;
        }

        if(strcmp(parsed_format[i], "$request_uri") == 0 || 
           strcmp(parsed_format[i], "%U") == 0) {
            wblp_config->fields[fields_off++] = REQ_URL;
            continue;
        }

        if(strcmp(parsed_format[i], "$server_protocol") == 0 || 
           strcmp(parsed_format[i], "%H") == 0) {
            wblp_config->fields[fields_off++] = REQ_PROTO;
            continue;
        }

        if(strcmp(parsed_format[i], "$request_length") == 0 || 
           strcmp(parsed_format[i], "%I") == 0) {
            wblp_config->fields[fields_off++] = REQ_SIZE;
            continue;
        }

        if(strcmp(parsed_format[i], "$request_time") == 0 || 
           strcmp(parsed_format[i], "%D") == 0) {
            wblp_config->fields[fields_off++] = REQ_PROC_TIME;
            continue;
        }

        if(strcmp(parsed_format[i], "$status") == 0 || 
           strcmp(parsed_format[i], "%>s") == 0 ||
           strcmp(parsed_format[i], "%s") == 0) {
            wblp_config->fields[fields_off++] = RESP_CODE;
            continue;
        }

        if(strcmp(parsed_format[i], "$bytes_sent") == 0 || 
           strcmp(parsed_format[i], "$body_bytes_sent") == 0 ||
           strcmp(parsed_format[i], "%b") == 0 ||
           strcmp(parsed_format[i], "%O") == 0 ||
           strcmp(parsed_format[i], "%B") == 0) {
            wblp_config->fields[fields_off++] = RESP_SIZE;
            continue;
        }

        if(strcmp(parsed_format[i], "$upstream_response_time") == 0) {
            wblp_config->fields[fields_off++] = UPS_RESP_TIME;
            continue;
        }

        if(strcmp(parsed_format[i], "$ssl_protocol") == 0) {
            wblp_config->fields[fields_off++] = SSL_PROTO;
            continue;
        }

        if(strcmp(parsed_format[i], "$ssl_cipher") == 0) {
            wblp_config->fields[fields_off++] = SSL_CIPHER_SUITE;
            continue;
        }

        if(strcmp(parsed_format[i], "$time_local") == 0 || strcmp(parsed_format[i], "[$time_local]") == 0 ||
           strcmp(parsed_format[i], "%t") == 0 || strcmp(parsed_format[i], "[%t]") == 0) {
            wblp_config->fields = reallocz(wblp_config->fields, (num_fields + 1) * sizeof(web_log_line_field_t));
            wblp_config->fields[fields_off++] = TIME;
            wblp_config->fields[fields_off++] = TIME; // TIME takes 2 fields
            wblp_config->num_fields++;                // TIME takes 2 fields
            continue;
        }

        wblp_config->fields[fields_off++] = CUSTOM;

    }

    for(int i = 0; parsed_format[i] != NULL; i++) 
        freez(parsed_format[i]);

    freez(parsed_format);
    return wblp_config;
}

/**
 * @brief Parse a web log line to extract individual fields.
 * @param[in] wblp_config Configuration that specifies how to parse the line.
 * @param[in] line Web log record to be parsed. '\n', '\r' or '\0' terminated.
 * @param[out] log_line_parsed Struct that stores the results of parsing.
 */
void parse_web_log_line(const Web_log_parser_config_t *wblp_config, 
                        char *line, size_t line_len, 
                        Log_line_parsed_t *log_line_parsed){

    /* Read parsing configuration */
    web_log_line_field_t *fields_format = wblp_config->fields;
    const int num_fields_config = wblp_config->num_fields;
    const char delimiter = wblp_config->delimiter;
    const int verify = wblp_config->verify_parsed_logs;

    /* Consume new lines and spaces at end of line */
    for(; line[line_len-1] == '\n' || line[line_len-1] == '\r' || line[line_len-1] == ' '; line_len--);

    char *field = line;
    char *offset = line;
    size_t field_size = 0;

    for(int i = 0; i < num_fields_config; i++ ){
        
        /* Consume double quotes and extra delimiters at beginning of field */
        while(*field == '"' || *field == delimiter) field++, offset++;

        /* Find offset boundaries of next field in line */
        while(((size_t)(offset - line) < line_len) && *offset != delimiter) offset++;
        
        if(unlikely(*(offset - 1) == '"')) offset--;

        field_size = (size_t) (offset - field);

        #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
        debug_log( "Field[%d]:%.*s", i, (int)field_size, field);
        #endif

        if(fields_format[i] == CUSTOM){
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: CUSTOM or UNKNOWN):%.*s", i, (int)field_size, field);
            #endif
            goto next_item;
        }


        char *port = field;
        size_t port_size = 0;
        size_t vhost_size = 0;

        if(fields_format[i] == VHOST_WITH_PORT){
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: VHOST_WITH_PORT):%.*s", i, (int)field_size, field);
            #endif

            if(unlikely(field[0] ==  '-' && field_size == 1)){
                log_line_parsed->vhost[0] = '\0';
                log_line_parsed->port = WEB_LOG_INVALID_PORT;
                log_line_parsed->parsing_errors++;
                goto next_item;
            }

            while(*port != ':' && vhost_size < field_size) { port++; vhost_size++; }
            if(likely(vhost_size < field_size)) {
                /* ':' detected in string */
                port++; 
                port_size = field_size - vhost_size - 1;
                field_size = vhost_size; // now field represents vhost and port is separate
            }
            else {
                /* no ':' detected in string - invalid */
                log_line_parsed->vhost[0] = '\0';
                log_line_parsed->port = WEB_LOG_INVALID_PORT;
                log_line_parsed->parsing_errors++;
                goto next_item;
            }
        }

        if(fields_format[i] == VHOST_WITH_PORT || fields_format[i] == VHOST){
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: VHOST):%.*s", i, (int)field_size, field);
            #endif

            if(unlikely(field[0] ==  '-' && field_size == 1)){
                log_line_parsed->vhost[0] = '\0';
                log_line_parsed->parsing_errors++;
                goto next_item;
            }

            // TODO: Add below case in code!!!
            // nginx $host and $http_host return ipv6 in [], apache doesn't
            // TODO: TEST! This case hasn't been tested!
            // char *pch = strchr(parsed[i], ']');
            // if(pch){
            //     *pch = '\0';
            //     memmove(parsed[i], parsed[i]+1, strlen(parsed[i]));
            // }

            snprintfz(log_line_parsed->vhost, VHOST_MAX_LEN, "%.*s", (int) field_size, field);

            if(verify){
                // if(field_size >= VHOST_MAX_LEN){
                //     #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                //     collector_error("VHOST is invalid");
                //     #endif
                //     log_line_parsed->vhost[0] = '\0';
                //     log_line_parsed->parsing_errors++;
                //     goto next_item; // TODO: Not entirely right, as it will also skip PORT parsing in case of VHOST_WITH_PORT
                // }
                
                if(unlikely(regexec(&vhost_regex, log_line_parsed->vhost, 0, NULL, 0) == REG_NOMATCH)){
                    #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                    collector_error("VHOST is invalid");
                    #endif
                    // log_line_parsed->vhost[0] = 'invalid';
                    snprintf(log_line_parsed->vhost, sizeof(WEB_LOG_INVALID_HOST_STR), WEB_LOG_INVALID_HOST_STR);
                    log_line_parsed->parsing_errors++;
                }
            }

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Extracted VHOST:%s", log_line_parsed->vhost);
            #endif

            if(fields_format[i] == VHOST) goto next_item;
        }

        if(fields_format[i] == VHOST_WITH_PORT || fields_format[i] == PORT){

            if(fields_format[i] != VHOST_WITH_PORT){
                port = field;
                port_size = field_size;
            }

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: PORT):%.*s", i, (int) port_size, port);
            #endif

            if(unlikely(port[0] ==  '-' && port_size == 1)){
                log_line_parsed->port = WEB_LOG_INVALID_PORT;
                log_line_parsed->parsing_errors++;
                goto next_item;
            }

            char port_d[PORT_MAX_LEN];
            snprintfz( port_d, PORT_MAX_LEN, "%.*s", (int) port_size, port);

            if(likely(str2int(&log_line_parsed->port, port_d, 10) == STR2XX_SUCCESS)){
                if(verify){
                    if(unlikely(log_line_parsed->port < 80 || log_line_parsed->port > 49151)){
                        #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                        collector_error("PORT is invalid (<80 or >49151)");
                        #endif
                        log_line_parsed->port = WEB_LOG_INVALID_PORT;
                        log_line_parsed->parsing_errors++;
                    }
                }
            }
            else{
                #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                collector_error("Error while extracting PORT from string");
                #endif
                log_line_parsed->port = WEB_LOG_INVALID_PORT;
                log_line_parsed->parsing_errors++;
            }
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Extracted PORT:%d", log_line_parsed->port);
            #endif

            goto next_item;
        }

        if(fields_format[i] == REQ_SCHEME){
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: REQ_SCHEME):%.*s", i, (int)field_size, field);
            #endif

            if(unlikely(field[0] ==  '-' && field_size == 1)){
                log_line_parsed->req_scheme[0] = '\0';
                log_line_parsed->parsing_errors++;
                goto next_item;
            }

            snprintfz(log_line_parsed->req_scheme, REQ_SCHEME_MAX_LEN, "%.*s", (int) field_size, field); 

            if(verify){
                if(unlikely( strcmp(log_line_parsed->req_scheme, "http") && 
                             strcmp(log_line_parsed->req_scheme, "https"))){
                    #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                    collector_error("REQ_SCHEME is invalid (must be either 'http' or 'https')");
                    #endif
                    log_line_parsed->req_scheme[0] = '\0';
                    log_line_parsed->parsing_errors++;
                }
            }
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Extracted REQ_SCHEME:%s", log_line_parsed->req_scheme);
            #endif
            goto next_item;
        }

        if(fields_format[i] == REQ_CLIENT){
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: REQ_CLIENT):%.*s", i, (int)field_size, field);
            #endif

            if(unlikely(field[0] ==  '-' && field_size == 1)){
                log_line_parsed->req_client[0] = '\0';
                log_line_parsed->parsing_errors++;
                goto next_item;
            }

            snprintfz(log_line_parsed->req_client, REQ_CLIENT_MAX_LEN, "%.*s", (int)field_size, field);

            if(verify){
                int regex_rc = regexec(&req_client_regex, log_line_parsed->req_client, 0, NULL, 0);
                if (likely(regex_rc == 0)) {/* do nothing */}
                else if (unlikely(regex_rc == REG_NOMATCH)) {
                    #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                    collector_error("REQ_CLIENT is invalid");
                    #endif
                    snprintf(log_line_parsed->req_client, REQ_CLIENT_MAX_LEN, "%s", WEB_LOG_INVALID_CLIENT_IP_STR);
                    log_line_parsed->parsing_errors++;
                }
                else {
                    size_t err_msg_size = regerror(regex_rc, &req_client_regex, NULL, 0);
                    char *err_msg = mallocz(err_msg_size);
                    regerror(regex_rc, &req_client_regex, err_msg, err_msg_size);
                    collector_error("req_client_regex error:%s", err_msg);
                    freez(err_msg);
                    m_assert(0, "req_client_regex has failed");
                }
            }

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Extracted REQ_CLIENT:%s", log_line_parsed->req_client);
            #endif

            goto next_item;
        }

        if(fields_format[i] == REQ || fields_format[i] == REQ_METHOD){

            /* If fields_format[i] == REQ, then field is filled in with request in the previous code */

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: REQ or REQ_METHOD):%.*s", i, (int)field_size, field);
            #endif

            snprintfz( log_line_parsed->req_method, REQ_METHOD_MAX_LEN, "%.*s", (int)field_size, field); 

            if(verify){
                if( unlikely( 
                        /* GET and POST are the most common requests, so check them first */
                        strcmp(log_line_parsed->req_method, "GET") &&
                        strcmp(log_line_parsed->req_method, "POST") &&

                        strcmp(log_line_parsed->req_method, "ACL") &&
                        strcmp(log_line_parsed->req_method, "BASELINE-CONTROL") &&
                        strcmp(log_line_parsed->req_method, "BIND") &&
                        strcmp(log_line_parsed->req_method, "CHECKIN") &&
                        strcmp(log_line_parsed->req_method, "CHECKOUT") &&
                        strcmp(log_line_parsed->req_method, "CONNECT") &&
                        strcmp(log_line_parsed->req_method, "COPY") &&
                        strcmp(log_line_parsed->req_method, "DELETE") &&
                        strcmp(log_line_parsed->req_method, "HEAD") &&
                        strcmp(log_line_parsed->req_method, "LABEL") &&
                        strcmp(log_line_parsed->req_method, "LINK") &&
                        strcmp(log_line_parsed->req_method, "LOCK") &&
                        strcmp(log_line_parsed->req_method, "MERGE") &&
                        strcmp(log_line_parsed->req_method, "MKACTIVITY") &&
                        strcmp(log_line_parsed->req_method, "MKCALENDAR") &&
                        strcmp(log_line_parsed->req_method, "MKCOL") &&
                        strcmp(log_line_parsed->req_method, "MKREDIRECTREF") &&
                        strcmp(log_line_parsed->req_method, "MKWORKSPACE") &&
                        strcmp(log_line_parsed->req_method, "MOVE") &&
                        strcmp(log_line_parsed->req_method, "OPTIONS") &&
                        strcmp(log_line_parsed->req_method, "ORDERPATCH") &&
                        strcmp(log_line_parsed->req_method, "PATCH") &&
                        strcmp(log_line_parsed->req_method, "PRI") &&
                        strcmp(log_line_parsed->req_method, "PROPFIND") &&
                        strcmp(log_line_parsed->req_method, "PROPPATCH") &&
                        strcmp(log_line_parsed->req_method, "PUT") &&
                        strcmp(log_line_parsed->req_method, "REBIND") &&
                        strcmp(log_line_parsed->req_method, "REPORT") &&
                        strcmp(log_line_parsed->req_method, "SEARCH") &&
                        strcmp(log_line_parsed->req_method, "TRACE") &&
                        strcmp(log_line_parsed->req_method, "UNBIND") &&
                        strcmp(log_line_parsed->req_method, "UNCHECKOUT") &&
                        strcmp(log_line_parsed->req_method, "UNLINK") &&
                        strcmp(log_line_parsed->req_method, "UNLOCK") &&
                        strcmp(log_line_parsed->req_method, "UPDATE") &&
                        strcmp(log_line_parsed->req_method, "UPDATEREDIRECTREF") &&
                        strcmp(log_line_parsed->req_method, "-"))) {

                    #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                    collector_error("REQ_METHOD is invalid");
                    #endif
                    log_line_parsed->req_method[0] = '\0';
                    log_line_parsed->parsing_errors++;
                }
            }
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Extracted REQ_METHOD:%s", log_line_parsed->req_method);
            #endif
            
            if(fields_format[i] == REQ && field[0] !=  '-') {
                while(*(offset + 1) == delimiter) offset++; // Consume extra whitespace characters
                field = ++offset; 
                while(*offset != delimiter && ((size_t)(offset - line) < line_len)) offset++;
                field_size = (size_t) (offset - field);
            } 
            else goto next_item;
        }

        if(fields_format[i] == REQ || fields_format[i] == REQ_URL){
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: REQ or REQ_URL):%.*s", i, (int)field_size, field);
            #endif

            snprintfz( log_line_parsed->req_URL, REQ_URL_MAX_LEN, "%.*s", (int)field_size, field);

            // if(unlikely(field[0] ==  '-' && field_size == 1)){
            //     log_line_parsed->req_method[0] = '\0';
            //     log_line_parsed->parsing_errors++;
            // }

            //if(verify){} ??

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG            
            debug_log( "Extracted REQ_URL:%s", log_line_parsed->req_URL ? log_line_parsed->req_URL : "NULL!");
            #endif

            if(fields_format[i] == REQ) {
                while(*(offset + 1) == delimiter) offset++; // Consume extra whitespace characters
                field = ++offset; 
                while(*offset != delimiter && ((size_t)(offset - line) < line_len)) offset++;
                field_size = (size_t) (offset - field);
            } 
            else goto next_item;
        }

        if(fields_format[i] == REQ || fields_format[i] == REQ_PROTO){

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: REQ or REQ_PROTO):%.*s", i, (int)field_size, field);
            #endif

            if(unlikely(field[0] ==  '-' && field_size == 1)){
                log_line_parsed->req_proto[0] = '\0';
                log_line_parsed->parsing_errors++;
                goto next_item;
            }

            if(unlikely( field_size > REQ_PROTO_PREF_SIZE + REQ_PROTO_MAX_LEN - 1)){
                field_size = REQ_PROTO_PREF_SIZE + REQ_PROTO_MAX_LEN - 1;
            }

            size_t req_proto_num_size = field_size - REQ_PROTO_PREF_SIZE;

            if(verify){
                if(unlikely(field_size < 6 || 
                            req_proto_num_size == 0 || 
                            strncmp(field, "HTTP/", REQ_PROTO_PREF_SIZE) ||
                            (   strncmp(&field[REQ_PROTO_PREF_SIZE], "1", req_proto_num_size) && 
                                strncmp(&field[REQ_PROTO_PREF_SIZE], "1.0", req_proto_num_size) && 
                                strncmp(&field[REQ_PROTO_PREF_SIZE], "1.1", req_proto_num_size) && 
                                strncmp(&field[REQ_PROTO_PREF_SIZE], "2", req_proto_num_size) && 
                                strncmp(&field[REQ_PROTO_PREF_SIZE], "2.0", req_proto_num_size)))) {
                    #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                    collector_error("REQ_PROTO is invalid");
                    #endif
                    log_line_parsed->req_proto[0] = '\0';
                    log_line_parsed->parsing_errors++;
                }
                else snprintfz(  log_line_parsed->req_proto, req_proto_num_size + 1, 
                                "%.*s", (int)req_proto_num_size, &field[REQ_PROTO_PREF_SIZE]); 
            }
            else snprintfz(  log_line_parsed->req_proto, req_proto_num_size + 1, 
                            "%.*s", (int)req_proto_num_size, &field[REQ_PROTO_PREF_SIZE]); 

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Extracted REQ_PROTO:%s", log_line_parsed->req_proto);
            #endif

            goto next_item;
        }

        if(fields_format[i] == REQ_SIZE){
            /* TODO: Differentiate between '-' or 0 and an invalid request size. 
             * right now, all these will set req_size == 0 */
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: REQ_SIZE):%.*s", i, (int)field_size, field);
            #endif

            char req_size_d[REQ_SIZE_MAX_LEN];
            snprintfz( req_size_d, REQ_SIZE_MAX_LEN, "%.*s", (int) field_size, field);

            if(field[0] ==  '-' && field_size == 1) { 
                log_line_parsed->req_size = 0; // Request size can be '-' 
            }
            else if(likely(str2int(&log_line_parsed->req_size, req_size_d, 10) == STR2XX_SUCCESS)){
                if(verify){
                    if(unlikely(log_line_parsed->req_size < 0)){
                        #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                        collector_error("REQ_SIZE is invalid (<0)");
                        #endif
                        log_line_parsed->req_size = 0;
                        log_line_parsed->parsing_errors++;
                    }
                }
            }
            else{
                collector_error("Error while extracting REQ_SIZE from string");
                log_line_parsed->req_size = 0;
                log_line_parsed->parsing_errors++;
            }
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Extracted REQ_SIZE:%d", log_line_parsed->req_size);
            #endif

            goto next_item;
        }

        if(fields_format[i] == REQ_PROC_TIME){
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: REQ_PROC_TIME):%.*s", i, (int)field_size, field);
            #endif

            if(unlikely(field[0] ==  '-' && field_size == 1)){
                log_line_parsed->req_proc_time = WEB_LOG_INVALID_PORT;
                log_line_parsed->parsing_errors++;
                goto next_item;
            }

            float f = 0;

            char req_proc_time_d[REQ_PROC_TIME_MAX_LEN];
            snprintfz( req_proc_time_d, REQ_PROC_TIME_MAX_LEN, "%.*s", (int) field_size, field);

            if(memchr(field, '.', field_size)){ // nginx time is in seconds with a milliseconds resolution.
                if(likely(str2float(&f, req_proc_time_d) == STR2XX_SUCCESS)){ 
                    log_line_parsed->req_proc_time = (int) (f * 1.0E6);
                }
                else { 
                    #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                    collector_error("Error while extracting REQ_PROC_TIME from string");
                    #endif
                    log_line_parsed->req_proc_time = 0;
                    log_line_parsed->parsing_errors++;
                }
            }
            else{ // apache time is in microseconds
                if(unlikely(str2int(&log_line_parsed->req_proc_time, req_proc_time_d, 10) != STR2XX_SUCCESS)) {
                    #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                    collector_error("Error while extracting REQ_PROC_TIME from string");
                    #endif
                    log_line_parsed->req_proc_time = 0;
                    log_line_parsed->parsing_errors++;
                }
            }

            if(verify){
                if(unlikely(log_line_parsed->req_proc_time < 0)){
                    #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                    collector_error("REQ_PROC_TIME is invalid (<0)");
                    #endif
                    log_line_parsed->req_proc_time = 0;
                    log_line_parsed->parsing_errors++;
                }
            }
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Extracted REQ_PROC_TIME:%d", log_line_parsed->req_proc_time);
            #endif

            goto next_item;
        }

        if(fields_format[i] == RESP_CODE){
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: RESP_CODE):%.*s\n", i, (int)field_size, field);
            #endif

            if(unlikely(field[0] ==  '-' && field_size == 1)){
                log_line_parsed->resp_code = 0;
                log_line_parsed->parsing_errors++;
                goto next_item;
            }

            char resp_code_d[REQ_RESP_CODE_MAX_LEN];
            snprintfz( resp_code_d, REQ_RESP_CODE_MAX_LEN, "%.*s", (int)field_size, field);

            if(likely(str2int(&log_line_parsed->resp_code, resp_code_d, 10) == STR2XX_SUCCESS)){  
                if(verify){
                    /* rfc7231
                     * Informational responses (100–199),
                     * Successful responses (200–299),
                     * Redirects (300–399),
                     * Client errors (400–499),
                     * Server errors (500–599). */
                    if(unlikely(log_line_parsed->resp_code < 100 || log_line_parsed->resp_code > 599)){
                        #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                        collector_error("RESP_CODE is invalid (<100 or >599)");
                        #endif
                        log_line_parsed->resp_code = 0;
                        log_line_parsed->parsing_errors++;
                    }
                }
            }
            else{ 
                #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                collector_error("Error while extracting RESP_CODE from string");
                #endif
                log_line_parsed->resp_code = 0;
                log_line_parsed->parsing_errors++;
            }
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Extracted RESP_CODE:%d", log_line_parsed->resp_code);
            #endif

            goto next_item;
        }

        if(fields_format[i] == RESP_SIZE){
            /* TODO: Differentiate between '-' or 0 and an invalid response size. 
             * right now, all these will set resp_size == 0 */
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: RESP_SIZE):%.*s", i, (int)field_size, field);
            #endif

            char resp_size_d[REQ_RESP_SIZE_MAX_LEN];
            snprintfz( resp_size_d, REQ_RESP_SIZE_MAX_LEN, "%.*s", (int)field_size, field);

            if(field[0] ==  '-' && field_size == 1) { 
                log_line_parsed->resp_size = 0; // Response size can be '-' 
            }
            else if(likely(str2int(&log_line_parsed->resp_size, resp_size_d, 10) == STR2XX_SUCCESS)){
                if(verify){
                    if(unlikely(log_line_parsed->resp_size < 0)){
                        #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                        collector_error("RESP_SIZE is invalid (<0)");
                        #endif
                        log_line_parsed->resp_size = 0;
                        log_line_parsed->parsing_errors++;
                    }
                }
            }
            else {
                #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                collector_error("Error while extracting RESP_SIZE from string");
                #endif
                log_line_parsed->resp_size = 0;
                log_line_parsed->parsing_errors++;
            }
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Extracted RESP_SIZE:%d", log_line_parsed->resp_size);
            #endif

            goto next_item;
        }

        if(fields_format[i] == UPS_RESP_TIME){
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: UPS_RESP_TIME):%.*s", i, (int)field_size, field);
            #endif

            if(field[0] ==  '-' && field_size == 1) { 
                log_line_parsed->ups_resp_time = 0;
                log_line_parsed->parsing_errors++;
                goto next_item;
            }

            /* Times of several responses are separated by commas and colons. Following the 
             * Go parser implementation, where only the first one is kept, the others are 
             * discarded. Also, there must be no space in between them. Needs testing... */
            char *pch = memchr(field, ',', field_size);
            if(pch) field_size = pch - field;

            float f = 0;

            char ups_resp_time_d[UPS_RESP_TIME_MAX_LEN];
            snprintfz( ups_resp_time_d, UPS_RESP_TIME_MAX_LEN, "%.*s", (int)field_size, field);

            if(memchr(field, '.', field_size)){ // nginx time is in seconds with a milliseconds resolution.
                if(likely(str2float(&f, ups_resp_time_d) == STR2XX_SUCCESS)){ 
                    log_line_parsed->ups_resp_time = (int) (f * 1.0E6);
                }
                else { 
                    #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                    collector_error("Error while extracting UPS_RESP_TIME from string");
                    #endif
                    log_line_parsed->ups_resp_time = 0;
                    log_line_parsed->parsing_errors++;
                }
            }
            else{ // unlike in the REQ_PROC_TIME case, apache doesn't have an equivalent here
                #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                collector_error("Error while extracting UPS_RESP_TIME from string");
                #endif
                log_line_parsed->ups_resp_time = 0;
                log_line_parsed->parsing_errors++;
            }
            if(verify){
                if(unlikely(log_line_parsed->ups_resp_time < 0)){
                    #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                    collector_error("UPS_RESP_TIME is invalid (<0)");
                    #endif
                    log_line_parsed->ups_resp_time = 0;
                    log_line_parsed->parsing_errors++;
                }
            }
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Extracted UPS_RESP_TIME:%d", log_line_parsed->ups_resp_time);
            #endif

            goto next_item;
        }

        if(fields_format[i] == SSL_PROTO){
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: SSL_PROTO):%.*s", i, (int)field_size, field);
            #endif

            if(field[0] ==  '-' && field_size == 1) { 
                log_line_parsed->ssl_proto[0] = '\0';
                log_line_parsed->parsing_errors++;
                goto next_item;
            }

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "SSL_PROTO field size:%zu", field_size);
            #endif

            snprintfz( log_line_parsed->ssl_proto, SSL_PROTO_MAX_LEN, "%.*s", (int)field_size, field); 

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "log_line_parsed->ssl_proto:%s", log_line_parsed->ssl_proto);
            #endif

            if(verify){
                if(unlikely(strcmp(log_line_parsed->ssl_proto, "TLSv1") && 
                     strcmp(log_line_parsed->ssl_proto, "TLSv1.1") &&
                     strcmp(log_line_parsed->ssl_proto, "TLSv1.2") &&
                     strcmp(log_line_parsed->ssl_proto, "TLSv1.3") &&
                     strcmp(log_line_parsed->ssl_proto, "SSLv2") &&
                     strcmp(log_line_parsed->ssl_proto, "SSLv3"))) {
                    #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                    collector_error("SSL_PROTO is invalid");
                    #endif
                    log_line_parsed->ssl_proto[0] = '\0';
                    log_line_parsed->parsing_errors++;
                }
            }

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Extracted SSL_PROTO:%s", log_line_parsed->ssl_proto);
            #endif

            goto next_item;
        }

        if(fields_format[i] == SSL_CIPHER_SUITE){
            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: SSL_CIPHER_SUITE):%.*s", i, (int)field_size, field);
            #endif

            if(field[0] ==  '-' && field_size == 1) { 
                log_line_parsed->ssl_cipher[0] = '\0';
                log_line_parsed->parsing_errors++;
            }

            snprintfz( log_line_parsed->ssl_cipher, SSL_CIPHER_SUITE_MAX_LEN, "%.*s", (int)field_size, field);

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "before: SSL_CIPHER_SUITE:%s", log_line_parsed->ssl_cipher);
            #endif

            if(verify){
                int regex_rc = regexec(&cipher_suite_regex, log_line_parsed->ssl_cipher, 0, NULL, 0);
                if (likely(regex_rc == 0)){/* do nothing */}
                else if (unlikely(regex_rc == REG_NOMATCH)) {
                    #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
                    collector_error("SSL_CIPHER_SUITE is invalid");
                    #endif
                    log_line_parsed->ssl_cipher[0] = '\0';
                    log_line_parsed->parsing_errors++;
                }
                else {
                    size_t err_msg_size = regerror(regex_rc, &cipher_suite_regex, NULL, 0);
                    char *err_msg = mallocz(err_msg_size);
                    regerror(regex_rc, &cipher_suite_regex, err_msg, err_msg_size);
                    collector_error("cipher_suite_regex error:%s", err_msg);
                    freez(err_msg);
                    m_assert(0, "cipher_suite_regex has failed");
                }
            }

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Extracted SSL_CIPHER_SUITE:%s", log_line_parsed->ssl_cipher);
            #endif

            goto next_item;
        }

        if(fields_format[i] == TIME){

            if(wblp_config->skip_timestamp_parsing){
                while(*offset != ']') offset++;
                i++;
                offset++;
                goto next_item;
            }

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Item %d (type: TIME - 1st of 2 fields):%.*s", i, (int)field_size, field);
            #endif

            // TODO: What if TIME is invalid?
            // if(field[0] ==  '-' && field_size == 1) { 
            //     log_line_parsed->timestamp = 0;
            //     log_line_parsed->parsing_errors++;
            //     ++i;
            //     goto next_item;
            // }

            char *datetime = field;

            if(memchr(datetime, '[', field_size)) {
                datetime++;
                field_size--;
            }

            struct tm ltm = {0};
            char *tz_str = strptime(datetime, "%d/%b/%Y:%H:%M:%S", &ltm);
            if(unlikely(tz_str == NULL)){
                collector_error("TIME datetime parsing failed");
                log_line_parsed->timestamp = 0;
                log_line_parsed->parsing_errors++;
                goto next_item;
            }

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "strptime() result: year:%d mon:%d day:%d hour:%d min:%d sec:%d", 
                ltm.tm_year, ltm.tm_mon, ltm.tm_mday, 
                ltm.tm_hour, ltm.tm_min, ltm.tm_sec);
            #endif

            /* Deal with 2nd part of datetime i.e. timezone */

            m_assert(*tz_str == ' ', "Invalid TIME timezone");
            ++tz_str;
            m_assert(*tz_str == '+' || *tz_str == '-', "Invalid TIME timezone");
            char tz_sign = *tz_str;

            char *tz_str_end = ++tz_str;
            while(*tz_str_end != ']') tz_str_end++;

            m_assert(tz_str_end - tz_str == 4, "Invalid TIME timezone string length");

            char tz_num[4];
            memcpy(tz_num, tz_str, tz_str_end - tz_str);

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "TIME 2nd part: %.*s", (int)(tz_str_end - tz_str), tz_str);
            #endif
            
            long int tz = strtol(tz_str, NULL, 10);
            long int tz_h = tz / 100;
            long int tz_m = tz % 100;
            int64_t tz_adj = (int64_t) tz_h * 3600 + (int64_t) tz_m * 60;
            if(tz_sign == '+') tz_adj *= -1; // if timezone is positive, we need to subtract it to get GMT

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            debug_log( "Timezone: int:%ld, hrs:%ld, mins:%ld", tz, tz_h, tz_m);
            #endif

            if(-1 == (log_line_parsed->timestamp = timegm(&ltm) + tz_adj)){
                collector_error("TIME datetime parsing failed");
                log_line_parsed->timestamp = 0;
                log_line_parsed->parsing_errors++;
            }

            #if ENABLE_PARSE_WEB_LOG_LINE_DEBUG
            char tb[80];
            strftime(tb, sizeof(tb), "%c", &ltm );
            debug_log( "Extracted TIME:%ld", log_line_parsed->timestamp);
            debug_log( "Extracted TIME string:%s", tb);
            #endif

            offset = tz_str_end + 1; // WARNING! this modifies the offset but it is required in the TIME case.
            ++i; // TIME takes up 2 fields_format[] spaces, so skip the next one

            goto next_item;
        }

next_item:
        /* If offset is located beyond the end of the line, terminate parsing */
        if(unlikely((size_t) (offset - line) >= line_len)) break;
        
        field = ++offset;
    }
}

/**
 * @brief Extract web log metrics from a group of web log fields.
 * @param[in] parser_config Configuration specifying how and what web log 
 * metrics to extract.
 * @param[in] line_parsed Web logs fields extracted from a web log line.
 * @param[out] metrics Web logs metrics exctracted from the \p line_parsed 
 * web log fields, using the \p parser_config configuration.
 */
void extract_web_log_metrics(Log_parser_config_t *parser_config, 
                            Log_line_parsed_t *line_parsed, 
                            Web_log_metrics_t *metrics){

    /* Extract number of parsed lines */
    /* NOTE: Commented out as it is done in flb_collect_logs_cb() now. */
    // metrics->num_lines++;

    /* Extract vhost */
    // TODO: Reduce number of reallocs
    if((parser_config->chart_config & CHART_VHOST) && *line_parsed->vhost){
        int i;
        for(i = 0; i < metrics->vhost_arr.size; i++){
            if(!strcmp(metrics->vhost_arr.vhosts[i].name, line_parsed->vhost)){
                metrics->vhost_arr.vhosts[i].count++;
                break;
            }
        }
        if(metrics->vhost_arr.size == i){ // Vhost not found in array - need to append
            metrics->vhost_arr.size++;
            if(metrics->vhost_arr.size >= metrics->vhost_arr.size_max){
                metrics->vhost_arr.size_max = metrics->vhost_arr.size * VHOST_BUFFS_SCALE_FACTOR + 1;
                metrics->vhost_arr.vhosts = reallocz( metrics->vhost_arr.vhosts, 
                                                      metrics->vhost_arr.size_max * sizeof(struct log_parser_metrics_vhost));
            }
            snprintf(metrics->vhost_arr.vhosts[metrics->vhost_arr.size - 1].name, VHOST_MAX_LEN, "%s", line_parsed->vhost);
            metrics->vhost_arr.vhosts[metrics->vhost_arr.size - 1].count = 1;
        }
    }

    /* Extract port */
    // TODO: Reduce number of reallocs
    if((parser_config->chart_config & CHART_PORT) && line_parsed->port){
        int i;
        for(i = 0; i < metrics->port_arr.size; i++){
            if(metrics->port_arr.ports[i].port == line_parsed->port){
                metrics->port_arr.ports[i].count++;
                break;
            }
        }
        if(metrics->port_arr.size == i){ // Port not found in array - need to append
            metrics->port_arr.size++;
            if(metrics->port_arr.size >= metrics->port_arr.size_max){
                metrics->port_arr.size_max = metrics->port_arr.size * PORT_BUFFS_SCALE_FACTOR + 1;
                metrics->port_arr.ports = reallocz( metrics->port_arr.ports, 
                                                    metrics->port_arr.size_max * sizeof(struct log_parser_metrics_port));
            }
            if(line_parsed->port == WEB_LOG_INVALID_PORT)
                snprintfz(metrics->port_arr.ports[metrics->port_arr.size - 1].name, PORT_MAX_LEN, WEB_LOG_INVALID_PORT_STR);
            else
                snprintfz(metrics->port_arr.ports[metrics->port_arr.size - 1].name, PORT_MAX_LEN, "%d", line_parsed->port);
            metrics->port_arr.ports[metrics->port_arr.size - 1].port = line_parsed->port;
            metrics->port_arr.ports[metrics->port_arr.size - 1].count = 1;
        } 
    }

    /* Extract client metrics */
    if(( parser_config->chart_config & ( CHART_IP_VERSION | CHART_REQ_CLIENT_CURRENT | CHART_REQ_CLIENT_ALL_TIME)) && *line_parsed->req_client) {
        
        /* Invalid IP version */
        if(unlikely(!strcmp(line_parsed->req_client, WEB_LOG_INVALID_CLIENT_IP_STR))){
            if(parser_config->chart_config & CHART_IP_VERSION) metrics->ip_ver.invalid++;
        }

        else if(strchr(line_parsed->req_client, ':')){
            /* IPv6 version */
            if(parser_config->chart_config & CHART_IP_VERSION) metrics->ip_ver.v6++;

            /* Unique Client IPv6 Address current poll */
            if(parser_config->chart_config & CHART_REQ_CLIENT_CURRENT){
                int i;
                for(i = 0; i < metrics->req_clients_current_arr.ipv6_size; i++){
                    if(!strcmp(metrics->req_clients_current_arr.ipv6_req_clients[i], line_parsed->req_client)) break;
                }
                if(metrics->req_clients_current_arr.ipv6_size == i){ // Req client not found in array - need to append
                    metrics->req_clients_current_arr.ipv6_size++;
                    metrics->req_clients_current_arr.ipv6_req_clients = reallocz(metrics->req_clients_current_arr.ipv6_req_clients, 
                        metrics->req_clients_current_arr.ipv6_size * sizeof(*metrics->req_clients_current_arr.ipv6_req_clients));
                    snprintf(metrics->req_clients_current_arr.ipv6_req_clients[metrics->req_clients_current_arr.ipv6_size - 1], 
                        REQ_CLIENT_MAX_LEN, "%s", line_parsed->req_client);
                }
            }

            /* Unique Client IPv6 Address all-time */
            if(parser_config->chart_config & CHART_REQ_CLIENT_ALL_TIME){
                int i;
                for(i = 0; i < metrics->req_clients_alltime_arr.ipv6_size; i++){
                    if(!strcmp(metrics->req_clients_alltime_arr.ipv6_req_clients[i], line_parsed->req_client)) break;
                }
                if(metrics->req_clients_alltime_arr.ipv6_size == i){ // Req client not found in array - need to append
                    metrics->req_clients_alltime_arr.ipv6_size++;
                    metrics->req_clients_alltime_arr.ipv6_req_clients = reallocz(metrics->req_clients_alltime_arr.ipv6_req_clients, 
                        metrics->req_clients_alltime_arr.ipv6_size * sizeof(*metrics->req_clients_alltime_arr.ipv6_req_clients));
                    snprintf(metrics->req_clients_alltime_arr.ipv6_req_clients[metrics->req_clients_alltime_arr.ipv6_size - 1], 
                        REQ_CLIENT_MAX_LEN, "%s", line_parsed->req_client);
                }
            }
        }

        
        else{
            /* IPv4 version */
            if(parser_config->chart_config & CHART_IP_VERSION) metrics->ip_ver.v4++;

            /* Unique Client IPv4 Address current poll */
            if(parser_config->chart_config & CHART_REQ_CLIENT_CURRENT){
                int i;
                for(i = 0; i < metrics->req_clients_current_arr.ipv4_size; i++){
                    if(!strcmp(metrics->req_clients_current_arr.ipv4_req_clients[i], line_parsed->req_client)) break;
                }
                if(metrics->req_clients_current_arr.ipv4_size == i){ // Req client not found in array - need to append
                    metrics->req_clients_current_arr.ipv4_size++;
                    metrics->req_clients_current_arr.ipv4_req_clients = reallocz(metrics->req_clients_current_arr.ipv4_req_clients, 
                        metrics->req_clients_current_arr.ipv4_size * sizeof(*metrics->req_clients_current_arr.ipv4_req_clients));
                    snprintf(metrics->req_clients_current_arr.ipv4_req_clients[metrics->req_clients_current_arr.ipv4_size - 1], 
                        REQ_CLIENT_MAX_LEN, "%s", line_parsed->req_client);
                }
            }

            /* Unique Client IPv4 Address all-time */
            if(parser_config->chart_config & CHART_REQ_CLIENT_ALL_TIME){
                int i;
                for(i = 0; i < metrics->req_clients_alltime_arr.ipv4_size; i++){
                    if(!strcmp(metrics->req_clients_alltime_arr.ipv4_req_clients[i], line_parsed->req_client)) break;
                }
                if(metrics->req_clients_alltime_arr.ipv4_size == i){ // Req client not found in array - need to append
                    metrics->req_clients_alltime_arr.ipv4_size++;
                    metrics->req_clients_alltime_arr.ipv4_req_clients = reallocz(metrics->req_clients_alltime_arr.ipv4_req_clients, 
                        metrics->req_clients_alltime_arr.ipv4_size * sizeof(*metrics->req_clients_alltime_arr.ipv4_req_clients));
                    snprintf(metrics->req_clients_alltime_arr.ipv4_req_clients[metrics->req_clients_alltime_arr.ipv4_size - 1], 
                        REQ_CLIENT_MAX_LEN, "%s", line_parsed->req_client);
                }
            }
        }
    }

    /* Extract request method */
    if(parser_config->chart_config & CHART_REQ_METHODS){
        for(int i = 0; i < REQ_METHOD_ARR_SIZE; i++){
            if(!strcmp(line_parsed->req_method, req_method_str[i])){
                metrics->req_method[i]++;
                break;
            }
        }
    }

    /* Extract request protocol */
    if(parser_config->chart_config & CHART_REQ_PROTO){
        if(!strcmp(line_parsed->req_proto, "1") || !strcmp(line_parsed->req_proto, "1.0")) metrics->req_proto.http_1++;
        else if(!strcmp(line_parsed->req_proto, "1.1")) metrics->req_proto.http_1_1++;
        else if(!strcmp(line_parsed->req_proto, "2") || !strcmp(line_parsed->req_proto, "2.0")) metrics->req_proto.http_2++;
        else metrics->req_proto.other++;
    }

    /* Extract bytes received and sent */
    if(parser_config->chart_config & CHART_BANDWIDTH){
        metrics->bandwidth.req_size += line_parsed->req_size;
        metrics->bandwidth.resp_size += line_parsed->resp_size;
    }

    /* Extract request processing time */
    if((parser_config->chart_config & CHART_REQ_PROC_TIME) && line_parsed->req_proc_time){
        if(line_parsed->req_proc_time < metrics->req_proc_time.min || metrics->req_proc_time.min == 0){
            metrics->req_proc_time.min = line_parsed->req_proc_time;
        }
        if(line_parsed->req_proc_time > metrics->req_proc_time.max || metrics->req_proc_time.max == 0){
            metrics->req_proc_time.max = line_parsed->req_proc_time;
        }
        metrics->req_proc_time.sum += line_parsed->req_proc_time;
        metrics->req_proc_time.count++;
    }

    /* Extract response code family, response code & response code type */
    if(parser_config->chart_config & (CHART_RESP_CODE_FAMILY | CHART_RESP_CODE | CHART_RESP_CODE_TYPE)){
        switch(line_parsed->resp_code / 100){
            /* Note: 304 and 401 should be treated as resp_success */
            case 1:
                metrics->resp_code_family.resp_1xx++;
                metrics->resp_code[line_parsed->resp_code - 100]++;
                metrics->resp_code_type.resp_success++;
                break;
            case 2:
                metrics->resp_code_family.resp_2xx++;
                metrics->resp_code[line_parsed->resp_code - 100]++;
                metrics->resp_code_type.resp_success++;
                break;
            case 3:
                metrics->resp_code_family.resp_3xx++;
                metrics->resp_code[line_parsed->resp_code - 100]++;
                if(line_parsed->resp_code == 304) metrics->resp_code_type.resp_success++;
                else metrics->resp_code_type.resp_redirect++;
                break;
            case 4:
                metrics->resp_code_family.resp_4xx++;
                metrics->resp_code[line_parsed->resp_code - 100]++;
                if(line_parsed->resp_code == 401) metrics->resp_code_type.resp_success++;
                else metrics->resp_code_type.resp_bad++;
                break;
            case 5:
                metrics->resp_code_family.resp_5xx++;
                metrics->resp_code[line_parsed->resp_code - 100]++;
                metrics->resp_code_type.resp_error++;
                break;
            default:
                metrics->resp_code_family.other++;
                metrics->resp_code[RESP_CODE_ARR_SIZE - 1]++;
                metrics->resp_code_type.other++;
                break;
        }
    }

    /* Extract SSL protocol */
    if(parser_config->chart_config & CHART_SSL_PROTO){
        if(!strcmp(line_parsed->ssl_proto, "TLSv1")) metrics->ssl_proto.tlsv1++;
        else if(!strcmp(line_parsed->ssl_proto, "TLSv1.1")) metrics->ssl_proto.tlsv1_1++;
        else if(!strcmp(line_parsed->ssl_proto, "TLSv1.2")) metrics->ssl_proto.tlsv1_2++;
        else if(!strcmp(line_parsed->ssl_proto, "TLSv1.3")) metrics->ssl_proto.tlsv1_3++;
        else if(!strcmp(line_parsed->ssl_proto, "SSLv2")) metrics->ssl_proto.sslv2++;
        else if(!strcmp(line_parsed->ssl_proto, "SSLv3")) metrics->ssl_proto.sslv3++;
        else metrics->ssl_proto.other++;
    }

    /* Extract SSL cipher suite */
    // TODO: Reduce number of reallocs
    if((parser_config->chart_config & CHART_SSL_CIPHER) && *line_parsed->ssl_cipher){
        int i;
        for(i = 0; i < metrics->ssl_cipher_arr.size; i++){
            if(!strcmp(metrics->ssl_cipher_arr.ssl_ciphers[i].name, line_parsed->ssl_cipher)){
                metrics->ssl_cipher_arr.ssl_ciphers[i].count++;
                break;
            }
        }
        if(metrics->ssl_cipher_arr.size == i){ // SSL cipher suite not found in array - need to append
            metrics->ssl_cipher_arr.size++;
            metrics->ssl_cipher_arr.ssl_ciphers = reallocz(metrics->ssl_cipher_arr.ssl_ciphers, 
                                        metrics->ssl_cipher_arr.size * sizeof(struct log_parser_metrics_ssl_cipher));
            snprintf( metrics->ssl_cipher_arr.ssl_ciphers[metrics->ssl_cipher_arr.size - 1].name, 
                      SSL_CIPHER_SUITE_MAX_LEN, "%s", line_parsed->ssl_cipher);
            metrics->ssl_cipher_arr.ssl_ciphers[metrics->ssl_cipher_arr.size - 1].count = 1;
        }
    }

    metrics->timestamp = line_parsed->timestamp;
}

/**
 * @brief Try to automatically detect the configuration for a web log parser.
 * @details It tries to automatically detect the configuration to be used for
 * a web log parser, by parsing a single web log line record and trying to pick 
 * a matching configuration (from a static list of predefined ones.)
 * @param[in] line Null-terminated web log line to use in guessing the configuration.
 * @param[in] delimiter Delimiter used to break down \p line in separate fields.
 * @returns Pointer to the web log parser configuration if automatic detection
 * was sucessful, otherwise NULL.
 */
Web_log_parser_config_t *auto_detect_web_log_parser_config(char *line, const char delimiter){
    for(int i = 0; csv_auto_format_guess_matrix[i] != NULL; i++){
        Web_log_parser_config_t *wblp_config = read_web_log_parser_config(csv_auto_format_guess_matrix[i], delimiter);
        if(count_fields(line, delimiter) == wblp_config->num_fields){
            wblp_config->verify_parsed_logs = 1; // Verification must be turned on to be able to pick up parsing_errors
            Log_line_parsed_t line_parsed = (Log_line_parsed_t) {0};
            parse_web_log_line(wblp_config, line, strlen(line), &line_parsed);
            if(line_parsed.parsing_errors == 0){
                return wblp_config;
            }
        }
        
        freez(wblp_config->fields);
        freez(wblp_config);
    }
    return NULL;
}
