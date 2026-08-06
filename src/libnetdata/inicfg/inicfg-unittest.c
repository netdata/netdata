// SPDX-License-Identifier: GPL-3.0-or-later

#include "inicfg_internals.h"

// the UTF-8 BOM, kept as its own string literal so the hex escapes cannot
// swallow the characters that follow it when it is concatenated
#define UTF8_BOM "\xEF\xBB\xBF"

#define INICFG_UT_SECTION "unittest"

typedef struct {
    const char *description;
    const char *content;         // the file inicfg_load() is given
    const char *name;            // the option to look up in INICFG_UT_SECTION
    const char *expected;        // NULL = the option must not exist
} inicfg_test_case_t;

static const inicfg_test_case_t test_cases[] = {
    {
        .description = "a file without a BOM parses (control)",
        .content = "[" INICFG_UT_SECTION "]\nkey = value\n",
        .name = "key",
        .expected = "value",
    },
    {
        .description = "a UTF-8 BOM before the first section is ignored",
        .content = UTF8_BOM "[" INICFG_UT_SECTION "]\nkey = value\n",
        .name = "key",
        .expected = "value",
    },
    {
        .description = "a UTF-8 BOM before a first-line comment is ignored",
        .content = UTF8_BOM "# a comment\n[" INICFG_UT_SECTION "]\nkey = value\n",
        .name = "key",
        .expected = "value",
    },
    {
        .description = "a UTF-8 BOM alone on the first line is ignored",
        .content = UTF8_BOM "\n[" INICFG_UT_SECTION "]\nkey = value\n",
        .name = "key",
        .expected = "value",
    },
    {
        .description = "a BOM sequence inside a value is preserved",
        .content = "[" INICFG_UT_SECTION "]\nkey = " UTF8_BOM "value\n",
        .name = "key",
        .expected = UTF8_BOM "value",
    },
    {
        .description = "a truncated BOM prefix is not stripped",
        .content = "\xEF" "\xBB" "[" INICFG_UT_SECTION "]\nkey = value\n",
        .name = "key",
        .expected = NULL,
    },
};

// writes content to a fresh temporary file; filename receives the path created
static bool inicfg_unittest_write_file(char *filename, size_t filename_size, const char *content) {
    const char *tmp = getenv("TMPDIR");
    if(!tmp || !*tmp) tmp = "/tmp";

    snprintfz(filename, filename_size, "%s/netdata-inicfg-unittest.XXXXXX", tmp);

    int fd = mkstemp(filename);
    if(fd == -1) {
        fprintf(stderr, "  cannot create a temporary file at '%s'\n", filename);
        return false;
    }

    size_t len = strlen(content);
    ssize_t written = write(fd, content, len);
    close(fd);

    if(written != (ssize_t)len) {
        fprintf(stderr, "  cannot write %zu bytes to '%s'\n", len, filename);
        unlink(filename);
        return false;
    }

    return true;
}

static int inicfg_unittest_run_case(const inicfg_test_case_t *tc) {
    char filename[FILENAME_MAX + 1];

    if(!inicfg_unittest_write_file(filename, sizeof(filename), tc->content))
        return 1;

    struct config cfg = APPCONFIG_INITIALIZER;
    int failed = 0;

    if(!inicfg_load(&cfg, filename, 1, NULL)) {
        fprintf(stderr, "  FAILED: inicfg_load() could not read '%s'\n", filename);
        failed = 1;
    }
    else {
        int exists = inicfg_exists(&cfg, INICFG_UT_SECTION, tc->name);

        if(!tc->expected) {
            if(exists) {
                fprintf(stderr, "  FAILED: [%s].%s should not exist\n", INICFG_UT_SECTION, tc->name);
                failed = 1;
            }
        }
        else if(!exists) {
            fprintf(stderr, "  FAILED: [%s].%s is missing\n", INICFG_UT_SECTION, tc->name);
            failed = 1;
        }
        else {
            const char *value = inicfg_get(&cfg, INICFG_UT_SECTION, tc->name, "");
            if(!value || strcmp(value, tc->expected) != 0) {
                fprintf(stderr, "  FAILED: [%s].%s is '%s', expected '%s'\n",
                        INICFG_UT_SECTION, tc->name, value ? value : "(null)", tc->expected);
                failed = 1;
            }
        }
    }

    inicfg_free(&cfg);
    unlink(filename);

    return failed;
}

typedef struct {
    const char *description;
    const char *input;
    const char *expected;        // what the buffer must hold afterwards
} inicfg_bom_test_case_t;

static const inicfg_bom_test_case_t bom_test_cases[] = {
    { "a BOM is removed", UTF8_BOM "text", "text" },
    { "a string without a BOM is left alone", "text", "text" },
    { "a BOM with nothing after it empties the string", UTF8_BOM, "" },
    { "an empty string is left alone", "", "" },
    { "a truncated BOM is left alone", "\xEF" "\xBB", "\xEF" "\xBB" },
    { "a single 0xEF is left alone", "\xEF", "\xEF" },
    { "a BOM that is not at the start is left alone", "x" UTF8_BOM, "x" UTF8_BOM },
};

// remove_utf8_bom() must edit the buffer in place and return that same buffer.
// health_readfile() depends on this: it joins continuation lines with offsets
// relative to the start of its buffer, so a BOM skipped by returning a shifted
// pointer would reappear on the next line it parses.
static int inicfg_unittest_remove_utf8_bom(void) {
    int failed = 0;

    for(size_t i = 0; i < _countof(bom_test_cases); i++) {
        const inicfg_bom_test_case_t *tc = &bom_test_cases[i];
        char buffer[64];

        fprintf(stderr, "  %s\n", tc->description);
        strncpyz(buffer, tc->input, sizeof(buffer) - 1);

        char *result = remove_utf8_bom(buffer);

        if(result != buffer) {
            fprintf(stderr, "  FAILED: remove_utf8_bom() did not return its own buffer\n");
            failed++;
        }

        if(strcmp(buffer, tc->expected) != 0) {
            fprintf(stderr, "  FAILED: the buffer holds '%s', expected '%s'\n", buffer, tc->expected);
            failed++;
        }
    }

    return failed;
}

int inicfg_unittest(void) {
    fprintf(stderr, "\n%s() running...\n", __FUNCTION__);

    int failed = inicfg_unittest_remove_utf8_bom();

    for(size_t i = 0; i < _countof(test_cases); i++) {
        const inicfg_test_case_t *tc = &test_cases[i];

        fprintf(stderr, "  %s\n", tc->description);
        failed += inicfg_unittest_run_case(tc);
    }

    fprintf(stderr, "inicfg tests completed: %zu run, %d failed\n",
            _countof(bom_test_cases) + _countof(test_cases), failed);

    return failed;
}
