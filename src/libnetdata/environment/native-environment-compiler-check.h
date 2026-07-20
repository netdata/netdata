// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NATIVE_ENVIRONMENT_COMPILER_CHECK_H
#define NETDATA_NATIVE_ENVIRONMENT_COMPILER_CHECK_H

// System and project declarations are already visible when this header is
// included. Poisoning here also catches native API names produced by macros.
#if (defined(__GNUC__) || defined(__clang__)) && !defined(NETDATA_NATIVE_ENVIRONMENT_ACCESS)
#if defined(OS_WINDOWS)
// MSYS aliases the ANSI entry point to its unsuffixed implementation.
#undef GetEnvironmentStringsA
#endif
#pragma GCC poison getenv _wgetenv getenv_s _wgetenv_s _dupenv_s _wdupenv_s secure_getenv
#pragma GCC poison setenv unsetenv putenv _putenv _wputenv _putenv_s _wputenv_s clearenv
#pragma GCC poison environ _environ __environ _wenviron _NSGetEnviron __p__environ
#pragma GCC poison GetEnvironmentStringsA GetEnvironmentStringsW
#pragma GCC poison FreeEnvironmentStringsA FreeEnvironmentStringsW
#pragma GCC poison GetEnvironmentVariableA GetEnvironmentVariableW
#pragma GCC poison SetEnvironmentVariableA SetEnvironmentVariableW
#pragma GCC poison ExpandEnvironmentStringsA ExpandEnvironmentStringsW
#endif

#endif // NETDATA_NATIVE_ENVIRONMENT_COMPILER_CHECK_H
