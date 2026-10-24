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
// terminator one byte past host->machine_guid[]. The streaming handshake used to accept a canonical
// UUID followed by arbitrary bytes and store the original string; it now refuses such a guid (see
// the validity cases below), but rrdhost_create() has other callers, so the bound is tested alone.
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

    // Regression: the streaming handshake validated the guid with regenerate_guid(), which rejects
    // only uuid_parse_flexi()'s -1 (NULL or empty), so any non-empty string was accepted. A guid
    // longer than GUID_LEN then created a host whose stored (truncated) key never matched the
    // untruncated string reconnects look up, so the child was refused as a duplicate forever.
    struct {
        const char *guid;
        bool valid;
    } validity[] = {
        { canonical, true },
        { "00000000-0000-0000-0000-00000000000A", true },
        { compact, true },
        { NULL, false },
        { "", false },
        { "hello", false },
        { "00000000-0000-0000-0000-00000000000", false },
        { "00000000-0000-0000-0000-00000000000g", false },
        { "0000-0000-0000-0000-0000-0000-00000000", false },
        { "00000000-0000-0000-0000-0000000000000", false },
        { "00000000-0000-0000-0000-000000000000x", false },
        { "00000000-0000-0000-0000-000000000000 and then some trailing bytes", false },
        // compact spelling with trailing bytes: parses to the same binary UUID as the compact one
        { "00000000000000000000000000000000x", false },
        { "00000000000000000000000000000000xxxx", false },
        { "00000000000000000000000000000000----", false },
        // four hyphens, 36 characters, but not in the canonical positions
        { "000000-00-0000000000-0000-0000000000", false },
    };

    for (size_t i = 0; i < sizeof(validity) / sizeof(validity[0]); i++) {
        RRDHOST_TEST(
            rrdhost_machine_guid_is_valid(validity[i].guid) == validity[i].valid,
            "guid '%s' should be %s",
            validity[i].guid ? validity[i].guid : "(null)",
            validity[i].valid ? "accepted" : "rejected");
    }

    fprintf(stderr, "%s() %s\n", __FUNCTION__, errors ? "FAILED" : "passed");
    return errors;
}
