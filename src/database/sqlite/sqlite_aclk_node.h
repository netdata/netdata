// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SQLITE_ACLK_NODE_H
#define NETDATA_SQLITE_ACLK_NODE_H

void aclk_check_node_info_collectors_and_manifest(void);
void send_node_info_with_wait(RRDHOST *host);
void send_node_update_with_wait(RRDHOST *host, int live, int queryable);

// The coalescing window of the node manifest: how long an armed request waits before it may be
// published, so a burst of function registrations becomes one message.
#define NODE_MANIFEST_WINDOW_S (30)

// Folds the ACLK session into the manifest content hash, so one value describes everything that must
// force a re-publish. A collision would suppress a manifest the cloud never received, at the same
// ~2^-64 manifest_dict_hash() already accepts for the content itself. Here (rather than beside its
// caller) so it can be unit tested.
//
// Every value including 0 is a valid key: what marks a config as having nothing outstanding is
// node_manifest_sent_token being 0, not the key, so this needs no reserved value.
static inline uint64_t manifest_publication_key(uint64_t hash, usec_t session)
{
    return XXH3_64bits_withSeed(&session, sizeof(session), hash);
}

// How many manifests one scan pass may publish. The manifest is the only one of the three messages
// aclk_check_node_info_collectors_and_manifest() sends that can be armed for the whole fleet at a
// single instant (aclk_arm_node_manifest_all_hosts(), on every SendNodeInstances), and a new ACLK
// session never suppresses the first manifest of a host - so without a budget one reconnect makes
// every host due in the same pass, and a large parent publishes its entire fleet in one second.
// That burst has no deadline to meet: the manifest is informational and fire-and-forget.
//
// The number is a pacing choice, not a derived limit: what it protects is the mqtt write buffer on
// a slow uplink and cloud-side ingestion of simultaneous manifests from every reconnecting parent,
// neither of which is measurable from this repository. It is deliberately independent of fleet
// size; a budget scaled to the host count (N/window per pass) would keep every host reachable but
// would also make the sustained message rate grow without limit, which is what this exists to stop.
//
// Deliberately NOT applied to node info and node collectors: their pacing is pre-existing
// cloud-visible behaviour, and changing it is not this feature's decision to make. A reconnect
// bursts all three message types; this budget bounds the one the manifest introduced.
#define MAX_NODE_MANIFESTS_PER_SCAN (20)

// Decides which due hosts get the budget in a scan pass.
//
// Granting it in index order does NOT drain fairly: a host that keeps re-arming refreshes its
// deadline to near-now every time it is served, so with B slots per pass and a W second window only
// the first B*W hosts of the index are ever reachable - past that, hosts at the head keep coming
// due and keep winning, and the tail is never served at all. rrdhost_root_index is walked in
// insertion order, so that tail is the same hosts on every pass, and a pass that takes longer than
// the timer period (it is skipped while alert_push_running) lowers B*W further.
//
// So the oldest deadline wins instead of the lowest index. A host that is not served only gets
// older, so its position strictly improves every pass until it is inside the budget, whatever the
// fleet size - while the budget itself stays fixed.
//
// `cutoff` is carried from the previous pass, because the deadlines that decide it are only known
// once the walk that collects them has finished. It is the oldest deadline the previous pass left
// UNSERVED (of the MAX_NODE_MANIFESTS_PER_SCAN oldest such), which is exactly the set the next pass
// should admit; deadlines only age, so a pass of lag cannot reorder them. Zero means unrestricted -
// no due host went unserved, so there is nothing to prioritize - and is also the initial value, so
// the first pass behaves as a plain budget. An armed deadline is never itself 0, so the sentinel is
// unambiguous, and manifest_pacer_end() restores it whenever a pass leaves nothing waiting.
typedef struct manifest_pacer {
    time_t cutoff;                                 // carried across passes; 0 means unrestricted
    time_t deadlines[MAX_NODE_MANIFESTS_PER_SCAN]; // oldest deadlines this pass could not serve
    size_t deferred;                               // entries used in deadlines[]
    size_t published;                              // manifests actually published this pass
} MANIFEST_PACER;

// Starts a pass. The cutoff survives; the per-pass counters do not.
static inline void manifest_pacer_begin(MANIFEST_PACER *pacer)
{
    pacer->deferred = 0;
    pacer->published = 0;
}

// Whether a due host may publish. Callers MUST test this before claiming the send, so a host denied
// a slot keeps its armed request instead of having it consumed and dropped.
static inline bool manifest_pacer_admit(const MANIFEST_PACER *pacer, time_t deadline)
{
    return pacer->published < MAX_NODE_MANIFESTS_PER_SCAN && (!pacer->cutoff || deadline <= pacer->cutoff);
}

// Charges the budget. Called only when a message was really enqueued, so a run of suppressed
// (unchanged) manifests cannot exhaust the budget for the hosts behind them.
static inline void manifest_pacer_published(MANIFEST_PACER *pacer)
{
    pacer->published++;
}

// Records a due host that was not served, keeping the MAX_NODE_MANIFESTS_PER_SCAN oldest deadlines
// sorted oldest first. A host that was admitted and claimed counts as served even if its manifest
// turned out to be unchanged, so it is not recorded here.
static inline void manifest_pacer_defer(MANIFEST_PACER *pacer, time_t deadline)
{
    size_t i = pacer->deferred;

    if (i == MAX_NODE_MANIFESTS_PER_SCAN) {
        if (deadline >= pacer->deadlines[MAX_NODE_MANIFESTS_PER_SCAN - 1])
            return; // newer than everything tracked - not among the oldest
        i = MAX_NODE_MANIFESTS_PER_SCAN - 1; // evict the newest of the tracked set
    }
    else
        pacer->deferred++;

    while (i > 0 && pacer->deadlines[i - 1] > deadline) {
        pacer->deadlines[i] = pacer->deadlines[i - 1];
        i--;
    }

    pacer->deadlines[i] = deadline;
}

// Ends a pass: hand the next one the oldest deadline this one could not serve, so those hosts are
// admitted ahead of any host that re-arms in the meantime. Nothing left waiting means no
// restriction, which is also what keeps a stale cutoff from outliving the backlog that produced it.
static inline void manifest_pacer_end(MANIFEST_PACER *pacer)
{
    pacer->cutoff = pacer->deferred ? pacer->deadlines[pacer->deferred - 1] : 0;
}

int rrdfunctions_manifest_pacer_unittest(void);

#endif //NETDATA_SQLITE_ACLK_NODE_H
