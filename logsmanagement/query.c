/** @file query.c
 *  @brief This is the file containing the implementation of the querying API.
 *
 *  @author Dimitris Pantazis
 */

#include "query.h"
#include <uv.h>
#include "circular_buffer.h"
#include "db_api.h"
#include "file_info.h"
#include "helper.h"

#define MEASURE_QUERY_TIME 0

static const char esc_ch[] = "[]\\^$.|?*+(){}";

/**
 * @brief Sanitise string to work with regular expressions
 * @param[in] s Input string to be sanitised - will not be modified
 * @return Sanitised string (escaped characters according to esc_ch[] array)
 */
UNIT_STATIC char *sanitise_string(char *const s){
    size_t s_len = strlen(s);
    /* Truncate keyword if longer than maximum allowed length */
    if(unlikely(s_len > MAX_KEYWORD_LEN)){
        s_len = MAX_KEYWORD_LEN;
        s[s_len] = '\0';
    }
    char *s_san = mallocz(s_len * 2);

    char *s_off = s;
    char *s_san_off = s_san; 
    while(*s_off) {
        for(char *esc_ch_off = (char *) esc_ch; *esc_ch_off; esc_ch_off++){
            if(*s_off == *esc_ch_off){
                *s_san_off++ = '\\';
                break;
            }
        }
        *s_san_off++ = *s_off++;
    }
    *s_san_off = '\0';
    return s_san;
}

LOGS_QUERY_RESULT_TYPE fetch_log_sources(BUFFER *wb){
    if(unlikely(!p_file_infos_arr || !p_file_infos_arr->count)) return NO_RESULTS_FOUND;

    for (int i = 0; i < p_file_infos_arr->count; i++) {
        buffer_sprintf( wb, "       \"%s\": {\n"
                            "         \"basename\": \"%s\",\n"
                            "         \"filename\": \"%s\",\n"
                            "         \"log type\": \"%s\",\n"
                            "         \"DB dir\": \"%s\",\n"
                            "         \"DB version\": %d,\n"
                            "         \"DB flush interval\": %d,\n"
                            "         \"DB disk space limit\": %" PRId64 "\n"
                            "      },\n", 
                        p_file_infos_arr->data[i]->chart_name,
                        p_file_infos_arr->data[i]->file_basename,
                        p_file_infos_arr->data[i]->filename,
                        log_src_type_t_str[p_file_infos_arr->data[i]->log_type],
                        p_file_infos_arr->data[i]->db_dir,
                        db_user_version(p_file_infos_arr->data[i]->db, -1),
                        p_file_infos_arr->data[i]->buff_flush_to_db_interval,
                        p_file_infos_arr->data[i]->blob_max_size * BLOB_MAX_FILES
                        );
    }
    /* Results are terminated as ",\n" but should actually be null-terminated, 
     * so replace those 2 characters with '\0' */
    wb->len -= 2;
    wb->buffer[wb->len] = '\0';
    return OK;
}

LOGS_QUERY_RESULT_TYPE execute_logs_manag_query(logs_query_params_t *p_query_params) {
    struct File_info *p_file_infos[LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES] = {NULL};

    if(unlikely(p_query_params->quota > MAX_LOG_MSG_SIZE)) p_query_params->quota = MAX_LOG_MSG_SIZE;

    /* Check all required query parameters are present */
    if(unlikely( !p_query_params->start_timestamp || 
                 !p_query_params->end_timestamp || 
                (!*p_query_params->filename && !*p_query_params->chart_name))){
        return INVALID_REQUEST_ERROR;
    }

    /* Find p_file_infos for this query according to chart_names or filenames if the former is not valid. Only one of 
     * the two will be used, not both charts_names and filenames can be mixed. */
    if(p_query_params->chart_name[0]){
        int pfi_off = 0;
        for(int cn_off = 0; p_query_params->chart_name[cn_off]; cn_off++) {
            for(int pfia_dat_off = 0; pfia_dat_off < p_file_infos_arr->count; pfia_dat_off++) {
                if( !strcmp(p_file_infos_arr->data[pfia_dat_off]->chart_name, p_query_params->chart_name[cn_off]) && 
                    p_file_infos_arr->data[pfia_dat_off]->db_mode != LOGS_MANAG_DB_MODE_NONE) {
                    p_file_infos[pfi_off++] = p_file_infos_arr->data[pfia_dat_off];
                    break;
                }
            }
        }
    } 
    else if(p_query_params->filename[0]){
        int pfi_off = 0;
        for(int fn_off = 0; p_query_params->filename[fn_off]; fn_off++) {
            for(int pfia_dat_off = 0; pfia_dat_off < p_file_infos_arr->count; pfia_dat_off++) {
                if( !strcmp(p_file_infos_arr->data[pfia_dat_off]->filename, p_query_params->filename[fn_off]) && 
                    p_file_infos_arr->data[pfia_dat_off]->db_mode != LOGS_MANAG_DB_MODE_NONE) {
                    p_file_infos[pfi_off++] = p_file_infos_arr->data[pfia_dat_off];
                    break;
                }
            }
        }
    }
    else return NO_MATCHING_CHART_OR_FILENAME_ERROR;

    if(unlikely(!p_file_infos[0])) return NO_MATCHING_CHART_OR_FILENAME_ERROR;

    
    if( p_query_params->sanitize_keyword && p_query_params->keyword && 
        *p_query_params->keyword && strcmp(p_query_params->keyword, " ")){
        p_query_params->keyword = sanitise_string(p_query_params->keyword); // freez(p_query_params->keyword) in this case
    }
    

    /* Secure DB lock to ensure no data will be transferred from the buffers to 
     * the DB during the query execution and also no other execute_logs_manag_query 
     * will try to access the DB at the same time. The operations happen 
     * atomically and the DB searches in series. */
    for(int pfi_off = 0; p_file_infos[pfi_off]; pfi_off++){
        uv_mutex_lock(p_file_infos[pfi_off]->db_mut);
    }

#if MEASURE_QUERY_TIME
    const msec_t start_time = now_realtime_msec();
#endif // MEASURE_QUERY_TIME

    /* Search DB(s) first */
    db_search_compound(p_query_params, p_file_infos);

#if MEASURE_QUERY_TIME
    const msec_t db_search_time = now_realtime_msec();
#endif // MEASURE_QUERY_TIME

    /* Search circular buffer ONLY IF the results len is less than the originally requested max size!
     * p_query_params->end_timestamp will be the originally requested here, as 
     * it won't have been updated in db_search() due to the
     * (p_query_params->results_buff->len >=  p_query_params->quota) condition */
    if (p_query_params->results_buff->len <  p_query_params->quota) {
        Circ_buff_t *circ_buffs[LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES] = {NULL};
        int pfi_off = -1;
        while(p_file_infos[++pfi_off]){
            circ_buffs[pfi_off] = p_file_infos[pfi_off]->circ_buff;
            uv_rwlock_rdlock(&circ_buffs[pfi_off]->buff_realloc_rwlock);
        }
        circ_buff_search(circ_buffs, p_query_params);
        for(pfi_off = 0; p_file_infos[pfi_off]; pfi_off++){
            uv_rwlock_rdunlock(&circ_buffs[pfi_off]->buff_realloc_rwlock);
        }
    }

    for(int pfi_off = 0; p_file_infos[pfi_off]; pfi_off++){
        uv_mutex_unlock(p_file_infos[pfi_off]->db_mut);
    }

#if MEASURE_QUERY_TIME
    const msec_t end_time = now_realtime_msec();
    debug(D_LOGS_MANAG, "It took %" PRId64 "ms to execute query (%" PRId64 "ms DB search, %" 
          PRId64 "ms circ buffer search), retrieving %zuKB.\n",
          (int64_t)end_time - start_time, (int64_t)db_search_time - start_time, 
          (int64_t)end_time - db_search_time,
          p_query_params->results_buff->len / 1000);
#endif // MEASURE_QUERY_TIME

    /* If keyword has been sanitised, it needs to be freed - otherwise it's just a pointer to a substring */
    if(p_query_params->sanitize_keyword && p_query_params->keyword){
        freez(p_query_params->keyword);
    }

    if(!p_query_params->results_buff->len) return NO_RESULTS_FOUND;

    return OK;
}
