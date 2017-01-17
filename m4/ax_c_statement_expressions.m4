# AC_C_STMT_EXPR
# -------------
# Define HAVE_STMT_EXPR if compiler has statement expressions.
AN_IDENTIFIER([_Generic], [AC_C_STMT_EXPR])
AC_DEFUN([AC_C_STMT_EXPR],
[AC_CACHE_CHECK([for statement expressions], ac_cv_c_stmt_expr,
[AC_COMPILE_IFELSE(
   [AC_LANG_SOURCE(
      [[int
        main (int argc, char **argv)
        {
          int x = ({ int y = 1; y; });
          return x;
        }
      ]])],
   [ac_cv_c_stmt_expr=yes],
   [ac_cv_c_stmt_expr=no])])
if test $ac_cv_c_stmt_expr = yes; then
  AC_DEFINE([HAVE_STMT_EXPR], 1,
           [Define to 1 if compiler supports statement expressions.])
fi
])# AC_C_STMT_EXPR

