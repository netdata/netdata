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
