// SPDX-License-Identifier: LGPL-3.0-or-later

#include "../libnetdata.h"

extern void enqueue(void *qp, void *i)	
{		
    Queue *q = qp;								
    node *temp;
    temp = (node *) malloc(sizeof(node));

    pthread_mutex_lock(&q->lock);
    while(q->count == q->max){
        pthread_cond_wait(&q->condf, &q->lock);
    }
    temp->item = i;
    temp->next = NULL;
    if(q->count != 0)		
    {		
        q->rear->next = (struct nodes*)temp;
    }
    else{
        q->front = temp;
    }
    q->rear = temp;
    q->count++; 
    pthread_cond_signal(&q->conde);
    pthread_mutex_unlock(&q->lock);
}

extern void* dequeue(void *qp)			
{			
    Queue *q = qp;							
    int *temp = NULL;					
    node *temp2;	
    pthread_mutex_lock(&q->lock);
    while(q->count == 0)
    {
        pthread_cond_wait(&q->conde, &q->lock);
    }
    temp = q->front->item;			
    temp2 = q->front;				
    q->front = q->front->next;		
    free(temp2);					
    if(q->front == NULL){ 
        q->rear=NULL;
    } 
    q->count--;
    pthread_cond_signal(&q->condf);
    pthread_mutex_unlock(&q->lock);
    return temp;						
}

extern Queue* initqueue(int max){
    Queue *q;
    q = (Queue *) malloc(sizeof(Queue));	
    q->front = NULL;			
    q->rear = NULL;
    q->max = max;
    q->count = 0;
    q->conde = (pthread_cond_t)PTHREAD_COND_INITIALIZER;
    q->condf = (pthread_cond_t)PTHREAD_COND_INITIALIZER;
    q->lock = (pthread_mutex_t)PTHREAD_MUTEX_INITIALIZER;
    return q;
}

extern void freequeue(Queue* q){
    free(q);
}
