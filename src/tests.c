#include "c_rhash.h"
#include <stdio.h>

#define KEY_1 "key1"
#define KEY_2 "keya"

int main(int argc, char *argv[]) {
    c_rhash hash = c_rhash_new(2);
    uint8_t val;
    c_rhash_insert_str_uint8(hash, KEY_1, 5);
    if(c_rhash_get_uint8_by_str(hash, KEY_1, &val))
        printf(" key not found\n");
    else
        printf(" value is %d\n", (int)val);
    c_rhash_insert_str_uint8(hash, KEY_2, 8);
    c_rhash_insert_str_uint8(hash, KEY_1, 6);

    if(c_rhash_get_uint8_by_str(hash, KEY_1, &val))
        printf(" key not found\n");
    else
        printf(" value is %d\n", (int)val);

    if(c_rhash_get_uint8_by_str(hash, KEY_2, &val))
        printf(" key not found\n");
    else
        printf(" value is %d\n", (int)val);
    c_rhash_destroy(hash);
    return 0;
}