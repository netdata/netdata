// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

ND_UUID UUID_generate_from_hash(const void *payload, size_t payload_len) {
    assert(sizeof(XXH128_hash_t) == sizeof(ND_UUID));

    ND_UUID uuid = UUID_ZERO;
    XXH128_hash_t *xxh3_128 = (XXH128_hash_t *)&uuid;

    // Hash the payload using XXH128
    // Assume xxh128_hash_function is your function to generate XXH128 hash
    *xxh3_128 = XXH3_128bits(payload, payload_len);

    // Set the UUID version (here, setting it to 4)
    uuid.uuid[6] = (uuid.uuid[6] & 0x0F) | 0x40; // Version 4

    // Set the UUID variant (standard variant for UUID)
    uuid.uuid[8] = (uuid.uuid[8] & 0x3F) | 0x80; // Variant is 10xxxxxx

    return uuid;
}

void uuid_unparse_lower_compact(const nd_uuid_t uuid, char *out) {
    static const char *hex_chars = "0123456789abcdef";
    for (int i = 0; i < 16; i++) {
        out[i * 2] = hex_chars[(uuid[i] >> 4) & 0x0F];
        out[i * 2 + 1] = hex_chars[uuid[i] & 0x0F];
    }
    out[32] = '\0'; // Null-terminate the string
}

static inline void nd_uuid_unparse_full(const nd_uuid_t uuid, char *out, const char *hex_chars) {
    int shifts = 0;
    for (int i = 0; i < 16; i++) {
        if (i == 4 || i == 6 || i == 8 || i == 10) {
            out[i * 2 + shifts] = '-';
            shifts++;
        }
        out[i * 2 + shifts] = hex_chars[(uuid[i] >> 4) & 0x0F];
        out[i * 2 + 1 + shifts] = hex_chars[uuid[i] & 0x0F];
    }
    out[36] = '\0'; // Null-terminate the string
}

// Wrapper functions for lower and upper case hexadecimal representation
void nd_uuid_unparse_lower(const nd_uuid_t uuid, char *out) {
    nd_uuid_unparse_full(uuid, out, "0123456789abcdef");
}

void nd_uuid_unparse_upper(const nd_uuid_t uuid, char *out) {
    nd_uuid_unparse_full(uuid, out, "0123456789ABCDEF");
}

inline int uuid_parse_compact(const char *in, nd_uuid_t uuid) {
    if (strlen(in) != 32)
        return -1; // Invalid input length

    for (int i = 0; i < 16; i++) {
        int high = hex_char_to_int(in[i * 2]);
        int low = hex_char_to_int(in[i * 2 + 1]);

        if (high < 0 || low < 0)
            return -1; // Invalid hexadecimal character

        uuid[i] = (high << 4) | low;
    }

    return 0; // Success
}

int uuid_parse_flexi(const char *in, nd_uuid_t uu) {
    if(!in || !*in)
        return -1;

    size_t hexCharCount = 0;
    size_t hyphenCount = 0;
    const char *s = in;
    int byteIndex = 0;
    nd_uuid_t uuid; // work on a temporary place, to not corrupt the previous value of uu if we fail

    while (*s && byteIndex < 16) {
        if (*s == '-') {
            s++;
            hyphenCount++;

            if (unlikely(hyphenCount > 4))
                // Too many hyphens
                return -2;
        }

        if (likely(isxdigit((uint8_t)*s))) {
            int high = hex_char_to_int(*s++);
            hexCharCount++;

            if (likely(isxdigit((uint8_t)*s))) {
                int low = hex_char_to_int(*s++);
                hexCharCount++;

                uuid[byteIndex++] = (high << 4) | low;
            }
            else
                // Not a valid UUID (expected a pair of hex digits)
                return -3;
        }
        else
            // Not a valid UUID
            return -4;
    }

    if (unlikely(byteIndex < 16))
        // Not enough data to form a UUID
        return -5;

    if (unlikely(hexCharCount != 32))
        // wrong number of hex digits
        return -6;

    if(unlikely(hyphenCount != 0 && hyphenCount != 4))
        // wrong number of hyphens
        return -7;

    // copy the final value
    memcpy(uu, uuid, sizeof(nd_uuid_t));

    return 0;
}


// ----------------------------------------------------------------------------
// unit test

static inline void remove_hyphens(const char *uuid_with_hyphens, char *uuid_without_hyphens) {
    while (*uuid_with_hyphens) {
        if (*uuid_with_hyphens != '-') {
            *uuid_without_hyphens++ = *uuid_with_hyphens;
        }
        uuid_with_hyphens++;
    }
    *uuid_without_hyphens = '\0';
}

int uuid_unittest(void) {
    const int num_tests = 100000;
    int failed_tests = 0;

    int i;
    for (i = 0; i < num_tests; i++) {
        nd_uuid_t original_uuid, parsed_uuid;
        char uuid_str_with_hyphens[UUID_STR_LEN], uuid_str_without_hyphens[UUID_COMPACT_STR_LEN];

        // Generate a random UUID
        switch(i % 2) {
            case 0:
                uuid_generate(original_uuid);
                break;

            case 1:
                uuid_generate_random(original_uuid);
                break;
        }

        // Unparse it with hyphens
        bool lower = false;
        switch(i % 3) {
            case 0:
                uuid_unparse_lower(original_uuid, uuid_str_with_hyphens);
                lower = true;
                break;

            case 1:
                uuid_unparse(original_uuid, uuid_str_with_hyphens);
                break;

            case 2:
                uuid_unparse_upper(original_uuid, uuid_str_with_hyphens);
                break;
        }

        // Remove the hyphens
        remove_hyphens(uuid_str_with_hyphens, uuid_str_without_hyphens);

        if(lower) {
            char test[UUID_COMPACT_STR_LEN];
            uuid_unparse_lower_compact(original_uuid, test);
            if(strcmp(test, uuid_str_without_hyphens) != 0) {
                printf("uuid_unparse_lower_compact() failed, expected '%s', got '%s'\n",
                       uuid_str_without_hyphens, test);
                failed_tests++;
            }
        }

        // Parse the UUID string with hyphens
        int parse_result = uuid_parse_flexi(uuid_str_with_hyphens, parsed_uuid);
        if (parse_result != 0) {
            printf("uuid_parse_flexi() returned -1 (parsing error) for UUID with hyphens: %s\n", uuid_str_with_hyphens);
            failed_tests++;
        } else if (uuid_compare(original_uuid, parsed_uuid) != 0) {
            printf("uuid_parse_flexi() parsed value mismatch for UUID with hyphens: %s\n", uuid_str_with_hyphens);
            failed_tests++;
        }

        // Parse the UUID string without hyphens
        parse_result = uuid_parse_flexi(uuid_str_without_hyphens, parsed_uuid);
        if (parse_result != 0) {
            printf("uuid_parse_flexi() returned -1 (parsing error) for UUID without hyphens: %s\n", uuid_str_without_hyphens);
            failed_tests++;
        }
        else if(uuid_compare(original_uuid, parsed_uuid) != 0) {
            printf("uuid_parse_flexi() parsed value mismatch for UUID without hyphens: %s\n", uuid_str_without_hyphens);
            failed_tests++;
        }

        if(failed_tests)
            break;
    }

    printf("UUID: failed %d out of %d tests.\n", failed_tests, i);
    return failed_tests;
}
