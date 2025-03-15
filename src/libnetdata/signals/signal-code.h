// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SIGNAL_CODE_H
#define NETDATA_SIGNAL_CODE_H

#include "libnetdata/common.h"
#include "../template-enum.h"

// SIGNAL_CODE combines signal number and signal code into a single 64-bit identifier
typedef uint64_t SIGNAL_CODE;

// Create a SIGNAL_CODE from signal number and signal code
SIGNAL_CODE signal_code(int signo, int si_code);

// Extract the signal number from a SIGNAL_CODE
#define SIGNAL_CODE_GET_SIGNO(code) ((int)((code) >> 32))

// Extract the signal code from a SIGNAL_CODE
#define SIGNAL_CODE_GET_SI_CODE(code) ((int)((code) & 0xFFFFFFFF))

// convert a signal code to string, by name or hex (if no name is available)
void SIGNAL_CODE_2str_h(SIGNAL_CODE code, char *buf, size_t size);

// parse a signal code from a string, by name or hex
SIGNAL_CODE SIGNAL_CODE_2id_h(const char *str);

#endif /* NETDATA_SIGNAL_CODE_H */