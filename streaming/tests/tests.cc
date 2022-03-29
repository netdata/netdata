
#include "tests.h"

// Add unit tests here
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

// test main run tests
int main(int argc, char *argv[]) {
    (void) argc;
    (void) argv;

    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}