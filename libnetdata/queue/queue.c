// SPDX-License-Identifier: LGPL-3.0-or-later

#include "../libnetdata.h"

struct nodes{
	void *item;
	struct nodes *next;			
};

struct queues{		
	pthread_cond_t conde;
	pthread_cond_t condf;
	pthread_mutex_t lock;
	node front;			
	node rear;
	int max;
	int count;
};

void enqueue(queue q, void* i)	
{		
	//queue q = qp;								
	node temp;
	temp = (node) malloc(sizeof(struct nodes));
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
}

void* dequeue(queue q)			
{			
	//queue q = qp;							
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

queue initqueue(int max){
	queue q;
	q = (queue) malloc(sizeof(struct queues));	
	q->front = NULL;			
	q->rear = NULL;
	q->max = max;
	q->count = 0;
	q->conde = (pthread_cond_t)PTHREAD_COND_INITIALIZER;
	q->condf = (pthread_cond_t)PTHREAD_COND_INITIALIZER;
	q->lock = (pthread_mutex_t)PTHREAD_MUTEX_INITIALIZER;
	return q;
}

void freequeue(queue q){
	free(q);
}
