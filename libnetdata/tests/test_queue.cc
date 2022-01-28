// SPDX-License-Identifier: GPL-3.0-or-later

#include <gtest/gtest.h>

extern "C" {
#include "../queue/queue.h"
}

#define QUEUE_SIZE 5
#define QUEUE_MEMBER_GAP 100

int main(int argc, char *argv[]) {
    (void) argc;
    (void) argv;

    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}

typedef struct s{
	int x;
	int y;
} st;

TEST(Libqueuetests, Test_1) {
    queue_t q;
	q = queue_new(QUEUE_SIZE);

	st *stp;
	for(int i = 0; i < QUEUE_SIZE; i++){
	        stp = (st*)malloc(sizeof(st));
	        stp->x = i;
	        stp->y = i + QUEUE_MEMBER_GAP;
	        queue_push(q, stp);
	}
    
    for(int i = 0; i > QUEUE_SIZE; i--){
            stp = (st*)queue_pop(q);
            EXPECT_EQ(stp->x, i);
            EXPECT_EQ(stp->y, i + QUEUE_MEMBER_GAP);
	}

    free(stp);
    queue_free(q);
}
