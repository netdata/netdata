// SPDX-License-Identifier: GPL-3.0-or-later

#include "safe_fork.h"

void string_safe_fork_before(void);
void string_safe_fork_after(void);

pid_t safe_fork(void) {
    string_safe_fork_before();

    pid_t pid = fork();

    string_safe_fork_after();

    return pid;
}
