// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ENVIRONMENT_H
#define NETDATA_ENVIRONMENT_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct nd_environment ND_ENVIRONMENT;
typedef struct nd_env_snapshot ND_ENV_SNAPSHOT;

// Import native storage during single-threaded initialization. Freeze native mirroring before
// starting application/library workers; managed updates and future child snapshots remain mutable.
// Lifecycle int functions return 0 on success and -1 with errno on failure.
int nd_environment_init(void);
int nd_environment_freeze_process(void);
bool nd_environment_is_initialized(void);
bool nd_environment_is_process_frozen(void);
ND_ENVIRONMENT *nd_environment_process(void);

// Process-global managed environment. Returned values are owned by the caller and freed with freez().
int nd_environment_set(const char *name, const char *value, bool overwrite);
int nd_environment_unset(const char *name);
char *nd_environment_get_dup(const char *name);

// Managed-only contexts for restricted child environments. The owner must serialize destruction
// against context operations; already-acquired snapshots remain valid independently.
ND_ENVIRONMENT *nd_environment_create_isolated(const char *const envp[]);
void nd_environment_destroy(ND_ENVIRONMENT *environment);
int nd_environment_context_set(ND_ENVIRONMENT *environment, const char *name, const char *value, bool overwrite);
int nd_environment_context_unset(ND_ENVIRONMENT *environment, const char *name);
char *nd_environment_context_get_dup(ND_ENVIRONMENT *environment, const char *name);

// Immutable spawn generations. Accessors remain valid until the acquired snapshot is released.
ND_ENV_SNAPSHOT *nd_environment_snapshot_acquire(ND_ENVIRONMENT *environment);
void nd_environment_snapshot_release(ND_ENV_SNAPSHOT *snapshot);
const char *const *nd_environment_snapshot_envp(const ND_ENV_SNAPSHOT *snapshot);
size_t nd_environment_snapshot_entries(const ND_ENV_SNAPSHOT *snapshot);
uint64_t nd_environment_snapshot_generation(const ND_ENV_SNAPSHOT *snapshot);
const char *nd_environment_snapshot_windows_block(const ND_ENV_SNAPSHOT *snapshot, size_t *size);

// Child-only operations used immediately after a safe single-threaded fork().
int nd_environment_fork_child_reset_from_native(void);
int nd_environment_fork_child_replace(const char *const envp[]);

#ifdef NETDATA_INTERNAL_CHECKS
// Focused fault injection and platform-neutral Windows-block validation hooks.
void nd_environment_test_fail_native_once(void);
void nd_environment_test_fail_snapshot_once(void);
char *nd_environment_test_windows_block(const char *const envp[], size_t *size);
#endif

#ifdef __cplusplus
}
#endif

#endif // NETDATA_ENVIRONMENT_H
