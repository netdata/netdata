/** @file page_buffer.c
 *  @brief This is an implementation of a page buffer to be user for temporarily 
 *         storing the logs before they are inserted into the database.
 *
 *  @author Dimitris Pantazis
 */

#include "circular_buffer.h"
#include "compression.h"
#include "helper.h"
#include "parser.h"

/**
 * @brief Performs parsing and metric extraction for a raw log message
 * @todo Doesn't really belong in this source file, should be moved.
 */
void generic_parser(void *arg){

    struct File_info *p_file_info = ((struct File_info *) arg);
    Circ_buff_t *buff = p_file_info->circ_buff;
    

    while (1){
        uv_mutex_lock(&p_file_info->notify_parser_thread_mut);
        while (p_file_info->log_batches_to_be_parsed == 0) {
            uv_cond_wait(&p_file_info->notify_parser_thread_cond,
                         &p_file_info->notify_parser_thread_mut);
        }
        p_file_info->log_batches_to_be_parsed--;
        uv_mutex_unlock(&p_file_info->notify_parser_thread_mut);

        Circ_buff_item_t *item = &buff->items[buff->parse % buff->num_of_items];

        uv_mutex_lock(p_file_info->parser_metrics_mut);

        /* Perform web log parsing */
        switch(p_file_info->log_type){
            case WEB_LOG:
            case FLB_WEB_LOG: {
                if(unlikely(0 != parse_web_log_buf( item->data, item->text_size, 
                                                    p_file_info->parser_config, 
                                                    p_file_info->parser_metrics))) { 
                    debug(D_LOGS_MANAG,"Parsed buffer did not contain any text or was of 0 size.");
                    m_assert(0, "Parsed buffer did not contain any text or was of 0 size.");
                }
                break;
            }
            case GENERIC:
            case FLB_GENERIC: {
                for(int i = 0; item->data[i]; i++){
                    if(unlikely(item->data[i] == '\n')){
                        p_file_info->parser_metrics->num_lines_total++;
                        p_file_info->parser_metrics->num_lines_rate++;
                    }
                } 
                /* +1 because last line is terminated by '\0' instead of '\n' */
                p_file_info->parser_metrics->num_lines_total++;
                p_file_info->parser_metrics->num_lines_rate++;               
                break;
            }
            default: 
                break; // Silence -Wswitch warning
        }

        /* Perform custom log chart parsing */
        for(int i = 0; p_file_info->parser_cus_config[i]; i++){
            p_file_info->parser_metrics->parser_cus[i]->count += 
                search_keyword( item->data, item->text_size, NULL, NULL, 
                                NULL, &p_file_info->parser_cus_config[i]->regex, 0);
        }
        
        uv_mutex_unlock(p_file_info->parser_metrics_mut);

        if(unlikely(netdata_exit)) break;

        buff->parse++;
        __atomic_or_fetch(&item->status, CIRC_BUFF_ITEM_STATUS_PARSED | CIRC_BUFF_ITEM_STATUS_STREAMED, __ATOMIC_RELAXED);
    }
}

/**
 * @brief Search circular buffer according to the query_params.
 * @details buff->tail can only be changed through circ_buff_read_item(), and 
 * circ_buff_search() and circ_buff_read_item() are mutually exclusive due 
 * to uv_mutex_lock() and uv_mutex_unlock() in queries and when writing to DB.
 * @param buff Buffer to be searched
 * @param p_query_params Query parameters to search according to.
 * @param max_query_page_size Max query size; if exceeded, search will stop.
 */
void circ_buff_search(  Circ_buff_t *const buff, logs_query_params_t *const p_query_params) {

    int head = __atomic_load_n(&buff->head, __ATOMIC_SEQ_CST) % buff->num_of_items;
    int tail = __atomic_load_n(&buff->tail, __ATOMIC_SEQ_CST) % buff->num_of_items;
    int full = __atomic_load_n(&buff->full, __ATOMIC_SEQ_CST);

    if ((head == tail) && !full) {
        debug(D_LOGS_MANAG, "Circ buff empty! Won't be searched.");
        return;  // Nothing to do if buff is empty
    }
    
    Circ_buff_item_t item = {0};
    BUFFER *results = p_query_params->results_buff;

    for (int i = tail; i != head; i = (i + 1) % buff->num_of_items) {
        item.timestamp = buff->items[i].timestamp;
        item.data = buff->items[i].data;
        item.text_size = buff->items[i].text_size;

        if (item.timestamp >= p_query_params->start_timestamp && 
            item.timestamp <= p_query_params->end_timestamp) {

            size_t old_results_len = results->len;
            buffer_sprintf(results, "\t\t[ %" PRIu64 ", \"", item.timestamp);
            /* In case of search_keyword, less than item.text_size space is 
            * required, but go for worst case scenario for now */
            buffer_increase(results, item.text_size); 

            if(!p_query_params->keyword || !*p_query_params->keyword || !strcmp(p_query_params->keyword, " ")){
                memcpy(&results->buffer[results->len], item.data, item.text_size);
                results->len += item.text_size;
                results->len--; // get rid of '\0'
                // Watch out! Changing the next line will break web_client_api_request_v1_logsmanagement()! 
                buffer_strcat(results, "\", \t0],\n");
            }
            else {
                size_t res_size = 0;
                const int matches = search_keyword( item.data, item.text_size, 
                                                    &results->buffer[results->len], &res_size, 
                                                    p_query_params->keyword, NULL, 
                                                    p_query_params->ignore_case);
                
                if(likely(matches > 0)) {
                    m_assert(res_size > 0, "res_size can't be <= 0");
                    results->len += res_size;  
                    results->len--; // get rid of '\0'
                    // buffer_strcat(results, "\"],\n");
                    // Watch out! Changing the next line will break web_client_api_request_v1_logsmanagement()! 
                    buffer_sprintf(results, "\", \t%d],\n", matches); 
                    p_query_params->keyword_matches += matches;
                }
                else if(unlikely(matches == 0)){
                    m_assert(res_size == 0, "res_size must be == 0");
                    /* No keyword matches, undo timestamp buffer_sprintf() */
                    results->len = old_results_len;
                }
                else{ 
                    /* matches < 0 - error during keyword search */
                    break;                   
                }   
            }

            if(results->len >= p_query_params->quota){
                p_query_params->end_timestamp = item.timestamp;
                break;
            }
        }
    }
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
        
    m_assert(available_text_space || 
            (!available_text_space && buff->allow_dropped_logs), "!available_text_space == 0 && !buff->allow_dropped_logs");
    return available_text_space;
}

int circ_buff_insert(Circ_buff_t *const buff){

    // TODO: Probably can be changed to __ATOMIC_RELAXED, but ideally a mutex should be used here.
    int head = __atomic_load_n(&buff->head, __ATOMIC_SEQ_CST) % buff->num_of_items;
    int tail = __atomic_load_n(&buff->tail, __ATOMIC_SEQ_CST) % buff->num_of_items;
    int full = __atomic_load_n(&buff->full, __ATOMIC_SEQ_CST);
    
    if (unlikely(( head == tail ) && full )) {
        error("Logs circular buffer out of space! Losing data!");
        m_assert(0, "Buff full");
        // TODO: How to handle this case, when circular buffer is out of space?
        return -1;
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
    
    buff->in->status = CIRC_BUFF_ITEM_STATUS_UNPROCESSED;
    buff->in->timestamp = 0;
    buff->in->data = tmp_data;
    buff->in->text_size = 0;
    // buff->in->text_compressed = tmp_data;
    buff->in->text_compressed_size = 0;
    buff->in->data_max_size = tmp_data_max_size;

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

    return 0;
}

Circ_buff_item_t *circ_buff_read_item(Circ_buff_t *const buff) {

    Circ_buff_item_t *item = &buff->items[buff->read % buff->num_of_items];

    m_assert(__atomic_load_n(&item->status, __ATOMIC_RELAXED) <= CIRC_BUFF_ITEM_STATUS_DONE, "Invalid status");

    if( /* No more records to be retrieved from the buffer */ 
        (buff->read % buff->num_of_items == __atomic_load_n(&buff->head, __ATOMIC_SEQ_CST) % buff->num_of_items) ||
        /* Current item either not parsed or streamed */
        __atomic_load_n(&item->status, __ATOMIC_RELAXED) != CIRC_BUFF_ITEM_STATUS_DONE ){
        
        __atomic_store_n(&buff->tail, buff->read, __ATOMIC_SEQ_CST);

        /* Tail moved so update buff full flag in case it is set */
        __atomic_store_n(&buff->full, 0, __ATOMIC_SEQ_CST);

        __atomic_store_n(&buff->text_size_total, 0, __ATOMIC_RELAXED);
        __atomic_store_n(&buff->text_compressed_size_total, 0, __ATOMIC_RELAXED);
        return NULL;
    }

    buff->read++;

    return item;
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
};
