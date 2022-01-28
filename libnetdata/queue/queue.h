// SPDX-License-Identifier: LGPL-3.0-or-later

#ifndef NETDATA_QUEUE_H
#define NETDATA_QUEUE_H 1

#include "stdlib.h"
#include "pthread.h"

typedef struct queue_s *queue_t;

queue_t queue_new(int size);
void queue_free(queue_t q);
int queue_push(queue_t q, void *data);
void *queue_pop(queue_t q);				

#endif //NETDATA_QUEUE_H
