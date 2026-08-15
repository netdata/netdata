#ifndef PAGE_TEST_H
#define PAGE_TEST_H

#ifdef __cplusplus
extern "C" {
#endif

int pgd_test(int argc, char *argv[]);
#if defined(__GNUC__) || defined(__clang__)
__attribute__((visibility("hidden")))
#endif
int pgd_storage_point_unittest(void);

#ifdef __cplusplus
}
#endif

#endif /* PAGE_TEST_H */
