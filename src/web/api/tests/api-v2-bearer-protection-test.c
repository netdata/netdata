// SPDX-License-Identifier: GPL-3.0-or-later

// Include the implementation to exercise api_v2_bearer_protection() without a running agent.
#include "../v2/api_v2_bearer.c"

#define TEST_MACHINE_GUID "11111111-2222-3333-4444-555555555555"
#define TEST_NODE_ID "66666666-7777-8888-9999-aaaaaaaaaaaa"
#define TEST_IDENTITY "claim_id=c&machine_guid=" TEST_MACHINE_GUID "&node_id=" TEST_NODE_ID

bool netdata_is_protected_by_bearer = false;
RRDHOST *localhost = NULL;

static bool test_claim_matches = true;
static bool test_flip_state_during_call = false;

bool claim_id_matches(const char *claim_id __maybe_unused) {
    // runs after the handler parsed the query: simulates another request toggling protection meanwhile
    if(test_flip_state_during_call)
        netdata_bearer_protection_set_enabled(!netdata_bearer_protection_is_enabled());

    return test_claim_matches;
}

bool claim_id_matches_any(const char *claim_id __maybe_unused) {
    return test_claim_matches;
}

struct test_case {
    const char *name;
    const char *query;
    bool initial;
    bool claim_matches;
    bool flip_state_during_call;
    int expected_code;
    bool expected_state;
    const char *expected_text; // substring of the response, or NULL
};

static int run_case(struct web_client *w, const struct test_case *tc) {
    char query[1024];
    strncpyz(query, tc->query, sizeof(query) - 1);

    netdata_bearer_protection_set_enabled(tc->initial);
    test_claim_matches = tc->claim_matches;
    test_flip_state_during_call = tc->flip_state_during_call;
    buffer_flush(w->response.data);

    int code = api_v2_bearer_protection(localhost, w, query);
    bool state = netdata_bearer_protection_is_enabled();
    const char *text = buffer_tostring(w->response.data);

    bool ok = code == tc->expected_code && state == tc->expected_state &&
              (!tc->expected_text || strstr(text, tc->expected_text));

    fprintf(stderr, "%s: %s (code %d, expected %d; state %s, expected %s; response '%s')\n",
            ok ? "PASS" : "FAIL", tc->name, code, tc->expected_code,
            state ? "on" : "off", tc->expected_state ? "on" : "off", text);

    return ok ? 0 : 1;
}

int main(void) {
    localhost = callocz(1, sizeof(RRDHOST));
    strncpyz(localhost->machine_guid, TEST_MACHINE_GUID, sizeof(localhost->machine_guid) - 1);
    if(uuid_parse(TEST_NODE_ID, localhost->node_id.uuid) != 0) {
        fprintf(stderr, "cannot parse the test node id\n");
        return 1;
    }

    struct web_client w = { 0 };
    w.response.data = buffer_create(1024, NULL);

    const struct test_case cases[] = {
        // valid values, any case
        { "on enables", "bearer_protection=on&" TEST_IDENTITY, false, true, false, HTTP_RESP_OK, true, "\"bearer_protection\":true" },
        { "ON enables", "bearer_protection=ON&" TEST_IDENTITY, false, true, false, HTTP_RESP_OK, true, NULL },
        { "True enables", "bearer_protection=True&" TEST_IDENTITY, false, true, false, HTTP_RESP_OK, true, NULL },
        { "yes enables", "bearer_protection=yes&" TEST_IDENTITY, false, true, false, HTTP_RESP_OK, true, NULL },
        { "off disables", "bearer_protection=off&" TEST_IDENTITY, true, true, false, HTTP_RESP_OK, false, "\"bearer_protection\":false" },
        { "NO disables", "bearer_protection=NO&" TEST_IDENTITY, true, true, false, HTTP_RESP_OK, false, NULL },
        { "false disables", "bearer_protection=false&" TEST_IDENTITY, true, true, false, HTTP_RESP_OK, false, NULL },

        // invalid values fail closed: 400, state unchanged
        { "1 is rejected", "bearer_protection=1&" TEST_IDENTITY, true, true, false, HTTP_RESP_BAD_REQUEST, true, "Invalid bearer_protection value" },
        { "enable is rejected", "bearer_protection=enable&" TEST_IDENTITY, true, true, false, HTTP_RESP_BAD_REQUEST, true, NULL },
        { "onn is rejected", "bearer_protection=onn&" TEST_IDENTITY, true, true, false, HTTP_RESP_BAD_REQUEST, true, NULL },
        { "invalid then valid is rejected", "bearer_protection=bad&bearer_protection=on&" TEST_IDENTITY, false, true, false, HTTP_RESP_BAD_REQUEST, false, NULL },
        { "valid then invalid is rejected", "bearer_protection=off&bearer_protection=bad&" TEST_IDENTITY, true, true, false, HTTP_RESP_BAD_REQUEST, true, NULL },

        // valid repeats: the last one wins
        { "on then off disables", "bearer_protection=on&bearer_protection=off&" TEST_IDENTITY, true, true, false, HTTP_RESP_OK, false, NULL },
        { "off then on enables", "bearer_protection=off&bearer_protection=on&" TEST_IDENTITY, false, true, false, HTTP_RESP_OK, true, NULL },

        // absent or empty value: read-only
        { "absent keeps on", TEST_IDENTITY, true, true, false, HTTP_RESP_OK, true, "\"bearer_protection\":true" },
        { "absent keeps off", TEST_IDENTITY, false, true, false, HTTP_RESP_OK, false, NULL },
        { "empty keeps on", "bearer_protection=&" TEST_IDENTITY, true, true, false, HTTP_RESP_OK, true, NULL },
        { "absent does not undo a concurrent enable", TEST_IDENTITY, false, true, true, HTTP_RESP_OK, true, NULL },
        { "absent does not undo a concurrent disable", TEST_IDENTITY, true, true, true, HTTP_RESP_OK, false, NULL },

        // identity checks run first, with their own messages
        { "wrong claim id", "bearer_protection=off&" TEST_IDENTITY, true, false, false, HTTP_RESP_BAD_REQUEST, true, "different claimed agent" },
        { "wrong node id", "bearer_protection=off&claim_id=c&machine_guid=" TEST_MACHINE_GUID "&node_id=" TEST_MACHINE_GUID, true, true, false, HTTP_RESP_BAD_REQUEST, true, "not matching local UUIDs" },
        { "wrong claim id and invalid value", "bearer_protection=bad&" TEST_IDENTITY, true, false, false, HTTP_RESP_BAD_REQUEST, true, "different claimed agent" },
        { "wrong node id and invalid value", "bearer_protection=bad&claim_id=c&machine_guid=" TEST_MACHINE_GUID "&node_id=" TEST_MACHINE_GUID, true, true, false, HTTP_RESP_BAD_REQUEST, true, "not matching local UUIDs" },
    };

    int errors = 0;
    for(size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++)
        errors += run_case(&w, &cases[i]);

    buffer_free(w.response.data);
    freez(localhost);

    if(errors) {
        fprintf(stderr, "bearer_protection tests failed: %d of %zu case(s)\n", errors, sizeof(cases) / sizeof(cases[0]));
        return 1;
    }

    fprintf(stderr, "bearer_protection tests passed: %zu case(s)\n", sizeof(cases) / sizeof(cases[0]));
    return 0;
}
