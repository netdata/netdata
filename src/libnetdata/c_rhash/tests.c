// SPDX-License-Identifier: GPL-3.0-or-later

#include <stdio.h>
#include <string.h>

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
    rc = 1; \
    goto test_cleanup; \
} passed_subtest_count++;};

#define ASSERT_VAL_UINT8(returned, expected) \
if(returned != expected) { \
    PRINT_ERR("Failed test. Value returned (%d) doesn't match expected (%d)! fnc:\"%s\",line:%d", returned, expected, __FUNCTION__, __LINE__); \
    rc = 1; \
    goto test_cleanup; \
} passed_subtest_count++;

#define ASSERT_VAL_PTR(returned, expected) \
if((void*)returned != (void*)expected) { \
    PRINT_ERR("Failed test. Value returned(%p) doesn't match expected(%p)! fnc:\"%s\",line:%d", (void*)returned, (void*)expected, __FUNCTION__, __LINE__); \
    rc = 1; \
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

struct test_string {
    const char *str;
    int counter;
};

struct test_string test_strings[] = {
    { .str = "Cillum reprehenderit eiusmod elit nisi aliquip esse exercitation commodo Lorem voluptate esse.", .counter = 0 },
    { .str = "Ullamco eiusmod tempor occaecat ad.", .counter = 0 },
    { .str = "Esse aliquip tempor sint tempor ullamco duis aute incididunt ad.", .counter = 0 },
    { .str = "Cillum Lorem labore cupidatat commodo proident adipisicing.", .counter = 0 },
    { .str = "Quis ad cillum officia exercitation.", .counter = 0 },
    { .str = "Ipsum enim dolor ullamco amet sint nisi ut occaecat sint non.", .counter = 0 },
    { .str = "Id duis officia ipsum cupidatat velit fugiat.", .counter = 0 },
    { .str = "Aliqua non occaecat voluptate reprehenderit reprehenderit veniam minim exercitation ea aliquip enim aliqua deserunt qui.", .counter = 0 },
    { .str = "Ullamco elit tempor laboris reprehenderit quis deserunt duis quis tempor reprehenderit magna dolore reprehenderit exercitation.", .counter = 0 },
    { .str = "Culpa do dolor quis incididunt et labore in ex.", .counter = 0 },
    { .str = "Aliquip velit cupidatat qui incididunt ipsum nostrud eiusmod ut proident nisi magna fugiat excepteur.", .counter = 0 },
    { .str = "Aliqua qui dolore tempor id proident ullamco sunt magna.", .counter = 0 },
    { .str = "Labore eiusmod ut fugiat dolore reprehenderit mollit magna.", .counter = 0 },
    { .str = "Veniam aliquip dolor excepteur minim nulla esse cupidatat esse.", .counter = 0 },
    { .str = "Do quis dolor irure nostrud occaecat aute proident anim.", .counter = 0 },
    { .str = "Enim veniam non nulla ad quis sit amet.", .counter = 0 },
    { .str = "Cillum reprehenderit do enim esse do ullamco consectetur ea.", .counter = 0 },
    { .str = "Sit et duis sint anim qui ad anim labore exercitation sunt cupidatat.", .counter = 0 },
    { .str = "Dolor officia adipisicing sint pariatur in dolor occaecat officia reprehenderit magna.", .counter = 0 },
    { .str = "Aliquip dolore qui occaecat eiusmod sunt incididunt reprehenderit minim et.", .counter = 0 },
    { .str = "Aute fugiat laboris cillum tempor consequat tempor do non laboris culpa officia nisi.", .counter = 0 },
    { .str = "Et excepteur do aliquip fugiat nisi velit tempor officia enim quis elit incididunt.", .counter = 0 },
    { .str = "Eu officia adipisicing incididunt occaecat officia cupidatat enim sit sit officia.", .counter = 0 },
    { .str = "Do amet cillum duis pariatur commodo nulla cillum magna nulla Lorem veniam cupidatat.", .counter = 0 },
    { .str = "Dolor adipisicing voluptate laboris occaecat culpa aliquip ipsum ut consequat aliqua aliquip commodo sunt velit.", .counter = 0 },
    { .str = "Nulla proident ipsum quis nulla.", .counter = 0 },
    { .str = "Laborum adipisicing nulla do aute aliqua est quis sint culpa pariatur laborum voluptate qui.", .counter = 0 },
    { .str = "Proident eiusmod sunt et nulla elit pariatur dolore irure ex voluptate excepteur adipisicing consectetur.", .counter = 0 },
    { .str = "Consequat ex voluptate officia excepteur aute deserunt proident commodo et.", .counter = 0 },
    { .str = "Velit sit cupidatat dolor dolore.", .counter = 0 },
    { .str = "Sunt enim do non anim nostrud exercitation ullamco ex proident commodo.", .counter = 0 },
    { .str = "Id ex officia cillum ad.", .counter = 0 },
    { .str = "Laboris in sunt eiusmod veniam laboris nostrud.", .counter = 0 },
    { .str = "Ex magna occaecat ea ea incididunt aliquip.", .counter = 0 },
    { .str = "Sunt eiusmod ex nostrud eu pariatur sit cupidatat ea adipisicing cillum culpa esse consequat aliquip.", .counter = 0 },
    { .str = "Excepteur commodo qui incididunt enim culpa sunt non excepteur Lorem adipisicing.", .counter = 0 },
    { .str = "Quis officia est ullamco reprehenderit incididunt occaecat pariatur ex reprehenderit nisi.", .counter = 0 },
    { .str = "Culpa irure proident proident et eiusmod irure aliqua ipsum cupidatat minim sit.", .counter = 0 },
    { .str = "Qui cupidatat aliquip est velit magna veniam.", .counter = 0 },
    { .str = "Pariatur ad ad mollit nostrud non irure minim veniam anim aliquip quis eu.", .counter = 0 },
    { .str = "Nisi ex minim eu adipisicing tempor Lorem nisi do ad exercitation est non eu.", .counter = 0 },
    { .str = "Cupidatat do mollit ad commodo cupidatat ut.", .counter = 0 },
    { .str = "Est non excepteur eiusmod nostrud et eu.", .counter = 0 },
    { .str = "Cupidatat mollit nisi magna officia ut elit eiusmod.", .counter = 0 },
    { .str = "Est aliqua consectetur laboris ex consequat est ut dolor.", .counter = 0 },
    { .str = "Duis eu laboris laborum ut id Lorem nostrud qui ad velit proident fugiat minim ullamco.", .counter = 0 },
    { .str = "Pariatur esse excepteur anim amet excepteur irure sint quis esse ex cupidatat ut.", .counter = 0 },
    { .str = "Esse reprehenderit amet qui excepteur aliquip amet.", .counter = 0 },
    { .str = "Ullamco laboris elit labore adipisicing aute nulla qui laborum tempor officia ut dolor aute.", .counter = 0 },
    { .str = "Commodo sunt cillum velit minim laborum Lorem aliqua tempor ad id eu.", .counter = 0 },
    { .str = NULL, .counter = 0 }
};

uint32_t test_strings_contain_element(const char *str) {
    struct test_string *str_desc = test_strings;
    while(str_desc->str) {
        if (!strcmp(str, str_desc->str))
            return str_desc - test_strings;
        str_desc++;
    }
    return -1;
}

#define TEST_INCREMENT_STR_KEYS_HASH_SIZE 20
int test_increment_str_keys() {
    c_rhash hash;
    const char *key;

    TEST_START();

    hash = c_rhash_new(TEST_INCREMENT_STR_KEYS_HASH_SIZE); // less than element count of test_strings

    c_rhash_iter_t iter = C_RHASH_ITER_T_INITIALIZER;

    // check iter on empty hash
    ASSERT_RETVAL(c_rhash_iter_str_keys, !=, 0, hash, &iter, &key);

    int32_t element_count = 0;
    while (test_strings[element_count].str) {
        ASSERT_RETVAL(c_rhash_insert_str_ptr, ==, 0, hash, test_strings[element_count].str, NULL);
        test_strings[element_count].counter++; // we want to test we got each key exactly once
        element_count++;
    }

    if (element_count <= TEST_INCREMENT_STR_KEYS_HASH_SIZE * 2) {
        // verify we are actually test also iteration trough single bin (when 2 keys have same hash pointing them to same bin)
        PRINT_ERR("For this test to properly test all the hash size needs to be much smaller than all test key count.");
        rc = 1;
        goto test_cleanup;
    }

    // we insert another type of key as iterator should skip it
    // in case is another type
    ASSERT_RETVAL(c_rhash_insert_uint64_ptr, ==, 0, hash, 5, NULL);

    c_rhash_iter_t_initialize(&iter);
    while(!c_rhash_iter_str_keys(hash, &iter, &key)) {
        element_count--;
        int i;
        if ( (i = test_strings_contain_element(key)) < 0) {
            PRINT_ERR("Key \"%s\" is not present in test_strings array! (Fnc: %s, Line: %d)", key, __FUNCTION__, __LINE__);
            rc = 1;
            goto test_cleanup;
        }
        passed_subtest_count++;

        test_strings[i].counter--;
    }
    ASSERT_VAL_UINT8(element_count, 0); // we added also same non string keys

    // check each key was present exactly once
    struct test_string *str_desc = test_strings;
    while (str_desc->str) {
        ASSERT_VAL_UINT8(str_desc->counter, 0);
        str_desc++;
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
    RUN_TEST(test_increment_str_keys);
    // TODO hash with mixed key tests
    // TODO iterator test
    return 0;
}
