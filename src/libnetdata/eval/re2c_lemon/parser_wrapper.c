/**
 * Integration wrapper for using the re2c/lemon parser with Netdata
 */

#include "../eval-internal.h"
#include "parser_internal.h"

// This file exists to allow the integration of the re2c/lemon parser
// with Netdata's build system. The actual implementation is in lexer.c
// which is generated from lexer.re.

// We're just re-exporting the interface here, so the functions can be
// properly found by the linker.

// See lexer.c and parser.c for the real implementations.