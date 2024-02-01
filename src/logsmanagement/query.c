// SPDX-License-Identifier: GPL-3.0-or-later

/** @file query.c
 *  
 *  @brief This is the file containing the implementation of the 
 *  logs management querying API.
 */

#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif

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
        return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_SERVER_ERR];   
    
    buffer_json_add_array_item_object(wb);
    buffer_json_member_add_string(wb, "id", "all");
    buffer_json_member_add_string(wb, "name", "all");
    buffer_json_member_add_string(wb, "pill", "100"); // TODO

    buffer_json_member_add_string(wb, "info", "All log sources");

    buffer_json_member_add_string(wb, "basename", "");
    buffer_json_member_add_string(wb, "filename", "");
    buffer_json_member_add_string(wb, "log_type", "");
    buffer_json_member_add_string(wb, "db_dir", "");
    buffer_json_member_add_uint64(wb, "db_version", 0);
    buffer_json_member_add_uint64(wb, "db_flush_freq", 0);
    buffer_json_member_add_int64( wb, "db_disk_space_limit", 0);
    buffer_json_object_close(wb); // options object

    bool queryable_sources = false;
    for (int i = 0; i < p_file_infos_arr->count; i++) {
        if(p_file_infos_arr->data[i]->db_mode == LOGS_MANAG_DB_MODE_FULL)
            queryable_sources = true;
    }

    if(!queryable_sources)
        return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_NOT_FOUND_ERR];

    for (int i = 0; i < p_file_infos_arr->count; i++) {
        buffer_json_add_array_item_object(wb);
        buffer_json_member_add_string(wb, "id", p_file_infos_arr->data[i]->chartname);
        buffer_json_member_add_string(wb, "name", p_file_infos_arr->data[i]->chartname);
        buffer_json_member_add_string(wb, "pill", "100"); // TODO

        char info[1024];
        snprintfz(info, sizeof(info), "Chart '%s' from log source '%s'",
                    p_file_infos_arr->data[i]->chartname,
                    p_file_infos_arr->data[i]->file_basename);

        buffer_json_member_add_string(wb, "info", info);

        buffer_json_member_add_string(wb, "basename", p_file_infos_arr->data[i]->file_basename);
        buffer_json_member_add_string(wb, "filename", p_file_infos_arr->data[i]->filename);
        buffer_json_member_add_string(wb, "log_type", log_src_type_t_str[p_file_infos_arr->data[i]->log_type]);
        buffer_json_member_add_string(wb, "db_dir", p_file_infos_arr->data[i]->db_dir);
        buffer_json_member_add_int64(wb, "db_version", db_user_version(p_file_infos_arr->data[i]->db, -1));
        buffer_json_member_add_int64(wb, "db_flush_freq", db_user_version(p_file_infos_arr->data[i]->db, -1));
        buffer_json_member_add_int64( wb, "db_disk_space_limit", p_file_infos_arr->data[i]->blob_max_size * BLOB_MAX_FILES);
        buffer_json_object_close(wb); // options object
    }

    return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_OK];
}

bool terminate_logs_manag_query(logs_query_params_t *const p_query_params){
    if(p_query_params->cancelled && __atomic_load_n(p_query_params->cancelled, __ATOMIC_RELAXED)) {
        return true;
    }

    if(now_monotonic_usec() > __atomic_load_n(p_query_params->stop_monotonic_ut, __ATOMIC_RELAXED))
        return true;

    return false;
}

const logs_qry_res_err_t *execute_logs_manag_query(logs_query_params_t *p_query_params) {
    struct File_info *p_file_infos[LOGS_MANAG_MAX_COMPOUND_QUERY_SOURCES] = {NULL};

    /* Check all required query parameters are present */
    if(unlikely(!p_query_params->req_from_ts || !p_query_params->req_to_ts))
        return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_INV_TS_ERR];

    /* Start with maximum possible actual timestamp range and reduce it 
     * accordingly when searching DB and circular buffer. */
    p_query_params->act_from_ts = p_query_params->req_from_ts;
    p_query_params->act_to_ts = p_query_params->req_to_ts;

    if(p_file_infos_arr == NULL) 
        return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_NOT_INIT_ERR];

    /* Find p_file_infos for this query according to chartnames or filenames 
     * if the former is not valid. Only one of the two will be used, 
     * charts_names and filenames cannot be mixed.
     * If neither list is provided, search all available log sources. */
    if(p_query_params->chartname[0]){
        int pfi_off = 0;
        for(int cn_off = 0; p_query_params->chartname[cn_off]; cn_off++) {
            for(int pfi_arr_off = 0; pfi_arr_off < p_file_infos_arr->count; pfi_arr_off++) {
                if( !strcmp(p_file_infos_arr->data[pfi_arr_off]->chartname, p_query_params->chartname[cn_off]) && 
                    p_file_infos_arr->data[pfi_arr_off]->db_mode != LOGS_MANAG_DB_MODE_NONE) {
                    p_file_infos[pfi_off++] = p_file_infos_arr->data[pfi_arr_off];
                    break;
                }
            }
        }
    } 
    else if(p_query_params->filename[0]){
        int pfi_off = 0;
        for(int fn_off = 0; p_query_params->filename[fn_off]; fn_off++) {
            for(int pfi_arr_off = 0; pfi_arr_off < p_file_infos_arr->count; pfi_arr_off++) {
                if( !strcmp(p_file_infos_arr->data[pfi_arr_off]->filename, p_query_params->filename[fn_off]) && 
                    p_file_infos_arr->data[pfi_arr_off]->db_mode != LOGS_MANAG_DB_MODE_NONE) {
                    p_file_infos[pfi_off++] = p_file_infos_arr->data[pfi_arr_off];
                    break;
                }
            }
        }
    }
    else{
        int pfi_off = 0;
        for(int pfi_arr_off = 0; pfi_arr_off < p_file_infos_arr->count; pfi_arr_off++) {
            if(p_file_infos_arr->data[pfi_arr_off]->db_mode != LOGS_MANAG_DB_MODE_NONE)
                p_file_infos[pfi_off++] = p_file_infos_arr->data[pfi_arr_off];
        }
    }

    if(unlikely(!p_file_infos[0])) 
        return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_NOT_FOUND_ERR];

    
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

    if( p_query_params->results_buff->len < p_query_params->quota && 
        !terminate_logs_manag_query(p_query_params))
            circ_buff_search(p_query_params, p_file_infos);

    if(!p_query_params->order_by_asc && 
        p_query_params->results_buff->len < p_query_params->quota &&
        !terminate_logs_manag_query(p_query_params)) 
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

    if(terminate_logs_manag_query(p_query_params)){
        return  (p_query_params->cancelled && 
                __atomic_load_n(p_query_params->cancelled, __ATOMIC_RELAXED)) ?
                &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_CANCELLED]  /* cancelled */ :
                &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_TIMEOUT]    /* timed out */ ;
    }

    if(!p_query_params->results_buff->len) 
        return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_NOT_FOUND_ERR];

    return &logs_qry_res_err[LOGS_QRY_RES_ERR_CODE_OK];
}
