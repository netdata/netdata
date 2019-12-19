# The build system

We are currently migrating from `autotools` to `CMake` as a build-system. This document 
currently describes how we intend to perform this migration, and will be updated after
the migration to explain how the new `CMake` configuration works.

## Stages during the build

1. The `netdata-installer.sh`, take in arguments and environment settings to control the
   build.
2. The configure step: `autoreconf -ivf ; ./configure` passing arguments into the configure
   script. This becomes `generation-time` in CMake. This includes package / system detection
   and configuration resulting in the `config.h` in the source root.
3. The build step: recurse through the generated Makefiles and build the executable.
4. The first install step: calls `make install` to handle all the install steps put into
   the Makefiles by the configure step (puts binaries / libraries / config into target
   tree structure).
5. The second install step: the rest of the installer after the make install handles
   system-level configuration (privilege setting, user / groups,  fetch/build/install `go.d`
   plugins, telemetry, installing service for startup, uninstaller, auto-updates.

The ideal migration result is to replace all of this with the following steps:
```
mkdir build ; cd build ; cmake .. -D... ; cmake --build . --target install
```

The `-D...` indicates where the command-line arguments for configuration are passed into
`CMake`.

## CMake generation time

At generation time we need to solve the following issues:

### Feature flags

Every command-line switch on the installer and the configure script needs to becomes an
argument to the CMake generation, we can do this with variables in the CMake cache:

CMakeLists.txt:
```
option(ENABLE_DBENGINE "Enable the dbengine storage" ON)
...
if(${ENABLE_DBENGINE})
...
endif()
```

Command-line interface
```
cmake -DENABLE_DBENGINE
```

### Dependency detection

We have a mixture of soft- and hard-depedencies on libraries. For most of these we expect
`pkg-config` information, for some we manually probe for libraries and include files. We
should treat all of the external dependencies consistently:

1. Default to autodetect using `pkg-config` (e.g. the standard `jemalloc` drops a `.pc`
   into the system but we do not check for it.
2. If no `.pc` is found perform a manual search for libraries under known names, and
   check for accessible symbols inside them.
3. Check that include paths work.
4. Allow a command-line override (e.g. `-DWITH_JEMALLOC=/...`).
5. If none of the above work then fail the install if the dependency is hard, otherwise
   indicate it is not present in the `config.h`.

Before doing any dependency detection we need to determine which search paths are 
really in use for the current compiler, after the `project` declaration we can use:
```
execute_process(COMMAND ${CMAKE_C_COMPILER} "--print-search-dirs"
                COMMAND grep "^libraries:"
                COMMAND sed "s/^libraries: =//"
                COMMAND tr ":" " "
                COMMAND tr -d "\n"
                OUTPUT_VARIABLE CC_SEARCH_DIRS
                RESULTS_VARIABLE CC_SEARCH_RES)
string(REGEX MATCH   "^[0-9]+" CC_SEARCH_RES ${CC_SEARCH_RES})
#string(STRIP "${CC_SEARCH_RES}" CC_SEARCH_RES)
if(0 LESS ${CC_SEARCH_RES})
    message(STATUS "Warning - cannot determine standard compiler library paths")
    # Note: we will probably need a different method for Windows...
endif()

```

The output format for this switch works on both `Clang` and `gcc`, it also includes
the include search path, which can be extracted in a similar way. Standard advice here
is to list the `ldconfig` cache or use the `-V` flag to check, but this does not work
consistently across platforms - in particular `gcc` will reconfigure `ld` when it is
called to gcc's internal view of search paths. During experiments each of these 
alternative missed / added unused paths. Dumping the compiler's own estimate of the
search paths seems to work consistently across clang/gcc/linux/freebsd configurations.

The default behaviour in CMake is to search across predefined paths (e.g. `CMAKE_LIBRARY_PATH`)
that are based on heuristics about the current platform. Most projects using CMake seem
to overwrite this with their own estimates.

We can use the extracted paths as a base, add our own heuristics based on OS and then
`set(CMAKE_LIBRARY_PATH ${OUR_OWN_LIB_SEARCH})` to get the best results. Roughly we do
the following for each external dependency:
```
set(WITH_JSONC "Detect" CACHE STRING "Manually set the path to a json-c installation")
...
if(${WITH_JSONC} STREQUAL "Detect")
    pkg_check_modules(JSONC json-c)     # Don't set the REQUIRED flag
    if(JSONC_FOUND)
        message(STATUS "libjsonc found through .pc -> ${JSONC_CFLAGS_OTHER} ${JSONC_LIBRARIES}")
        # ... setup using JSONC_CFLAGS_OTHER JSONC_LIBRARIES and JSONC_INCLUDE_DIRS
    else()
        find_library(LIB_JSONC
                     NAMES json-c libjson-c
                     PATHS ${CMAKE_LIBRARY_PATH})       # Includes our additions by this point
        if(${LIB_JSONC} STREQUAL "LIB_JSONC-NOTFOUND")
            message(STATUS "Library json-c not installed, disabling")
        else()
            check_library_exists(${LIB_JSONC} json_object_get_type "" HAVE_JSONC)
            # ... setup using heuristics for CFLAGS and check include files are available
        endif()
    endif()
else()
    # ... use explicit path as base to check for library and includes ...
endif()

```

For checking the include path we have two options, if we overwrite the `CMAKE_`... variables
to change the internal search path we can use:
```
CHECK_INCLUDE_FILE(json/json.h HAVE_JSONC_H)
```
Or we can build a custom search path and then use:
```
find_file(HAVE_JSONC_H json/json.h PATHS ${OUR_INCLUDE_PATHS})
```

Note: we may have cases where there is no `.pc` but we have access to a `.cmake` (e.g. AWS SDK, mongodb,cmocka) - these need to be checked / pulled inside the repo while building a prototype.

### Compiler compatability checks

In CMakeLists.txt:

```
CHECK_INCLUDE_FILE(sys/prctl.h HAVE_PRCTL_H)
configure_file(cmake/config.in config.h)
```

In cmake/config.in:

```
#cmakedefine HAVE_PRCTL_H 1
```

If we want to check explicitly if something compiles (e.g. the accept4 check, or the 
`strerror_r` typing issue) then we set the `CMAKE_`... paths and then use:
```
check_c_source_compiles(
    "
    #include <string.h>
    int main() { char x = *strerror_r(0, &x, sizeof(x)); return 0; }
    "
    STRERROR_R_CHAR_P)

```
This produces a bool that we can use inside CMake or propagate into the `config.h`.

We can handle the atomic checks with:
```
check_c_source_compiles(
    "
    int main (int argc, char **argv)
    {
      volatile unsigned long ul1 = 1, ul2 = 0, ul3 = 2;
      __atomic_load_n(&ul1, __ATOMIC_SEQ_CST);
      __atomic_compare_exchange(&ul1, &ul2, &ul3, 1, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST);
      __atomic_fetch_add(&ul1, 1, __ATOMIC_SEQ_CST);
      __atomic_fetch_sub(&ul3, 1, __ATOMIC_SEQ_CST);
      __atomic_or_fetch(&ul1, ul2, __ATOMIC_SEQ_CST);
      __atomic_and_fetch(&ul1, ul2, __ATOMIC_SEQ_CST);
      volatile unsigned long long ull1 = 1, ull2 = 0, ull3 = 2;
      __atomic_load_n(&ull1, __ATOMIC_SEQ_CST);
      __atomic_compare_exchange(&ull1, &ull2, &ull3, 1, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST);
      __atomic_fetch_add(&ull1, 1, __ATOMIC_SEQ_CST);
      __atomic_fetch_sub(&ull3, 1, __ATOMIC_SEQ_CST);
      __atomic_or_fetch(&ull1, ull2, __ATOMIC_SEQ_CST);
      __atomic_and_fetch(&ull1, ull2, __ATOMIC_SEQ_CST);
      return 0;
    }
    "
    HAVE_C__ATOMIC)
```

For the specific problem of getting the correct type signature in log.c for the `strerror_r`
calls we can replicate what we have now, or we can delete this code completely and use a 
better solution that is documented [here](http://www.club.cc.cmu.edu/~cmccabe/blog_strerror.html).
To replicate what we have now:
```
check_c_source_compiles(
    "
    #include <string.h>
    int main() { char x = *strerror_r(0, &x, sizeof(x)); return 0; }
    "
    STRERROR_R_CHAR_P)

check_c_source_compiles(
    "
    #include <string.h>
    int main() { int x = strerror_r(0, &x, sizeof(x)); return 0; }
    "
    STRERROR_R_INT)

if("${STRERROR_R_CHAR_P}" OR "${STRERROR_R_INT}")
    set(HAVE_DECL_STRERROR_R 1)
endif()
message(STATUS "Result was ${HAVE_DECL_STRERROR_R}")

```

Note: I did not find an explicit way to select compiler when both `clang` and `gcc` are
present. We might have an implicit way (like redirecting `cc`) but we should put one in.



### Debugging problems in test compilations

Test compilations attempt to feed a test-input into the targetted compiler and result
in a yes/no decision, this is similar to `AC_LANG_SOURCE(.... if test $ac_...` in .`m4`.
We have two techniques to use in CMake:
```
cmake_minimum_required(VERSION 3.1.0)
include(CheckCCompilerFlag)
project(empty C)

check_c_source_compiles(
    "
    #include <string.h>
    int main() { char x = *strerror_r(0, &x, sizeof(x)); return 0; }
    "
    STRERROR_R_CHAR_P)

try_compile(HAVE_JEMALLOC ${CMAKE_CURRENT_BINARY_DIR}
            ${CMAKE_CURRENT_SOURCE_DIR}/quickdemo.c
            LINK_LIBRARIES jemalloc)
```

The `check_c_source_compiles` is light-weight:

* Inline source for the test, easy to follow.
* Build errors are reported in `CMakeFiles/CMakeErrors.log`

But we cannot alter the include-paths / library-paths / compiler-flags specifically for
the test without overwriting the current CMake settings. The alternative approach is
slightly more heavy-weight:

* Can't inline source for `try_compile` - it requires a `.c` file in the tree.
* Build errors are not shown, the recovery process for them is somewhat difficult.

```
rm -rf * && cmake .. --debug-trycompile
grep jemal CMakeFiles/CMakeTmp/CMakeFiles/*dir/*
cd CMakeFiles/CMakeTmp/CMakeFiles/cmTC_d6f0e.dir  # for example
cmake --build ../..
```

This implies that we can do this to diagnose problems / develop test-programs, but we
have to make them *bullet-proof* as we cannot expose this to end-users. This means that
the results of the compilation must be *crisp* - exactly yes/no if the feature we are
testing is supported.

### System configuration checks

For any system configuration checks that fall outside of the above scope (includes, libraries,
packages, test-compilation checks) we have a fall-back that we can use to glue any holes
that we need, e.g. to pull out the packaging strings, inside the `CMakeLists.h`:
```
execute_process(COMMAND cat ${CMAKE_CURRENT_SOURCE_DIR}/packaging/version
                COMMAND tr -d '\n'
                OUTPUT_VARIABLE VERSION_FROM_FILE)
message(STATUS "Packaging version ${VERSION_FROM_FILE}")
```
and this in the `config.h.in`:
```
#define VERSION_FROM_FILE "@VERSION_FROM_FILE@"
```

## CMake build time

We have a working definition of the targets that is in use with CLion and works on modern
CMake (3.15). It breaks on older CMake version (e.g. 3.7) with an error message (issue#7091).
No PoC yet to fix this, but it looks like changing the target properties should do it (in the
worst case we can drop the separate object completely and merge the sources directly into
the final target).

Steps needed for building a prototype:

1. Pick a reasonable configuration.
2. Use the PoC techniques above to do a full generation of `CMAKE_` variables in the cache
   according to the feature options and dependencies.
3. Push these into the project variables.
4. Work on it until the build succeeds in at least one known configuration.
5. Smoke-test that the output is valid (i.e. the executable loads and runs, and we can
   access the dashboard).
6. Do a full comparison of the `config.h` generated by autotools against the CMake version
   and document / fix any deviations.

## CMake install target

I've only looked at this superficially as we do not have a prototype yet, but each of the
first-stage install steps (in `make install`) and the second-stage (in `netdata-installer.sh`)
look feasible.

## General issues

*   We need to choose a minimum CMake version that is an available package across all of our
    supported environments. There is currently a build issue #7091 that documents a problem 
    in the compilation phase (we cannot link in libnetdata as an object on old CMake versions
    and need to find a different way to express this).

*   The default variable-expansion / comparisons in CMake are awkward, we need this to make it
    sane:
    ```
    cmake_policy(SET CMP0054 "NEW")
    ```
*   Default paths for libs / includes are not comprehensive on most environments, we still need
    some heuristics for common locations, e.g. `/usr/local` on FreeBSD.

# Recommendations

We should follow these steps:

1. Build a prototype.
2. Build a test-environment to check the prototype against environments / configurations that
   the team uses.
3. Perform an "internal" release - merge the new CMake into master, but not announce it or 
   offer to support it.
4. Check it works for the team internally.
5. Do a soft-release: offer it externally as a replacement option for autotools.
6. Gather feedback and usage reports on a wider range of configurations.
7. Do a hard-release: switch over the preferred build-system in the installation instructions.
8. Gather feedback and usage reports on a wider range of configurations (again).
9. Deprecate / remove the autotools build-system completely (so that we can support a single
   build-system).

Some smaller miscellaeneous suggestions:

1. Remove the `_Generic` / `strerror_r` config to make the system simpler (use the technique
   on the blog post to make the standard version re-enterant so that it is thread-safe).
2. Pull in jemalloc by source into the repo if it is our preferred malloc implementation.

# Background

* [Stack overflow starting point](https://stackoverflow.com/questions/7132862/how-do-i-convert-an-autotools-project-to-a-cmake-project#7680240)
* [CMake wiki including previous autotools conversions](https://gitlab.kitware.com/cmake/community/wikis/Home)
* [Commands section in old CMake docs](https://cmake.org/cmake/help/v2.8.8/cmake.html#section_Commands)
* [try_compile in newer CMake docs](https://cmake.org/cmake/help/v3.7/command/try_compile.html)
* [configure_file in newer CMake docs](https://cmake.org/cmake/help/v3.7/command/configure_file.html?highlight=configure_file)
* [header checks in CMake](https://stackoverflow.com/questions/647892/how-to-check-header-files-and-library-functions-in-cmake-like-it-is-done-in-auto)
* [how to write platform checks](https://gitlab.kitware.com/cmake/community/wikis/doc/tutorials/How-To-Write-Platform-Checks)

