// SPDX-License-Identifier: LGPL-3.0-or-later

#ifndef NETDATA_QUEUE_H
#define NETDATA_QUEUE_H 1

#include "stdlib.h"
#include "pthread.h"

typedef struct nodes *node;
typedef struct queues *queue;

extern queue initqueue(int max);
extern void freequeue(queue q);
extern int enqueue(queue, void*);	
extern void* dequeue(queue);				

#endif //NETDATA_QUEUE_H
