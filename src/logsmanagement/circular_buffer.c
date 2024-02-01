// SPDX-License-Identifier: GPL-3.0-or-later

/** @file circular_buffer.c
 *  @brief This is the implementation of a circular buffer to be used 
 *         for saving collected logs in memory, until they are stored 
 *         into the database.
 */

#include "circular_buffer.h"
#include "helper.h"
#include "parser.h"

struct qsort_item {
    Circ_buff_item_t *cbi;
    struct File_info *pfi;
};

static int qsort_timestamp (const void *item_a, const void *item_b) {
   return ( (int64_t)((struct qsort_item*)item_a)->cbi->timestamp - 
            (int64_t)((struct qsort_item*)item_b)->cbi->timestamp);
}

static int reverse_qsort_timestamp (const void * item_a, const void * item_b) {
   return -qsort_timestamp(item_a, item_b);
}

/**
 * @brief Search circular buffers according to the query_params.
 * @details If multiple buffers are to be searched, the results will be sorted
 * according to timestamps.
 * 
 * Note that buff->tail can only be changed through circ_buff_read_done(), and 
 * circ_buff_search() and circ_buff_read_done() are mutually exclusive due 
 * to uv_mutex_lock() and uv_mutex_unlock() in queries and when writing to DB.
 * 
 * @param p_query_params Query parameters to search according to.
 * @param p_file_infos   File_info structs to be searched.
 */
void circ_buff_search(logs_query_params_t *const p_query_params, struct File_info *const p_file_infos[]) {
    
    for(int pfi_off = 0; p_file_infos[pfi_off]; pfi_off++)
        uv_rwlock_rdlock(&p_file_infos[pfi_off]->circ_buff->buff_realloc_rwlock);

    int buffs_size = 0, 
        buff_max_num_of_items = 0;

    while(p_file_infos[buffs_size]){
        if(p_file_infos[buffs_size]->circ_buff->num_of_items > buff_max_num_of_items) 
            buff_max_num_of_items = p_file_infos[buffs_size]->circ_buff->num_of_items;
        buffs_size++;
    }

    struct qsort_item items[buffs_size * buff_max_num_of_items + 1]; // worst case allocation
    
    int items_off = 0;

    for(int buff_off = 0; p_file_infos[buff_off]; buff_off++){
        Circ_buff_t *buff = p_file_infos[buff_off]->circ_buff;
        /* TODO: The following 3 operations need to be replaced with a struct
         * to gurantee atomicity. */
        int head = __atomic_load_n(&buff->head, __ATOMIC_SEQ_CST) % buff->num_of_items;
        int tail = __atomic_load_n(&buff->tail, __ATOMIC_SEQ_CST) % buff->num_of_items;
        int full = __atomic_load_n(&buff->full, __ATOMIC_SEQ_CST);

        if ((head == tail) && !full) continue;  // Nothing to do if buff is empty

        for (int i = tail; i != head; i = (i + 1) % buff->num_of_items){
            items[items_off].cbi = &buff->items[i];
            items[items_off++].pfi = p_file_infos[buff_off];
        }
    }

    items[items_off].cbi = NULL;
    items[items_off].pfi = NULL;

    if(items[0].cbi) 
        qsort(items, items_off, sizeof(items[0]), p_query_params->order_by_asc ? qsort_timestamp : reverse_qsort_timestamp);

    
    BUFFER *const res_buff = p_query_params->results_buff;
 
    logs_query_res_hdr_t res_hdr = { // result header
        .timestamp = p_query_params->act_to_ts,
        .text_size = 0,
        .matches = 0,
        .log_source = "",
        .log_type = ""
    }; 

    for (int i = 0; items[i].cbi; i++) {

        /* If exceeding quota or timeout is reached and new timestamp is different than previous, 
         * terminate query but inform caller about act_to_ts to continue from (its next value) in next call. */
        if( (res_buff->len >= p_query_params->quota || terminate_logs_manag_query(p_query_params)) && 
                items[i].cbi->timestamp != res_hdr.timestamp){
            p_query_params->act_to_ts = res_hdr.timestamp;
            break;
        }

        res_hdr.timestamp = items[i].cbi->timestamp;
        res_hdr.text_size = items[i].cbi->text_size;
        strncpyz(res_hdr.log_source, log_src_t_str[items[i].pfi->log_source], sizeof(res_hdr.log_source) - 1);
        strncpyz(res_hdr.log_type, log_src_type_t_str[items[i].pfi->log_type], sizeof(res_hdr.log_type) - 1);
        strncpyz(res_hdr.basename, items[i].pfi->file_basename, sizeof(res_hdr.basename) - 1);
        strncpyz(res_hdr.filename, items[i].pfi->filename, sizeof(res_hdr.filename) - 1);
        strncpyz(res_hdr.chartname, items[i].pfi->chartname, sizeof(res_hdr.chartname) - 1);

        if (p_query_params->order_by_asc ?
            ( res_hdr.timestamp >= p_query_params->req_from_ts  && res_hdr.timestamp <= p_query_params->req_to_ts  ) : 
            ( res_hdr.timestamp >= p_query_params->req_to_ts    && res_hdr.timestamp <= p_query_params->req_from_ts) ){

            /* In case of search_keyword, less than sizeof(res_hdr) + temp_msg.text_size 
             * space is required, but go for worst case scenario for now */
            buffer_increase(res_buff, sizeof(res_hdr) + res_hdr.text_size); 

            if(!p_query_params->keyword || !*p_query_params->keyword || !strcmp(p_query_params->keyword, " ")){
                /* NOTE: relying on items[i]->cbi->num_lines to get number of log lines
                 * might not be 100% correct, since parsing must have taken place 
                 * already to return correct count. Maybe an issue under heavy load. */
                res_hdr.matches = items[i].cbi->num_lines;
                memcpy(&res_buff->buffer[res_buff->len + sizeof(res_hdr)], items[i].cbi->data, res_hdr.text_size);
            }
            else {
                res_hdr.matches = search_keyword(   items[i].cbi->data, res_hdr.text_size, 
                                                    &res_buff->buffer[res_buff->len + sizeof(res_hdr)], 
                                                    &res_hdr.text_size, p_query_params->keyword, NULL, 
                                                    p_query_params->ignore_case);

                m_assert(   (res_hdr.matches > 0 && res_hdr.text_size > 0) || 
                            (res_hdr.matches == 0 && res_hdr.text_size == 0), 
                            "res_hdr.matches and res_hdr.text_size must both be > 0 or == 0.");

                if(unlikely(res_hdr.matches < 0)) 
                    break; /* res_hdr.matches < 0 - error during keyword search */        
            }

            if(res_hdr.text_size){
                res_buff->buffer[res_buff->len + sizeof(res_hdr) + res_hdr.text_size - 1] = '\n'; // replace '\0' with '\n' 
                memcpy(&res_buff->buffer[res_buff->len], &res_hdr, sizeof(res_hdr));
                res_buff->len += sizeof(res_hdr) + res_hdr.text_size; 
                p_query_params->num_lines += res_hdr.matches;
            }

            m_assert(TEST_MS_TIMESTAMP_VALID(res_hdr.timestamp), "res_hdr.timestamp is invalid");
        }
    }

    for(int pfi_off = 0; p_file_infos[pfi_off]; pfi_off++)
        uv_rwlock_rdunlock(&p_file_infos[pfi_off]->circ_buff->buff_realloc_rwlock);
}

/**
 * @brief Query circular buffer if there is space for item insertion.
 * @param buff Circular buffer to query for available space.
 * @param requested_text_space Size of raw (uncompressed) space needed.
 * @note If buff->allow_dropped_logs is 0, then this function will block and
 * it will only return once there is available space as requested. In this 
 * case, it will never return 0.
 * @return \p requested_text_space if there is enough space, else 0.
 */
size_t circ_buff_prepare_write(Circ_buff_t *const buff, size_t const requested_text_space){

    /* Calculate how much is the maximum compressed space that will 
     * be required on top of the requested space for the raw data. */
    buff->in->text_compressed_size = (size_t) LZ4_compressBound(requested_text_space);
    m_assert(buff->in->text_compressed_size != 0, "requested text compressed space is zero");
    size_t const required_space = requested_text_space + buff->in->text_compressed_size;

    size_t available_text_space = 0;
    size_t total_cached_mem_ex_in;
    
try_to_acquire_space:
    total_cached_mem_ex_in = 0;
    for (int i = 0; i < buff->num_of_items; i++){
        total_cached_mem_ex_in += buff->items[i].data_max_size;
    }

    /* If the required space is more than the allocated space of the input
    * buffer, then we need to check if the input buffer can be reallocated:
    * 
    * a) If the total memory consumption of the circular buffer plus the 
    * required space is less than the limit set by "circular buffer max size" 
    * for this log source, then the input buffer can be reallocated.
    * 
    * b) If the total memory consumption of the circular buffer plus the 
    * required space is more than the limit set by "circular buffer max size" 
    * for this log source, we will attempt to reclaim some of the circular 
    * buffer allocated memory from any empty items. 
    * 
    * c) If after reclaiming the total memory consumption is still beyond the 
    * configuration limit, either 0 will be returned as the available space 
    * for raw logs in the input buffer, or the function will block and repeat
    * the same process, until there is available space to be returned, depending
    * of the configuration value of buff->allow_dropped_logs.
    * */
    if(required_space > buff->in->data_max_size) {
        if(likely(total_cached_mem_ex_in + required_space <= buff->total_cached_mem_max)){
            buff->in->data_max_size = required_space;
            buff->in->data = reallocz(buff->in->data, buff->in->data_max_size);

            available_text_space = requested_text_space;
        }
        else if(likely(__atomic_load_n(&buff->full, __ATOMIC_SEQ_CST) == 0)){
            int head = __atomic_load_n(&buff->head, __ATOMIC_SEQ_CST) % buff->num_of_items;
            int tail = __atomic_load_n(&buff->tail, __ATOMIC_SEQ_CST) % buff->num_of_items;

            for (int i = (head == tail ? (head + 1) % buff->num_of_items : head); 
                i != tail; i = (i + 1) % buff->num_of_items) {
                
                m_assert(i <= buff->num_of_items, "i > buff->num_of_items");
                buff->items[i].data_max_size = 1;
                buff->items[i].data = reallocz(buff->items[i].data, buff->items[i].data_max_size);
            }

            total_cached_mem_ex_in = 0;
            for (int i = 0; i < buff->num_of_items; i++){
                total_cached_mem_ex_in += buff->items[i].data_max_size;
            }

            if(total_cached_mem_ex_in + required_space <= buff->total_cached_mem_max){
                buff->in->data_max_size = required_space;
                buff->in->data = reallocz(buff->in->data, buff->in->data_max_size);

                available_text_space = requested_text_space;
            }
            else available_text_space = 0;
        }
    } else available_text_space = requested_text_space;

    __atomic_store_n(&buff->total_cached_mem, total_cached_mem_ex_in + buff->in->data_max_size, __ATOMIC_RELAXED);

    if(unlikely(!buff->allow_dropped_logs && !available_text_space)){
        sleep_usec(CIRC_BUFF_PREP_WR_RETRY_AFTER_MS * USEC_PER_MS);
        goto try_to_acquire_space;
    }
        
    m_assert(available_text_space || buff->allow_dropped_logs, "!available_text_space == 0 && !buff->allow_dropped_logs");
    return available_text_space;
}

/**
 * @brief Insert item from temporary input buffer to circular buffer.
 * @param buff Circular buffer to insert the item into
 * @return 0 in case of success or -1 in case there was an error (e.g. buff 
 * is out of space).
 */
int circ_buff_insert(Circ_buff_t *const buff){

    // TODO: Probably can be changed to __ATOMIC_RELAXED, but ideally a mutex should be used here.
    int head = __atomic_load_n(&buff->head, __ATOMIC_SEQ_CST) % buff->num_of_items;
    int tail = __atomic_load_n(&buff->tail, __ATOMIC_SEQ_CST) % buff->num_of_items;
    int full = __atomic_load_n(&buff->full, __ATOMIC_SEQ_CST);
   
    /* If circular buffer does not have any free items, it will be expanded
     * by reallocating the `items` array and adding one more item. */
    if (unlikely(( head == tail ) && full )) {
        debug_log( "buff out of space! will be expanded.");
        uv_rwlock_wrlock(&buff->buff_realloc_rwlock);

        
        Circ_buff_item_t *items_new = callocz(buff->num_of_items + 1, sizeof(Circ_buff_item_t));

        for(int i = 0; i < buff->num_of_items; i++){
            Circ_buff_item_t *item_old = &buff->items[head++ % buff->num_of_items];
            items_new[i] = *item_old;
        }
        freez(buff->items);
        buff->items = items_new;

        buff->parse = buff->parse - buff->tail;
        head = buff->head = buff->num_of_items++;
        buff->tail = buff->read = 0;
        buff->full = 0; 

        __atomic_add_fetch(&buff->buff_realloc_cnt, 1, __ATOMIC_RELAXED);

        uv_rwlock_wrunlock(&buff->buff_realloc_rwlock);
    }

    Circ_buff_item_t *cur_item = &buff->items[head];

    char *tmp_data = cur_item->data;
    size_t tmp_data_max_size = cur_item->data_max_size;

    cur_item->status = buff->in->status;
    cur_item->timestamp = buff->in->timestamp;
    cur_item->data = buff->in->data;
    cur_item->text_size = buff->in->text_size;
    cur_item->text_compressed = buff->in->text_compressed;
    cur_item->text_compressed_size = buff->in->text_compressed_size;
    cur_item->data_max_size = buff->in->data_max_size;
    cur_item->num_lines = buff->in->num_lines;
    
    buff->in->status = CIRC_BUFF_ITEM_STATUS_UNPROCESSED;
    buff->in->timestamp = 0;
    buff->in->data = tmp_data;
    buff->in->text_size = 0;
    // buff->in->text_compressed = tmp_data;
    buff->in->text_compressed_size = 0;
    buff->in->data_max_size = tmp_data_max_size;
    buff->in->num_lines = 0;

    __atomic_add_fetch(&buff->text_size_total, cur_item->text_size, __ATOMIC_SEQ_CST);
    
    if( __atomic_add_fetch(&buff->text_compressed_size_total, cur_item->text_compressed_size, __ATOMIC_SEQ_CST)){
            __atomic_store_n(&buff->compression_ratio, 
            __atomic_load_n(&buff->text_size_total, __ATOMIC_SEQ_CST) / 
            __atomic_load_n(&buff->text_compressed_size_total, __ATOMIC_SEQ_CST), 
            __ATOMIC_SEQ_CST);
    } else __atomic_store_n( &buff->compression_ratio, 0, __ATOMIC_SEQ_CST);


    if(unlikely(__atomic_add_fetch(&buff->head, 1, __ATOMIC_SEQ_CST) % buff->num_of_items == 
                __atomic_load_n(&buff->tail, __ATOMIC_SEQ_CST) % buff->num_of_items)){ 
        __atomic_store_n(&buff->full, 1, __ATOMIC_SEQ_CST);
    }

    __atomic_or_fetch(&cur_item->status, CIRC_BUFF_ITEM_STATUS_PARSED | CIRC_BUFF_ITEM_STATUS_STREAMED, __ATOMIC_SEQ_CST);

    return 0;
}

/**
 * @brief Return pointer to next item to be read from the circular buffer.
 * @param buff Circular buffer to get next item from.
 * @return Pointer to the next circular buffer item to be read, or NULL
 * if there are no more items to be read.
 */
Circ_buff_item_t *circ_buff_read_item(Circ_buff_t *const buff) {

    Circ_buff_item_t *item = &buff->items[buff->read % buff->num_of_items];

    m_assert(__atomic_load_n(&item->status, __ATOMIC_RELAXED) <= CIRC_BUFF_ITEM_STATUS_DONE, "Invalid status");

    if( /* No more records to be retrieved from the buffer - pay attention that 
         * there is no `% buff->num_of_items` operation, as we need to check 
         * the case where buff->read is exactly equal to buff->head. */ 
        (buff->read == (__atomic_load_n(&buff->head, __ATOMIC_SEQ_CST))) ||
        /* Current item either not parsed or streamed */
        (__atomic_load_n(&item->status, __ATOMIC_RELAXED) != CIRC_BUFF_ITEM_STATUS_DONE) ){
        
        return NULL;
    }

    __atomic_sub_fetch(&buff->text_size_total, item->text_size, __ATOMIC_SEQ_CST);

    if( __atomic_sub_fetch(&buff->text_compressed_size_total, item->text_compressed_size, __ATOMIC_SEQ_CST)){
            __atomic_store_n(&buff->compression_ratio, 
            __atomic_load_n(&buff->text_size_total, __ATOMIC_SEQ_CST) / 
            __atomic_load_n(&buff->text_compressed_size_total, __ATOMIC_SEQ_CST), 
            __ATOMIC_SEQ_CST);
    } else __atomic_store_n( &buff->compression_ratio, 0, __ATOMIC_SEQ_CST);

    buff->read++;

    return item;
}

/**
 * @brief Complete buffer read process.
 * @param buff Circular buffer to complete read process on.
 */
void circ_buff_read_done(Circ_buff_t *const buff){
    /* Even if one item was read, it means buffer cannot be full anymore */
    if(__atomic_load_n(&buff->tail, __ATOMIC_RELAXED) != buff->read) 
        __atomic_store_n(&buff->full, 0, __ATOMIC_SEQ_CST);

    __atomic_store_n(&buff->tail, buff->read, __ATOMIC_SEQ_CST);
}

/**
 * @brief Create a new circular buffer.
 * @param num_of_items Number of Circ_buff_item_t items in the buffer.
 * @param max_size Maximum memory the circular buffer can occupy.
 * @param allow_dropped_logs Maximum memory the circular buffer can occupy.
 * @return Pointer to the new circular buffer structure.
 */
Circ_buff_t *circ_buff_init(const int num_of_items, 
                            const size_t max_size,
                            const int allow_dropped_logs ) {
    Circ_buff_t *buff = callocz(1, sizeof(Circ_buff_t));
    buff->num_of_items = num_of_items;
    buff->items = callocz(buff->num_of_items, sizeof(Circ_buff_item_t));
    buff->in = callocz(1, sizeof(Circ_buff_item_t));

    uv_rwlock_init(&buff->buff_realloc_rwlock);

    buff->total_cached_mem_max = max_size;
    buff->allow_dropped_logs = allow_dropped_logs;

    return buff;
}

/**
 * @brief Destroy a circular buffer.
 * @param buff Circular buffer to be destroyed.
 */
void circ_buff_destroy(Circ_buff_t *buff){
    for (int i = 0; i < buff->num_of_items; i++) freez(buff->items[i].data);
    freez(buff->items);
    freez(buff->in->data);
    freez(buff->in);
    freez(buff);
}
