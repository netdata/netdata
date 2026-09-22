// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

#define RRDHOST_TEST(condition, ...)                                                                                   \
    do {                                                                                                               \
        if (!(condition)) {                                                                                            \
            fprintf(stderr, "  FAILED: ");                                                                             \
            fprintf(stderr, __VA_ARGS__);                                                                              \
            fprintf(stderr, "\n");                                                                                     \
            errors++;                                                                                                  \
        }                                                                                                              \
    } while (0)

// Regression: rrdhost_create() copied the machine-guid with a bound of GUID_LEN + 1, but strncpyz()
// takes the destination size MINUS ONE, so a guid of GUID_LEN + 1 characters or more placed the
// terminator one byte past host->machine_guid[]. The guid is attacker-influenced: the streaming
// handshake accepts a canonical UUID followed by arbitrary bytes and stores the original string.
//
// The destination here is wrapped in a struct with a canary immediately after it, so an over-long
// copy fails deterministically rather than only under ASan - the byte after machine_guid[] inside
// RRDHOST is alignment padding, which no sanitizer flags.
int rrdhost_machine_guid_unittest(void)
{
    fprintf(stderr, "\n%s() running...\n", __FUNCTION__);

    int errors = 0;

    struct {
        char machine_guid[GUID_LEN + 1];
        char canary;
    } target;

    static const char canonical[] = "00000000-0000-0000-0000-000000000000";
    static const char compact[] = "00000000000000000000000000000000";

    struct {
        const char *guid;
        const char *expected;
    } cases[] = {
        // shorter than GUID_LEN: kept whole (uuid_parse_flexi() accepts the compact spelling, so
        // this is a guid the daemon can really be given)
        { "", "" },
        { compact, compact },
        // exactly GUID_LEN: kept whole
        { canonical, canonical },
        // longer than GUID_LEN: truncated, never written past the array
        { "00000000-0000-0000-0000-0000000000000", canonical },
        { "00000000-0000-0000-0000-000000000000 and then some trailing bytes", canonical },
    };

    for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        memset(&target, 0, sizeof(target));
        target.canary = 'C';

        rrdhost_machine_guid_copy(target.machine_guid, cases[i].guid);

        RRDHOST_TEST(
            strcmp(target.machine_guid, cases[i].expected) == 0,
            "guid '%s' was stored as '%s', expected '%s'",
            cases[i].guid,
            target.machine_guid,
            cases[i].expected);

        RRDHOST_TEST(
            target.canary == 'C',
            "guid '%s' overwrote the byte after machine_guid[]",
            cases[i].guid);
    }

    fprintf(stderr, "%s() %s\n", __FUNCTION__, errors ? "FAILED" : "passed");
    return errors;
}
