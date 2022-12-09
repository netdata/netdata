#include <stdio.h>

#include "c_rhash.h"

// terminal color codes
#define KNRM "\x1B[0m"
#define KRED "\x1B[31m"
#define KGRN "\x1B[32m"
#define KYEL "\x1B[33m"
#define KBLU "\x1B[34m"
#define KMAG "\x1B[35m"
#define KCYN "\x1B[36m"
#define KWHT "\x1B[37m"

#define KEY_1 "key1"
#define KEY_2 "keya"

#define PRINT_ERR(str, ...) fprintf(stderr, "└─╼ ❌ " KRED str KNRM "\n" __VA_OPT__(,) __VA_ARGS__)

#define ASSERT_RETVAL(fnc, comparator, expected_retval, ...) \
{ int rval; \
if(!((rval = fnc(__VA_ARGS__)) comparator expected_retval)) { \
    PRINT_ERR("Failed test. Value returned by \"%s\" in fnc:\"%s\",line:%d is not equal to expected value. Expected:%d, Got:%d", #fnc, __FUNCTION__, __LINE__, expected_retval, rval); \
    goto test_cleanup; \
}} passed_subtest_count++;

#define ASSERT_VAL_UINT8(returned, expected) \
if(returned != expected) { \
    PRINT_ERR("Failed test. Value returned (%d) from hash doesn't match expected (%d)! fnc:\"%s\",line:%d", returned, expected, __FUNCTION__, __LINE__); \
    goto test_cleanup; \
} passed_subtest_count++;

#define ASSERT_VAL_PTR(returned, expected) \
if((void*)returned != (void*)expected) { \
    PRINT_ERR("Failed test. Value returned(%p) from hash doesn't match expected(%p)! fnc:\"%s\",line:%d", (void*)returned, (void*)expected, __FUNCTION__, __LINE__); \
    goto test_cleanup; \
} passed_subtest_count++;

#define ALL_SUBTESTS_PASS() printf("└─╼ ✅" KGRN " Test \"%s\" DONE. All of %zu subtests PASS. (line:%d)\n" KNRM, __FUNCTION__, passed_subtest_count, __LINE__);

#define TEST_START() size_t passed_subtest_count = 0; int rc = 0; printf("╒═ Starting test \"%s\"\n", __FUNCTION__);

int test_str_uint8() {
    c_rhash hash = c_rhash_new(100);
    uint8_t val;

    TEST_START();
    // function should fail on empty hash
    ASSERT_RETVAL(c_rhash_get_uint8_by_str, !=, 0, hash, KEY_1, &val);

    ASSERT_RETVAL(c_rhash_insert_str_uint8, ==, 0, hash, KEY_1, 5);
    ASSERT_RETVAL(c_rhash_get_uint8_by_str, ==, 0, hash, KEY_1, &val);
    ASSERT_VAL_UINT8(5, val);

    ASSERT_RETVAL(c_rhash_insert_str_uint8, ==, 0, hash, KEY_2, 8);
    ASSERT_RETVAL(c_rhash_get_uint8_by_str, ==, 0, hash, KEY_1, &val);
    ASSERT_VAL_UINT8(5, val);
    ASSERT_RETVAL(c_rhash_get_uint8_by_str, ==, 0, hash, KEY_2, &val);
    ASSERT_VAL_UINT8(8, val);
    ASSERT_RETVAL(c_rhash_get_uint8_by_str, !=, 0, hash, "sndnskjdf", &val);

    // test update of key
    ASSERT_RETVAL(c_rhash_insert_str_uint8, ==, 0, hash, KEY_1, 100);
    ASSERT_RETVAL(c_rhash_get_uint8_by_str, ==, 0, hash, KEY_1, &val);
    ASSERT_VAL_UINT8(100, val);

    ALL_SUBTESTS_PASS();
test_cleanup:
    c_rhash_destroy(hash);
    return rc;
}

int test_uint64_ptr() {
    c_rhash hash = c_rhash_new(100);
    void *val;

    TEST_START();

    // function should fail on empty hash
    ASSERT_RETVAL(c_rhash_get_ptr_by_uint64, !=, 0, hash, 0, &val);

    ASSERT_RETVAL(c_rhash_insert_uint64_ptr, ==, 0, hash, 0, &hash);
    ASSERT_RETVAL(c_rhash_get_ptr_by_uint64, ==, 0, hash, 0, &val);
    ASSERT_VAL_PTR(&hash, val);

    ASSERT_RETVAL(c_rhash_insert_uint64_ptr, ==, 0, hash, 1, &val);
    ASSERT_RETVAL(c_rhash_get_ptr_by_uint64, ==, 0, hash, 0, &val);
    ASSERT_VAL_PTR(&hash, val);
    ASSERT_RETVAL(c_rhash_get_ptr_by_uint64, ==, 0, hash, 1, &val);
    ASSERT_VAL_PTR(&val, val);
    ASSERT_RETVAL(c_rhash_get_ptr_by_uint64, !=, 0, hash, 2, &val);

    ALL_SUBTESTS_PASS();
test_cleanup:
    c_rhash_destroy(hash);
    return rc;
}

#define UINT64_PTR_INC_ITERATION_COUNT 5000
int test_uint64_ptr_incremental() {
    c_rhash hash = c_rhash_new(100);
    void *val;

    TEST_START();

    char a = 0x20;
    char *ptr = &a;
    while(ptr < &a + UINT64_PTR_INC_ITERATION_COUNT) {
        ASSERT_RETVAL(c_rhash_insert_uint64_ptr, ==, 0, hash, (ptr-&a), ptr);
        ptr++;
    }

    ptr = &a;
    char *retptr;
    for(int i = 0; i < UINT64_PTR_INC_ITERATION_COUNT; i++) {
        ASSERT_RETVAL(c_rhash_get_ptr_by_uint64, ==, 0, hash, i, (void**)&retptr);
        ASSERT_VAL_PTR(retptr, (&a+i));
    }

    ALL_SUBTESTS_PASS();
test_cleanup:
    c_rhash_destroy(hash);
    return rc;
}

#define RUN_TEST(fnc) \
if(fnc()) \
    return 1;

int main(int argc, char *argv[]) {
    RUN_TEST(test_str_uint8);
    RUN_TEST(test_uint64_ptr);
    RUN_TEST(test_uint64_ptr_incremental);
    // TODO hash with mixed key tests
    // TODO iterator test
    return 0;
}
