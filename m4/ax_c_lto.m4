# AC_C_LTO
# -------------
# Define HAVE_LTO if -flto works.
AN_IDENTIFIER([lto], [AC_C_LTO])
AC_DEFUN([AC_C_LTO],
[AC_CACHE_CHECK([if -flto builds executables], ac_cv_c_lto,
[AC_RUN_IFELSE(
   [AC_LANG_SOURCE(
      [[#include <stdio.h>
        int main(int argc, char **argv) {
          return 0;
        }
      ]])],
   [ac_cv_c_lto=yes],
   [ac_cv_c_lto=no],
   [ac_cv_c_lto=${ac_cv_c_lto_cross_compile}])])
if test "${ac_cv_c_lto}" = "yes"; then
  AC_DEFINE([HAVE_LTO], 1,
           [Define to 1 if -flto works.])
fi
])# AC_C_LTO
