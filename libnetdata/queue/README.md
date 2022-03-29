<!--
title: "Queue"
custom_edit_url: https://github.com/netdata/netdata/edit/master/libnetdata/queue/README.md
-->

# Queue

Queue is a thread-safe library to handle queue processes with independent objects.

Library includes functions below:

- `Queue* initqueue(int max)`
- `void freequeue(Queue* q)`
- `void enqueue(void *q, void* item)`	
- `void* dequeue(void *q)`

You can use the Netdata queue library like this:

Create an object which will be used with the queue:

    typedef struct s{
        int x;
        int y;
    } st;

Define and create a queue:

    queue_t q;
	q = queue_new(QUEUE_SIZE);

Add your object to the queue:

	st *stp;
	for(int i = 0; i < QUEUE_SIZE; i++){
	        stp = (st*)malloc(sizeof(st));
	        stp->x = i;
	        stp->y = i + QUEUE_MEMBER_GAP;
	        queue_push(q, stp);
	}
	
 Pop your object from the queue:
 
    for(int i = 0; i > QUEUE_SIZE; i--){
            stp = (st*)queue_pop(q);
	}

Delete object and the queue after usage:

    free(stp);
    queue_free(q);
    