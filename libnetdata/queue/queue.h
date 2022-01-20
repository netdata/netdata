// SPDX-License-Identifier: LGPL-3.0-or-later

#ifndef NETDATA_QUEUE_H
#define NETDATA_QUEUE_H 

#include "../libnetdata.h"
		
typedef struct nodes				
{
	void *item;
	struct nodes *next;			
} node;

typedef struct Queues			
{		
    pthread_cond_t conde;
    pthread_cond_t condf;
    pthread_mutex_t lock;
    node *front;			
    node *rear;
    int max;
    int count;
} Queue;

Queue* initqueue(int max);
void freequeue(Queue* q);
void enqueue(void *q, void* item);	
void* dequeue(void *q);				

#endif //NETDATA_QUEUE_H
