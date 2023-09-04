// SPDX-License-Identifier: GPL-3.0-or-later

/** @file query.c
 *  
 *  @brief This is the file containing the implementation of the 
 *  logs management querying API.
 */

#define _GNU_SOURCE

#include "query.h"
#include <uv.h>
#include <sys/resource.h>
#include "circular_buffer.h"
#include "db_api.h"
#include "file_info.h"
#include "helper.h"

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

const logs_qry_res_err_t *fetch_log_sources(BUFFER *wb){
    if(unlikely(!p_file_infos_arr || !p_file_infos_arr->count)) 
        return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_NOT_FOUND_ERR];

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

    return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_OK];
}

const logs_qry_res_err_t *execute_logs_manag_query(logs_query_params_t *p_query_params) {
    struct File_info *p_file_infos[LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES] = {NULL};

    /* Check all required query parameters are present */
    if(unlikely(!*p_query_params->filename && !*p_query_params->chart_name))
        return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_INV_REQ_ERR];

    if(unlikely(!p_query_params->req_from_ts || !p_query_params->req_to_ts))
        return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_INV_TS_ERR];

    /* Start with maximum possible actual timestamp range and reduce it 
     * accordingly when searching DB and circular buffer. */
    p_query_params->act_from_ts = p_query_params->req_from_ts;
    p_query_params->act_to_ts = p_query_params->req_to_ts;

    if(p_file_infos_arr == NULL) 
        return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_NOT_INIT_ERR];

    /* Find p_file_infos for this query according to chart_names or filenames 
     * if the former is not valid. Only one of the two will be used, 
     * charts_names and filenames cannot be mixed. */
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
    else return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_NO_MATCH_ERR];

    if(unlikely(!p_file_infos[0])) return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_NO_MATCH_ERR];

    
    if( p_query_params->sanitize_keyword && p_query_params->keyword && 
        *p_query_params->keyword && strcmp(p_query_params->keyword, " ")){
        p_query_params->keyword = sanitise_string(p_query_params->keyword); // freez(p_query_params->keyword) in this case
    }

    struct rusage ru_start, ru_end;
    getrusage(RUSAGE_THREAD, &ru_start);

    /* Secure DB lock to ensure no data will be transferred from the buffers to 
     * the DB during the query execution and also no other execute_logs_manag_query 
     * will try to access the DB at the same time. The operations happen 
     * atomically and the DB searches in series. */
    for(int pfi_off = 0; p_file_infos[pfi_off]; pfi_off++)
        uv_mutex_lock(p_file_infos[pfi_off]->db_mut);


    /* If results are requested in ascending timestamp order, search DB(s) first 
     * and then the circular buffers. Otherwise, search the circular buffers
     * first and the DB(s) second. In both cases, the quota must be respected. */
    if(p_query_params->order_by_asc) 
        db_search(p_query_params, p_file_infos);

    if (p_query_params->results_buff->len < p_query_params->quota) {
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

    if(!p_query_params->order_by_asc && p_query_params->results_buff->len <  p_query_params->quota) 
        db_search(p_query_params, p_file_infos);

    for(int pfi_off = 0; p_file_infos[pfi_off]; pfi_off++)
        uv_mutex_unlock(p_file_infos[pfi_off]->db_mut);

    getrusage(RUSAGE_THREAD, &ru_end);
    
    __atomic_add_fetch(&p_file_infos[0]->cpu_time_per_mib.user,
        p_query_params->results_buff->len ? (   ru_end.ru_utime.tv_sec    * USEC_PER_SEC - 
                                                ru_start.ru_utime.tv_sec  * USEC_PER_SEC +
                                                ru_end.ru_utime.tv_usec   - 
                                                ru_start.ru_utime.tv_usec ) * (1 MiB) / p_query_params->results_buff->len : 0
        , __ATOMIC_RELAXED);

    __atomic_add_fetch(&p_file_infos[0]->cpu_time_per_mib.sys,
        p_query_params->results_buff->len ? (   ru_end.ru_stime.tv_sec    * USEC_PER_SEC - 
                                                ru_start.ru_stime.tv_sec  * USEC_PER_SEC +
                                                ru_end.ru_stime.tv_usec   - 
                                                ru_start.ru_stime.tv_usec ) * (1 MiB) / p_query_params->results_buff->len : 0
        , __ATOMIC_RELAXED);

    /* If keyword has been sanitised, it needs to be freed - otherwise it's just a pointer to a substring */
    if(p_query_params->sanitize_keyword && p_query_params->keyword){
        freez(p_query_params->keyword);
    }

    if(!p_query_params->results_buff->len) return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_NOT_FOUND_ERR];

    return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_OK];
}
