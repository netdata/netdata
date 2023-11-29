// SPDX-License-Identifier: GPL-3.0-or-later

/** @file circular_buffer.h
 *  @brief Header of circular_buffer.c 
 */

#ifndef CIRCULAR_BUFFER_H_
#define CIRCULAR_BUFFER_H_

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <uv.h>
#include "defaults.h"
#include "query.h"
#include "file_info.h"

// Forward declaration to break circular dependency
struct File_info;

typedef enum {
    CIRC_BUFF_ITEM_STATUS_UNPROCESSED = 0,
    CIRC_BUFF_ITEM_STATUS_PARSED = 1,
    CIRC_BUFF_ITEM_STATUS_STREAMED = 2,
    CIRC_BUFF_ITEM_STATUS_DONE = 3              // == CIRC_BUFF_ITEM_STATUS_PARSED | CIRC_BUFF_ITEM_STATUS_STREAMED
} circ_buff_item_status_t;

typedef struct Circ_buff_item {
    circ_buff_item_status_t status;				/**< Denotes if item is unprocessed, in processing or processed **/
    msec_t timestamp;							/**< Epoch datetime of when data was collected **/
    char *data;									/**< Base of buffer to store both uncompressed and compressed logs **/
    size_t text_size;							/**< Size of uncompressed logs **/
    char *text_compressed;						/**< Pointer offset within *data that points to start of compressed logs **/
    size_t text_compressed_size;				/**< Size of compressed logs **/
    size_t data_max_size;						/**< Allocated size of *data **/
    unsigned long num_lines;                    /**< Number of log records in item */
} Circ_buff_item_t;

typedef struct Circ_buff {
    int num_of_items;                           /**< Number of preallocated items in the buffer **/
    Circ_buff_item_t *items;                    /**< Array of all circular buffer items **/
    Circ_buff_item_t *in;                       /**< Circular buffer item to write new data into **/
    int head;							        /**< Position of next item insertion **/
    int read;							        /**< Index between tail and head, used to read items out of Circ_buff **/
    int tail;							        /**< Last valid item in Circ_buff **/
    int parse;									/**< Points to next item in buffer to be parsed **/
    int full;							        /**< When head == tail, this indicates if buffer is full or empty **/
    uv_rwlock_t buff_realloc_rwlock;            /**< RW lock to lock buffer operations when reallocating or expanding buffer **/
    unsigned int buff_realloc_cnt;              /**< Counter of how any buffer reallocations have occurred **/
    size_t total_cached_mem;				    /**< Total memory allocated for Circ_buff (excluding *in) **/
    size_t total_cached_mem_max;				/**< Maximum allowable size for total_cached_mem **/
    int allow_dropped_logs;                     /**< Boolean to indicate whether logs are allowed to be dropped if buffer is full */
    size_t text_size_total;				        /**< Total size of items[]->text_size **/
    size_t text_compressed_size_total;	        /**< Total size of items[]->text_compressed_size **/
    int compression_ratio;				        /**< text_size_total / text_compressed_size_total **/
} Circ_buff_t;

void circ_buff_search(logs_query_params_t *const p_query_params, struct File_info *const p_file_infos[]);
size_t circ_buff_prepare_write(Circ_buff_t *const buff, size_t const requested_text_space);
int circ_buff_insert(Circ_buff_t *const buff);
Circ_buff_item_t *circ_buff_read_item(Circ_buff_t *const buff);
void circ_buff_read_done(Circ_buff_t *const buff);
Circ_buff_t *circ_buff_init(const int num_of_items, const size_t max_size, const int allow_dropped_logs);
void circ_buff_destroy(Circ_buff_t *buff);

#endif  // CIRCULAR_BUFFER_H_
