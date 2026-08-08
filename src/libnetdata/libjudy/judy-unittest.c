// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#define JUDY_TEST(condition, message) do {                                               \
        if (!(condition)) {                                                               \
            fprintf(stderr, "judy unittest FAILED: %s (%s:%d)\\n",                    \
                    (message), __FUNCTION__, __LINE__);                                  \
            errors++;                                                                     \
        }                                                                                 \
    } while (0)

#define JUDY_TEST_DENSE_ENTRIES 4096
#define JUDY_TEST_REPETITIONS 32

static int judy_unittest_cycle(void) {
#ifdef JU_64BIT
    static const Word_t sparse_keys[] = {
        UINT64_C(0x0000000100000000),
        UINT64_C(0x0000ffff00000000),
        UINT64_C(0x7fffffffffffffff),
        UINT64_MAX,
    };
#else
    static const Word_t sparse_keys[] = {
        UINT32_C(0x00010000),
        UINT32_C(0x7fffffff),
        UINT32_MAX,
    };
#endif

    int errors = 0;
    Pvoid_t array = NULL;

    for (Word_t index = 0; index < JUDY_TEST_DENSE_ENTRIES; index++) {
        Pvoid_t *value = JudyLIns(&array, index, PJE0);
        JUDY_TEST(value && value != PJERR, "insert dense key");
        if (!value || value == PJERR)
            goto cleanup;

        *value = (Pvoid_t)(uintptr_t)(index + 1);
    }

    for (size_t i = 0; i < _countof(sparse_keys); i++) {
        Pvoid_t *value = JudyLIns(&array, sparse_keys[i], PJE0);
        JUDY_TEST(value && value != PJERR, "insert sparse key");
        if (!value || value == PJERR)
            goto cleanup;

        *value = (Pvoid_t)(uintptr_t)(JUDY_TEST_DENSE_ENTRIES + i + 1);
    }

    bool first = true;
    Word_t index = 0;
    size_t count = 0;
    Pvoid_t *value;
    while ((value = JudyLFirstThenNext(array, &index, &first))) {
        JUDY_TEST(value != PJERR, "ordered iteration does not return an error");
        if (value == PJERR)
            break;

        // A broken JudyLNext() must fail this test rather than hang the test process.
        JUDY_TEST(count < JUDY_TEST_DENSE_ENTRIES + _countof(sparse_keys),
                  "ordered iteration terminates");
        if (count >= JUDY_TEST_DENSE_ENTRIES + _countof(sparse_keys))
            break;

        Word_t expected_index = count < JUDY_TEST_DENSE_ENTRIES ?
                                        (Word_t)count : sparse_keys[count - JUDY_TEST_DENSE_ENTRIES];
        JUDY_TEST(index == expected_index, "ordered iteration returns the expected key");
        JUDY_TEST((uintptr_t)*value == count + 1, "ordered iteration returns the expected value");
        count++;
    }

    JUDY_TEST(count == JUDY_TEST_DENSE_ENTRIES + _countof(sparse_keys),
              "ordered iteration visits every key exactly once");

cleanup:
    if (array) {
        Word_t freed = JudyLFreeArray(&array, PJE0);
        JUDY_TEST(freed != (Word_t)JERR, "destroy populated array");
        JUDY_TEST(array == NULL, "destroy clears the root pointer");
    }

    return errors;
}

int judy_unittest(void) {
    int errors = 0;

    fprintf(stderr, "\\nrunning Judy unittest\\n");

    for (size_t repetition = 0; repetition < JUDY_TEST_REPETITIONS; repetition++)
        errors += judy_unittest_cycle();

    if (errors)
        fprintf(stderr, "Judy unittest: %d ERROR(S)\\n", errors);
    else
        fprintf(stderr, "Judy unittest: OK\\n");

    return errors;
}
