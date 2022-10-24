#ifndef REPLICATION_H
#define REPLICATION_H

#ifdef __cplusplus
extern "C" {
#endif /* __cplusplus */

#include "daemon/common.h"

void replicate_chart_response(RRDHOST *rh, RRDSET *rs,
                              bool start_streaming, time_t after, long before);

bool replicate_chart_request(FILE *outfp, RRDHOST *rh, RRDSET *rs,
                             time_t first_entry_child, time_t last_entry_child,
                             time_t response_first_start_time, time_t response_last_end_time);

#ifdef __cplusplus
};
#endif /* __cplusplus */

#endif /* REPLICATION_H */
