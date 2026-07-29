// SPDX-License-Identifier: GPL-3.0-or-later

#include "node_manifest.h"

#include "nodeinstance/manifest/v1/manifest.pb.h"

#include "schema_wrapper_utils.h"

// NOTE: the protobuf HttpAccess enum value names (HTTP_ACCESS_SAME_SPACE, ...)
// collide with the C HTTP_ACCESS enum constants from libnetdata. We therefore do
// NOT pull the proto namespace in with `using namespace`; the proto symbols are
// always fully qualified, while the unqualified HTTP_ACCESS_* names in the table
// below refer to the C enum.
namespace manifestpb = nodeinstance::manifest::v1;

// Every HTTP_ACCESS flag is a single bit, and every bit has a 1:1 proto
// counterpart. Keep this table in sync with HTTP_ACCESS in
// libnetdata/user-auth/http-access.h - a bit missing here is silently dropped
// from the manifest, which makes the cloud deny a permission the agent grants.
// The assertions and the switch tripwire below all fail the build if that happens.
static const struct {
    HTTP_ACCESS bit;
    manifestpb::HttpAccess proto;
} http_access_map[] = {
    { HTTP_ACCESS_SIGNED_ID,                 manifestpb::HTTP_ACCESS_SIGNED_IN                  },
    { HTTP_ACCESS_SAME_SPACE,                manifestpb::HTTP_ACCESS_SAME_SPACE                 },
    { HTTP_ACCESS_COMMERCIAL_SPACE,          manifestpb::HTTP_ACCESS_COMMERCIAL                 },
    { HTTP_ACCESS_ANONYMOUS_DATA,            manifestpb::HTTP_ACCESS_ANONYMOUS_DATA             },
    { HTTP_ACCESS_SENSITIVE_DATA,            manifestpb::HTTP_ACCESS_SENSITIVE_DATA             },
    { HTTP_ACCESS_VIEW_AGENT_CONFIG,         manifestpb::HTTP_ACCESS_VIEW_CONFIG                },
    { HTTP_ACCESS_EDIT_AGENT_CONFIG,         manifestpb::HTTP_ACCESS_EDIT_CONFIG                },
    { HTTP_ACCESS_VIEW_NOTIFICATIONS_CONFIG, manifestpb::HTTP_ACCESS_VIEW_NOTIFICATIONS_CONFIG  },
    { HTTP_ACCESS_EDIT_NOTIFICATIONS_CONFIG, manifestpb::HTTP_ACCESS_EDIT_NOTIFICATIONS_CONFIG  },
    { HTTP_ACCESS_VIEW_ALERTS_SILENCING,     manifestpb::HTTP_ACCESS_VIEW_ALERTS_SILENCING      },
    { HTTP_ACCESS_EDIT_ALERTS_SILENCING,     manifestpb::HTTP_ACCESS_EDIT_ALERTS_SILENCING      },
};

#define HTTP_ACCESS_MAP_ENTRIES (sizeof(http_access_map) / sizeof(http_access_map[0]))

// HTTP_ACCESS_ALL is the OR of every defined bit, so its population count is the bit count.
static_assert(
    HTTP_ACCESS_MAP_ENTRIES == (size_t)__builtin_popcount((unsigned)HTTP_ACCESS_ALL),
    "http_access_map is out of sync with HTTP_ACCESS_ALL");

// Requiring the bits to be contiguous from bit 0 pins the shape of HTTP_ACCESS_ALL, so a bit
// cannot be dropped from it without also tripping the count above.
static_assert(
    (unsigned)HTTP_ACCESS_ALL == (1ULL << HTTP_ACCESS_MAP_ENTRIES) - 1ULL,
    "HTTP_ACCESS bits are no longer contiguous from bit 0 - update http_access_map");

// Both assertions above are functions of HTTP_ACCESS_ALL alone, which is itself hand-maintained:
// a bit added to the HTTP_ACCESS enum but forgotten in HTTP_ACCESS_ALL is invisible to them. This
// switch is the tripwire that reads the enum directly - with no default:, an unhandled enumerator
// is named by -Wswitch, promoted to an error here so it fails the build rather than scrolling past
// (this file is not built with -Werror). Never called; it exists only to be compiled.
#pragma GCC diagnostic push
#pragma GCC diagnostic error "-Wswitch"
[[maybe_unused]] static void http_access_map_completeness_tripwire(HTTP_ACCESS bit)
{
    switch (bit) {
        case HTTP_ACCESS_NONE:
        case HTTP_ACCESS_SIGNED_ID:
        case HTTP_ACCESS_SAME_SPACE:
        case HTTP_ACCESS_COMMERCIAL_SPACE:
        case HTTP_ACCESS_ANONYMOUS_DATA:
        case HTTP_ACCESS_SENSITIVE_DATA:
        case HTTP_ACCESS_VIEW_AGENT_CONFIG:
        case HTTP_ACCESS_EDIT_AGENT_CONFIG:
        case HTTP_ACCESS_VIEW_NOTIFICATIONS_CONFIG:
        case HTTP_ACCESS_EDIT_NOTIFICATIONS_CONFIG:
        case HTTP_ACCESS_VIEW_ALERTS_SILENCING:
        case HTTP_ACCESS_EDIT_ALERTS_SILENCING:
            break;
    }
}
#pragma GCC diagnostic pop

static void set_function_access(manifestpb::Function *f, HTTP_ACCESS access)
{
    for (size_t i = 0; i < HTTP_ACCESS_MAP_ENTRIES; i++) {
        if (access & http_access_map[i].bit)
            f->add_access(http_access_map[i].proto);
    }
}

// host function tags are stored as a single whitespace-separated string; the
// proto carries them as a repeated field, so split on whitespace.
static void set_function_tags(manifestpb::Function *f, const char *tags)
{
    const char *p = tags;
    while (*p) {
        while (*p == ' ' || *p == '\t')
            p++;
        const char *start = p;
        while (*p && *p != ' ' && *p != '\t')
            p++;
        if (p > start)
            f->add_tags(std::string(start, (size_t)(p - start)));
    }
}

char *generate_update_node_instance_manifest_message(size_t *len, struct update_node_instance_manifest *manifest)
{
    manifestpb::UpdateNodeInstanceManifest msg;

    if (manifest->node_id)
        msg.set_node_id(manifest->node_id);
    if (manifest->claim_id)
        msg.set_claim_id(manifest->claim_id);

    set_google_timestamp_from_timeval(manifest->updated_at, msg.mutable_updated_at());

    manifestpb::Functions *functions = msg.mutable_functions();

    if (manifest->functions) {
        void *fn_value;
        dfe_start_read(manifest->functions, fn_value) {
            struct rrd_function_manifest_entry *fn = (struct rrd_function_manifest_entry *)fn_value;

            manifestpb::Function *f = functions->add_items();
            f->set_name(fn_value_dfe.name);
            if (fn->help && *fn->help)
                f->set_help(fn->help);
            set_function_tags(f, fn->tags);
            // the proto field is unsigned; a negative priority would wrap, so fall back to the
            // default the way pluginsd already does for its own input
            f->set_priority(fn->priority > 0 ? (uint32_t)fn->priority : RRDFUNCTIONS_PRIORITY_DEFAULT);
            f->set_version(fn->version);
            set_function_access(f, fn->access);
        }
        dfe_done(fn_value);
    }

    *len = PROTO_COMPAT_MSG_SIZE(msg);
    char *bin = (char*)mallocz(*len);
    msg.SerializeToArray(bin, *len);

    return bin;
}
