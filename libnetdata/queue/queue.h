// SPDX-License-Identifier: LGPL-3.0-or-later

#ifndef NETDATA_QUEUE_H
#define NETDATA_QUEUE_H 1

#include "stdlib.h"
#include "pthread.h"

typedef struct node_s *node;
typedef struct queue_s *queue_t;

struct node_s{
	void *item;
	struct node_s *next;			
};

struct queue_s{		
	pthread_cond_t conde;
	pthread_cond_t condf;
	pthread_mutex_t lock;
	node front;			
	node rear;
	int max;
	int count;
	int blocked;
};

queue_t queue_new(int size, int blocked);
void queue_free(queue_t q);
int queue_push(queue_t q, void *data);
void *queue_pop(queue_t q);				

#endif //NETDATA_QUEUE_H
