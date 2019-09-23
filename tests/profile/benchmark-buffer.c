/* SPDX-License-Identifier: GPL-3.0-or-later */

#include "config.h"
#include "libnetdata/libnetdata.h"
#include <pthread.h>
#include <stdlib.h>

#define NUMTHREADS 8

pthread_t threads[NUMTHREADS];

//ugly workaround for missing symbol
void send_statistics(const char *action, const char *action_result, const char *action_data){
}

void netdata_cleanup_and_exit(int ret) { exit(ret); }

pthread_mutex_t mprintf_lock = PTHREAD_MUTEX_INITIALIZER;
#define muprintf(...) { pthread_mutex_lock(&mprintf_lock); printf (__VA_ARGS__); fflush(stdout); pthread_mutex_unlock(&mprintf_lock); }

pthread_barrier_t next_test_barrier;
void threads_sync_next_test_start(){
    pthread_barrier_wait(&next_test_barrier);
}
#define threads_synce_test_end threads_sync_next_test_start

typedef struct {
    char cmd;
    size_t size;
} test_command;

//based on real life scenario
//test run of full netdata and thread WEB static 3
test_command test2_cmmds[] = {
    { '+',  16384 },
    { '+',  4096 },
    { '+',  4096 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '+', 100 },
    { '-', 100 },
    { '-', 4096 },
    { '-', 4096 },
    { '+', 16384 },
    { '+', 4096 },
    { '+', 4096 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '-', 4096 },
    { '-', 16384 },
    { '-', 4096 },
    { '-', 16384 },
    { NULL, 0}
};

#define TEST2_BUFFER_MAX_COUNT 100
#define TEST2_REPEAT_COUNT 250000

int test2_insert_stack(BUFFER **stack, size_t size, BUFFER* buffer) {
    if(!buffer || !stack)
        return 1;
    for(int i = 0; i < TEST2_BUFFER_MAX_COUNT; i++){
        if(!stack[i]) {
            stack[i] = buffer;
            return 0;
        }
    }
    return 1;
}

void test2( void *name ) {
    BUFFER **stack = calloc(TEST2_BUFFER_MAX_COUNT, sizeof(BUFFER*));

    for(int repeat = 0; repeat < TEST2_REPEAT_COUNT; repeat++) {
        int i = 0;
        while(test2_cmmds[i].cmd) {
            if(test2_cmmds[i].cmd == '+') {
                BUFFER *new = buffer_create(test2_cmmds[i].size);
                if(test2_insert_stack(stack, TEST2_BUFFER_MAX_COUNT, new)) {
                    muprintf("%s: Out of space in working buffer.\n", name);
                    exit(1);
                }
            } else if (test2_cmmds[i].cmd == '-') {
                for(int j = 0; j < TEST2_BUFFER_MAX_COUNT; j++){
/*                    if(stack[j]) {
                        muprintf("Alloc: Size req:%zu real:%zu\n", test2_cmmds[i].size, stack[j]->size);
                    }*/
                    if(stack[j] && stack[j]->size == test2_cmmds[i].size){
                        buffer_free(stack[j]);
                        stack[j] = NULL;
                    }
                }
            }
            i++;
        }
    }

    for(int j = 0; j < TEST2_BUFFER_MAX_COUNT; j++)
        if(stack[j])
            buffer_free(stack[j]);

    free((void*) stack);
}

#define TESTA_REPEAT_COUNT 10000000
#define TESTA_BUF_SIZE 256
void testA( void *tname ) {
    BUFFER* buffer;
    for(long long int i = 0; i < TESTA_REPEAT_COUNT; i++) {
        buffer = buffer_create(TESTA_BUF_SIZE);
        buffer_snprintf(buffer, TESTA_BUF_SIZE, "Test1: %s", tname);
        buffer_free(buffer);
    }
}

typedef void *(*test_fnc)(void*);

test_fnc test_list[] = {
    testA,
    test2,
    testA,
    NULL
};

void *thread_test_worker( void *ptr ) {
    int i = 0;
    test_fnc current_test = test_list[0];
    while(test_list[i]) {
        threads_sync_next_test_start();
        test_list[i](ptr);
        threads_synce_test_end();
        i++;
    }
    free(ptr);
}

#define TNAME_BUFSIZE 100
int main(int argc, char **argv) {
    printf("Benchmarking netdata Buffer.c\n");
    printf("Mempool is: %s\n", buffer_mempool_status());

    pthread_mutex_init(&mprintf_lock, NULL);
    pthread_barrier_init(&next_test_barrier, NULL, NUMTHREADS+1); //+1 is for main - to be able to synchronize tests

    char *tname = NULL;
    for(int i = 0; i < NUMTHREADS; i++) {
        tname = calloc(1, TNAME_BUFSIZE);
        snprintf(tname, TNAME_BUFSIZE, "ThreadIdx%02d", i);
        if(pthread_create(&threads[i], NULL, thread_test_worker, (void*)tname)) {
            printf("Failed to create thread %d.\n", i);
            exit(1);
        }
    }

    int i = 0;
    test_fnc current_test = test_list[0];
    while(test_list[i]) {
        muprintf("Starting Test %d\n", i+1);
        threads_sync_next_test_start();
        threads_synce_test_end();
        i++;
    }

    BUFFER *buf = buffer_create(256);
    for(int i = 0; i < NUMTHREADS; i++) {
        if(pthread_join(threads[i], NULL)) {
            printf("Failed to join thread %d.\n", i);
            exit(1);
        }
    }

    printf("ALL SUCCESSFUL.\n");
    exit(0);
}
