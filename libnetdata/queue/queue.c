// SPDX-License-Identifier: LGPL-3.0-or-later

#include "queue.h"

struct node_s{
	void *item;
	struct node_s *next;			
};

typedef struct node_s *node;

struct queue_s{		
	pthread_cond_t conde;
	pthread_cond_t condf;
	pthread_mutex_t lock;
	node front;			
	node rear;
	int max;
	int count;
};

/*
 * @Input:
 *      q / The queue instance that the item will be pushed into
 * 		i / The item which will be pushed into the queue
 * @Return
 *      If the node is pushed the queue, it returns with 1, otherwise 0 
 */
int queue_push(queue_t q, void* i)	
{		
	node temp;
	temp = (node) malloc(sizeof(struct node_s));
	if (temp == NULL){
		return 0;
	}
	pthread_mutex_lock(&q->lock);
	while(q->count == q->max){
		pthread_cond_wait(&q->condf, &q->lock);
	}
	temp->item = i;
	temp->next = NULL;
	if(q->count != 0)		
	{		
		q->rear->next = (node)temp;
	}
	else{
		q->front = temp;
	}
	q->rear = temp;
    	q->count++; 
	pthread_cond_signal(&q->conde);
	pthread_mutex_unlock(&q->lock);
	return 1;
}

/*
 * @Input:
 *      q / The queue instance that the item will be popped from
 * @Return
 *      Returns the node if it is popped succesfully, otherwise NULL
 */
void* queue_pop(queue_t q)			
{			
	void* temp = NULL;					
	node temp2;	
	pthread_mutex_lock(&q->lock);
	while(q->count == 0)
	{
		pthread_cond_wait(&q->conde, &q->lock);
	}
	
	temp = q->front->item;			
	temp2 = q->front;				
	q->front = q->front->next;		
	free(temp2);					
	if(q->front==NULL){
		q->rear=NULL;
	} 
	q->count--;
	pthread_cond_signal(&q->condf);
	pthread_mutex_unlock(&q->lock);
	return temp;						
}

/*
 * @Input:
 *      max / The queue size.
 * @Return
 *      Returns the queue if it is created successfully, otherwise NULL
 */
queue_t queue_new(int max){
	queue_t q = NULL;
	q = (queue_t) malloc(sizeof(struct queue_s));	
	q->front = NULL;			
	q->rear = NULL;
	q->max = max;
	q->count = 0;
	q->conde = (pthread_cond_t)PTHREAD_COND_INITIALIZER;
	q->condf = (pthread_cond_t)PTHREAD_COND_INITIALIZER;
	q->lock = (pthread_mutex_t)PTHREAD_MUTEX_INITIALIZER;
	return q;
}

/*
 * @Input:
 *      q / The queue which will be deleted.
 */
void queue_free(queue_t q){
	free(q);
}
