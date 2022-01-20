// SPDX-License-Identifier: LGPL-3.0-or-later

#ifndef NETDATA_QUEUE_H
#define NETDATA_QUEUE_H 

#include "../libnetdata.h"
		
typedef struct nodes *node;
typedef struct queues *queue;

queue initqueue(int max);
void freequeue(queue q);
void enqueue(queue, void*);	
void* dequeue(queue);				

#endif //NETDATA_QUEUE_H
