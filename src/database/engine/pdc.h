// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef DBENGINE_PDC_H
#define DBENGINE_PDC_H

#include "../engine/rrdengine.h"

struct rrdeng_cmd;

#define PDCJudyLIns             JudyLIns
#define PDCJudyLGet             JudyLGet
#define PDCJudyLFirst           JudyLFirst
#define PDCJudyLNext            JudyLNext
#define PDCJudyLLast            JudyLLast
#define PDCJudyLPrev            JudyLPrev
#define PDCJudyLFirstThenNext   JudyLFirstThenNext
#define PDCJudyLLastThenPrev    JudyLLastThenPrev
#define PDCJudyLFreeArray       JudyLFreeArray

typedef struct extent_page_details_list EPDL;
typedef void (*execute_extent_page_details_list_t)(struct rrdengine_instance *ctx, EPDL *epdl, enum storage_priority priority);
void pdc_to_epdl_router(struct rrdengine_instance *ctx, struct page_details_control *pdc, execute_extent_page_details_list_t exec_first_extent_list, execute_extent_page_details_list_t exec_rest_extent_list);
void epdl_find_extent_and_populate_pages(struct rrdengine_instance *ctx, EPDL *epdl, bool worker);

struct aral_statistics *pdc_aral_stats(void);
struct aral_statistics *pd_aral_stats(void);
struct aral_statistics *epdl_aral_stats(void);
struct aral_statistics *deol_aral_stats(void);

size_t extent_buffer_cache_size(void);

void pdc_init(void);
void page_details_init(void);
void epdl_init(void);
void deol_init(void);
void extent_buffer_cleanup1(void);

void epdl_cmd_dequeued(void *epdl_ptr);
void epdl_cmd_queued(void *epdl_ptr, struct rrdeng_cmd *cmd);

struct extent_buffer {
    size_t bytes;

    struct {
        struct extent_buffer *prev;
        struct extent_buffer *next;
    } cache;

    uint8_t data[];
};

void extent_buffer_init(void);
struct extent_buffer *extent_buffer_get(size_t size);
void extent_buffer_release(struct extent_buffer *eb);

#endif // DBENGINE_PDC_H
