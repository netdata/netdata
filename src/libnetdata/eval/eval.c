// SPDX-License-Identifier: GPL-3.0-or-later

/*
 * This file has been split into several parts:
 * - eval-parser.c:  Parser implementation
 * - eval-execute.c: Execution/evaluation logic
 * - eval-utils.c:   Common utilities
 * - eval-functions.c: Dynamic function registry
 *
 * Any new changes should be made to those files instead of this one.
 * This file is now just a stub for initialization and backward compatibility.
 */

#include "../libnetdata.h"
#include "eval-internal.h"
