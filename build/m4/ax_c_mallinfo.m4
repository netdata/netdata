# AC_C_MALLINFO
# -------------
# Define HAVE_C_MALLINFO if mallinfo() works.
AN_IDENTIFIER([mallinfo], [AC_C_MALLINFO])
AC_DEFUN([AC_C_MALLINFO],
[AC_CACHE_CHECK([for mallinfo], ac_cv_c_mallinfo,
[AC_LINK_IFELSE(
  [AC_LANG_PROGRAM(
    [[#include <malloc.h>]],
    [[
      struct mallinfo mi = mallinfo();
      /* make sure that fields exists */
      mi.uordblks = 0;
      mi.hblkhd = 0;
      mi.arena = 0;
    ]]
  )],
  [ac_cv_c_mallinfo=yes],
  [ac_cv_c_mallinfo=no])])
if test $ac_cv_c_mallinfo = yes; then
  AC_DEFINE([HAVE_C_MALLINFO], 1,
           [Define to 1 if glibc mallinfo exists.])
fi
])# AC_C_MALLINFO
