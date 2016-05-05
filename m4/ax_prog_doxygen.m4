# ===========================================================================
#      http://www.gnu.org/software/autoconf-archive/ax_prog_doxygen.html
# ===========================================================================
#
# SYNOPSIS
#
#   DX_INIT_DOXYGEN(PROJECT-NAME, [DOXYFILE-PATH], [OUTPUT-DIR], ...)
#   DX_DOXYGEN_FEATURE(ON|OFF)
#   DX_DOT_FEATURE(ON|OFF)
#   DX_HTML_FEATURE(ON|OFF)
#   DX_CHM_FEATURE(ON|OFF)
#   DX_CHI_FEATURE(ON|OFF)
#   DX_MAN_FEATURE(ON|OFF)
#   DX_RTF_FEATURE(ON|OFF)
#   DX_XML_FEATURE(ON|OFF)
#   DX_PDF_FEATURE(ON|OFF)
#   DX_PS_FEATURE(ON|OFF)
#
# DESCRIPTION
#
#   The DX_*_FEATURE macros control the default setting for the given
#   Doxygen feature. Supported features are 'DOXYGEN' itself, 'DOT' for
#   generating graphics, 'HTML' for plain HTML, 'CHM' for compressed HTML
#   help (for MS users), 'CHI' for generating a seperate .chi file by the
#   .chm file, and 'MAN', 'RTF', 'XML', 'PDF' and 'PS' for the appropriate
#   output formats. The environment variable DOXYGEN_PAPER_SIZE may be
#   specified to override the default 'a4wide' paper size.
#
#   By default, HTML, PDF and PS documentation is generated as this seems to
#   be the most popular and portable combination. MAN pages created by
#   Doxygen are usually problematic, though by picking an appropriate subset
#   and doing some massaging they might be better than nothing. CHM and RTF
#   are specific for MS (note that you can't generate both HTML and CHM at
#   the same time). The XML is rather useless unless you apply specialized
#   post-processing to it.
#
#   The macros mainly control the default state of the feature. The use can
#   override the default by specifying --enable or --disable. The macros
#   ensure that contradictory flags are not given (e.g.,
#   --enable-doxygen-html and --enable-doxygen-chm,
#   --enable-doxygen-anything with --disable-doxygen, etc.) Finally, each
#   feature will be automatically disabled (with a warning) if the required
#   programs are missing.
#
#   Once all the feature defaults have been specified, call DX_INIT_DOXYGEN
#   with the following parameters: a one-word name for the project for use
#   as a filename base etc., an optional configuration file name (the
#   default is '$(srcdir)/Doxyfile', the same as Doxygen's default), and an
#   optional output directory name (the default is 'doxygen-doc'). To run
#   doxygen multiple times for different configuration files and output
#   directories provide more parameters: the second, forth, sixth, etc
#   parameter are configuration file names and the third, fifth, seventh,
#   etc parameter are output directories. No checking is done to catch
#   duplicates.
#
#   Automake Support
#
#   The DX_RULES substitution can be used to add all needed rules to the
#   Makefile. Note that this is a substitution without being a variable:
#   only the @DX_RULES@ syntax will work.
#
#   The provided targets are:
#
#     doxygen-doc: Generate all doxygen documentation.
#
#     doxygen-run: Run doxygen, which will generate some of the
#                  documentation (HTML, CHM, CHI, MAN, RTF, XML)
#                  but will not do the post processing required
#                  for the rest of it (PS, PDF).
#
#     doxygen-ps:  Generate doxygen PostScript documentation.
#
#     doxygen-pdf: Generate doxygen PDF documentation.
#
#   Note that by default these are not integrated into the automake targets.
#   If doxygen is used to generate man pages, you can achieve this
#   integration by setting man3_MANS to the list of man pages generated and
#   then adding the dependency:
#
#     $(man3_MANS): doxygen-doc
#
#   This will cause make to run doxygen and generate all the documentation.
#
#   The following variable is intended for use in Makefile.am:
#
#     DX_CLEANFILES = everything to clean.
#
#   Then add this variable to MOSTLYCLEANFILES.
#
# LICENSE
#
#   Copyright (c) 2009 Oren Ben-Kiki <oren@ben-kiki.org>
#   Copyright (c) 2015 Olaf Mandel <olaf@mandel.name>
#
#   Copying and distribution of this file, with or without modification, are
#   permitted in any medium without royalty provided the copyright notice
#   and this notice are preserved. This file is offered as-is, without any
#   warranty.

#serial 20

## ----------##
## Defaults. ##
## ----------##

DX_ENV=""
AC_DEFUN([DX_FEATURE_doc],  ON)
AC_DEFUN([DX_FEATURE_dot],  OFF)
AC_DEFUN([DX_FEATURE_man],  OFF)
AC_DEFUN([DX_FEATURE_html], ON)
AC_DEFUN([DX_FEATURE_chm],  OFF)
AC_DEFUN([DX_FEATURE_chi],  OFF)
AC_DEFUN([DX_FEATURE_rtf],  OFF)
AC_DEFUN([DX_FEATURE_xml],  OFF)
AC_DEFUN([DX_FEATURE_pdf],  ON)
AC_DEFUN([DX_FEATURE_ps],   ON)

## --------------- ##
## Private macros. ##
## --------------- ##

# DX_ENV_APPEND(VARIABLE, VALUE)
# ------------------------------
# Append VARIABLE="VALUE" to DX_ENV for invoking doxygen and add it
# as a substitution (but not a Makefile variable). The substitution
# is skipped if the variable name is VERSION.
AC_DEFUN([DX_ENV_APPEND],
[AC_SUBST([DX_ENV], ["$DX_ENV $1='$2'"])dnl
m4_if([$1], [VERSION], [], [AC_SUBST([$1], [$2])dnl
AM_SUBST_NOTMAKE([$1])])dnl
])

# DX_DIRNAME_EXPR
# ---------------
# Expand into a shell expression prints the directory part of a path.
AC_DEFUN([DX_DIRNAME_EXPR],
         [[expr ".$1" : '\(\.\)[^/]*$' \| "x$1" : 'x\(.*\)/[^/]*$']])

# DX_IF_FEATURE(FEATURE, IF-ON, IF-OFF)
# -------------------------------------
# Expands according to the M4 (static) status of the feature.
AC_DEFUN([DX_IF_FEATURE], [ifelse(DX_FEATURE_$1, ON, [$2], [$3])])

# DX_REQUIRE_PROG(VARIABLE, PROGRAM)
# ----------------------------------
# Require the specified program to be found for the DX_CURRENT_FEATURE to work.
AC_DEFUN([DX_REQUIRE_PROG], [
AC_PATH_TOOL([$1], [$2])
if test "$DX_FLAG_[]DX_CURRENT_FEATURE$$1" = 1; then
    AC_MSG_WARN([$2 not found - will not DX_CURRENT_DESCRIPTION])
    AC_SUBST(DX_FLAG_[]DX_CURRENT_FEATURE, 0)
fi
])

# DX_TEST_FEATURE(FEATURE)
# ------------------------
# Expand to a shell expression testing whether the feature is active.
AC_DEFUN([DX_TEST_FEATURE], [test "$DX_FLAG_$1" = 1])

# DX_CHECK_DEPEND(REQUIRED_FEATURE, REQUIRED_STATE)
# -------------------------------------------------
# Verify that a required features has the right state before trying to turn on
# the DX_CURRENT_FEATURE.
AC_DEFUN([DX_CHECK_DEPEND], [
test "$DX_FLAG_$1" = "$2" \
|| AC_MSG_ERROR([doxygen-DX_CURRENT_FEATURE ifelse([$2], 1,
                            requires, contradicts) doxygen-DX_CURRENT_FEATURE])
])

# DX_CLEAR_DEPEND(FEATURE, REQUIRED_FEATURE, REQUIRED_STATE)
# ----------------------------------------------------------
# Turn off the DX_CURRENT_FEATURE if the required feature is off.
AC_DEFUN([DX_CLEAR_DEPEND], [
test "$DX_FLAG_$1" = "$2" || AC_SUBST(DX_FLAG_[]DX_CURRENT_FEATURE, 0)
])

# DX_FEATURE_ARG(FEATURE, DESCRIPTION,
#                CHECK_DEPEND, CLEAR_DEPEND,
#                REQUIRE, DO-IF-ON, DO-IF-OFF)
# --------------------------------------------
# Parse the command-line option controlling a feature. CHECK_DEPEND is called
# if the user explicitly turns the feature on (and invokes DX_CHECK_DEPEND),
# otherwise CLEAR_DEPEND is called to turn off the default state if a required
# feature is disabled (using DX_CLEAR_DEPEND). REQUIRE performs additional
# requirement tests (DX_REQUIRE_PROG). Finally, an automake flag is set and
# DO-IF-ON or DO-IF-OFF are called according to the final state of the feature.
AC_DEFUN([DX_ARG_ABLE], [
    AC_DEFUN([DX_CURRENT_FEATURE], [$1])
    AC_DEFUN([DX_CURRENT_DESCRIPTION], [$2])
    AC_ARG_ENABLE(doxygen-$1,
                  [AS_HELP_STRING(DX_IF_FEATURE([$1], [--disable-doxygen-$1],
                                                      [--enable-doxygen-$1]),
                                  DX_IF_FEATURE([$1], [don't $2], [$2]))],
                  [
case "$enableval" in
#(
y|Y|yes|Yes|YES)
    AC_SUBST([DX_FLAG_$1], 1)
    $3
;; #(
n|N|no|No|NO)
    AC_SUBST([DX_FLAG_$1], 0)
;; #(
*)
    AC_MSG_ERROR([invalid value '$enableval' given to doxygen-$1])
;;
esac
], [
AC_SUBST([DX_FLAG_$1], [DX_IF_FEATURE([$1], 1, 0)])
$4
])
if DX_TEST_FEATURE([$1]); then
    $5
    :
fi
if DX_TEST_FEATURE([$1]); then
    $6
    :
else
    $7
    :
fi
])

## -------------- ##
## Public macros. ##
## -------------- ##

# DX_XXX_FEATURE(DEFAULT_STATE)
# -----------------------------
AC_DEFUN([DX_DOXYGEN_FEATURE], [AC_DEFUN([DX_FEATURE_doc],  [$1])])
AC_DEFUN([DX_DOT_FEATURE],     [AC_DEFUN([DX_FEATURE_dot], [$1])])
AC_DEFUN([DX_MAN_FEATURE],     [AC_DEFUN([DX_FEATURE_man],  [$1])])
AC_DEFUN([DX_HTML_FEATURE],    [AC_DEFUN([DX_FEATURE_html], [$1])])
AC_DEFUN([DX_CHM_FEATURE],     [AC_DEFUN([DX_FEATURE_chm],  [$1])])
AC_DEFUN([DX_CHI_FEATURE],     [AC_DEFUN([DX_FEATURE_chi],  [$1])])
AC_DEFUN([DX_RTF_FEATURE],     [AC_DEFUN([DX_FEATURE_rtf],  [$1])])
AC_DEFUN([DX_XML_FEATURE],     [AC_DEFUN([DX_FEATURE_xml],  [$1])])
AC_DEFUN([DX_XML_FEATURE],     [AC_DEFUN([DX_FEATURE_xml],  [$1])])
AC_DEFUN([DX_PDF_FEATURE],     [AC_DEFUN([DX_FEATURE_pdf],  [$1])])
AC_DEFUN([DX_PS_FEATURE],      [AC_DEFUN([DX_FEATURE_ps],   [$1])])

# DX_INIT_DOXYGEN(PROJECT, [CONFIG-FILE], [OUTPUT-DOC-DIR], ...)
# --------------------------------------------------------------
# PROJECT also serves as the base name for the documentation files.
# The default CONFIG-FILE is "$(srcdir)/Doxyfile" and OUTPUT-DOC-DIR is
# "doxygen-doc".
# More arguments are interpreted as interleaved CONFIG-FILE and
# OUTPUT-DOC-DIR values.
AC_DEFUN([DX_INIT_DOXYGEN], [

# Files:
AC_SUBST([DX_PROJECT], [$1])
AC_SUBST([DX_CONFIG], ['ifelse([$2], [], [$(srcdir)/Doxyfile], [$2])'])
AC_SUBST([DX_DOCDIR], ['ifelse([$3], [], [doxygen-doc], [$3])'])
m4_if(m4_eval(3 < m4_count($@)), 1, [m4_for([DX_i], 4, m4_count($@), 2,
      [AC_SUBST([DX_CONFIG]m4_eval(DX_i[/2]),
                'm4_default_nblank_quoted(m4_argn(DX_i, $@),
                                          [$(srcdir)/Doxyfile])')])])dnl
m4_if(m4_eval(3 < m4_count($@)), 1, [m4_for([DX_i], 5, m4_count($@,), 2,
      [AC_SUBST([DX_DOCDIR]m4_eval([(]DX_i[-1)/2]),
                'm4_default_nblank_quoted(m4_argn(DX_i, $@),
                                          [doxygen-doc])')])])dnl
m4_define([DX_loop], m4_dquote(m4_if(m4_eval(3 < m4_count($@)), 1,
          [m4_for([DX_i], 4, m4_count($@), 2, [, m4_eval(DX_i[/2])])],
          [])))dnl

# Environment variables used inside doxygen.cfg:
DX_ENV_APPEND(SRCDIR, $srcdir)
DX_ENV_APPEND(PROJECT, $DX_PROJECT)
DX_ENV_APPEND(VERSION, $PACKAGE_VERSION)

# Doxygen itself:
DX_ARG_ABLE(doc, [generate any doxygen documentation],
            [],
            [],
            [DX_REQUIRE_PROG([DX_DOXYGEN], doxygen)
             DX_REQUIRE_PROG([DX_PERL], perl)],
            [DX_ENV_APPEND(PERL_PATH, $DX_PERL)])

# Dot for graphics:
DX_ARG_ABLE(dot, [generate graphics for doxygen documentation],
            [DX_CHECK_DEPEND(doc, 1)],
            [DX_CLEAR_DEPEND(doc, 1)],
            [DX_REQUIRE_PROG([DX_DOT], dot)],
            [DX_ENV_APPEND(HAVE_DOT, YES)
             DX_ENV_APPEND(DOT_PATH, [`DX_DIRNAME_EXPR($DX_DOT)`])],
            [DX_ENV_APPEND(HAVE_DOT, NO)])

# Man pages generation:
DX_ARG_ABLE(man, [generate doxygen manual pages],
            [DX_CHECK_DEPEND(doc, 1)],
            [DX_CLEAR_DEPEND(doc, 1)],
            [],
            [DX_ENV_APPEND(GENERATE_MAN, YES)],
            [DX_ENV_APPEND(GENERATE_MAN, NO)])

# RTF file generation:
DX_ARG_ABLE(rtf, [generate doxygen RTF documentation],
            [DX_CHECK_DEPEND(doc, 1)],
            [DX_CLEAR_DEPEND(doc, 1)],
            [],
            [DX_ENV_APPEND(GENERATE_RTF, YES)],
            [DX_ENV_APPEND(GENERATE_RTF, NO)])

# XML file generation:
DX_ARG_ABLE(xml, [generate doxygen XML documentation],
            [DX_CHECK_DEPEND(doc, 1)],
            [DX_CLEAR_DEPEND(doc, 1)],
            [],
            [DX_ENV_APPEND(GENERATE_XML, YES)],
            [DX_ENV_APPEND(GENERATE_XML, NO)])

# (Compressed) HTML help generation:
DX_ARG_ABLE(chm, [generate doxygen compressed HTML help documentation],
            [DX_CHECK_DEPEND(doc, 1)],
            [DX_CLEAR_DEPEND(doc, 1)],
            [DX_REQUIRE_PROG([DX_HHC], hhc)],
            [DX_ENV_APPEND(HHC_PATH, $DX_HHC)
             DX_ENV_APPEND(GENERATE_HTML, YES)
             DX_ENV_APPEND(GENERATE_HTMLHELP, YES)],
            [DX_ENV_APPEND(GENERATE_HTMLHELP, NO)])

# Seperate CHI file generation.
DX_ARG_ABLE(chi, [generate doxygen seperate compressed HTML help index file],
            [DX_CHECK_DEPEND(chm, 1)],
            [DX_CLEAR_DEPEND(chm, 1)],
            [],
            [DX_ENV_APPEND(GENERATE_CHI, YES)],
            [DX_ENV_APPEND(GENERATE_CHI, NO)])

# Plain HTML pages generation:
DX_ARG_ABLE(html, [generate doxygen plain HTML documentation],
            [DX_CHECK_DEPEND(doc, 1) DX_CHECK_DEPEND(chm, 0)],
            [DX_CLEAR_DEPEND(doc, 1) DX_CLEAR_DEPEND(chm, 0)],
            [],
            [DX_ENV_APPEND(GENERATE_HTML, YES)],
            [DX_TEST_FEATURE(chm) || DX_ENV_APPEND(GENERATE_HTML, NO)])

# PostScript file generation:
DX_ARG_ABLE(ps, [generate doxygen PostScript documentation],
            [DX_CHECK_DEPEND(doc, 1)],
            [DX_CLEAR_DEPEND(doc, 1)],
            [DX_REQUIRE_PROG([DX_LATEX], latex)
             DX_REQUIRE_PROG([DX_MAKEINDEX], makeindex)
             DX_REQUIRE_PROG([DX_DVIPS], dvips)
             DX_REQUIRE_PROG([DX_EGREP], egrep)])

# PDF file generation:
DX_ARG_ABLE(pdf, [generate doxygen PDF documentation],
            [DX_CHECK_DEPEND(doc, 1)],
            [DX_CLEAR_DEPEND(doc, 1)],
            [DX_REQUIRE_PROG([DX_PDFLATEX], pdflatex)
             DX_REQUIRE_PROG([DX_MAKEINDEX], makeindex)
             DX_REQUIRE_PROG([DX_EGREP], egrep)])

# LaTeX generation for PS and/or PDF:
if DX_TEST_FEATURE(ps) || DX_TEST_FEATURE(pdf); then
    DX_ENV_APPEND(GENERATE_LATEX, YES)
else
    DX_ENV_APPEND(GENERATE_LATEX, NO)
fi

# Paper size for PS and/or PDF:
AC_ARG_VAR(DOXYGEN_PAPER_SIZE,
           [a4wide (default), a4, letter, legal or executive])
case "$DOXYGEN_PAPER_SIZE" in
#(
"")
    AC_SUBST(DOXYGEN_PAPER_SIZE, "")
;; #(
a4wide|a4|letter|legal|executive)
    DX_ENV_APPEND(PAPER_SIZE, $DOXYGEN_PAPER_SIZE)
;; #(
*)
    AC_MSG_ERROR([unknown DOXYGEN_PAPER_SIZE='$DOXYGEN_PAPER_SIZE'])
;;
esac

# Rules:
AS_IF([[test $DX_FLAG_html -eq 1]],
[[DX_SNIPPET_html="## ------------------------------- ##
## Rules specific for HTML output. ##
## ------------------------------- ##

DX_CLEAN_HTML = \$(DX_DOCDIR)/html]dnl
m4_foreach([DX_i], [m4_shift(DX_loop)], [[\\
                \$(DX_DOCDIR]DX_i[)/html]])[

"]],
[[DX_SNIPPET_html=""]])
AS_IF([[test $DX_FLAG_chi -eq 1]],
[[DX_SNIPPET_chi="
DX_CLEAN_CHI = \$(DX_DOCDIR)/\$(PACKAGE).chi]dnl
m4_foreach([DX_i], [m4_shift(DX_loop)], [[\\
               \$(DX_DOCDIR]DX_i[)/\$(PACKAGE).chi]])["]],
[[DX_SNIPPET_chi=""]])
AS_IF([[test $DX_FLAG_chm -eq 1]],
[[DX_SNIPPET_chm="## ------------------------------ ##
## Rules specific for CHM output. ##
## ------------------------------ ##

DX_CLEAN_CHM = \$(DX_DOCDIR)/chm]dnl
m4_foreach([DX_i], [m4_shift(DX_loop)], [[\\
               \$(DX_DOCDIR]DX_i[)/chm]])[\
${DX_SNIPPET_chi}

"]],
[[DX_SNIPPET_chm=""]])
AS_IF([[test $DX_FLAG_man -eq 1]],
[[DX_SNIPPET_man="## ------------------------------ ##
## Rules specific for MAN output. ##
## ------------------------------ ##

DX_CLEAN_MAN = \$(DX_DOCDIR)/man]dnl
m4_foreach([DX_i], [m4_shift(DX_loop)], [[\\
               \$(DX_DOCDIR]DX_i[)/man]])[

"]],
[[DX_SNIPPET_man=""]])
AS_IF([[test $DX_FLAG_rtf -eq 1]],
[[DX_SNIPPET_rtf="## ------------------------------ ##
## Rules specific for RTF output. ##
## ------------------------------ ##

DX_CLEAN_RTF = \$(DX_DOCDIR)/rtf]dnl
m4_foreach([DX_i], [m4_shift(DX_loop)], [[\\
               \$(DX_DOCDIR]DX_i[)/rtf]])[

"]],
[[DX_SNIPPET_rtf=""]])
AS_IF([[test $DX_FLAG_xml -eq 1]],
[[DX_SNIPPET_xml="## ------------------------------ ##
## Rules specific for XML output. ##
## ------------------------------ ##

DX_CLEAN_XML = \$(DX_DOCDIR)/xml]dnl
m4_foreach([DX_i], [m4_shift(DX_loop)], [[\\
               \$(DX_DOCDIR]DX_i[)/xml]])[

"]],
[[DX_SNIPPET_xml=""]])
AS_IF([[test $DX_FLAG_ps -eq 1]],
[[DX_SNIPPET_ps="## ----------------------------- ##
## Rules specific for PS output. ##
## ----------------------------- ##

DX_CLEAN_PS = \$(DX_DOCDIR)/\$(PACKAGE).ps]dnl
m4_foreach([DX_i], [m4_shift(DX_loop)], [[\\
              \$(DX_DOCDIR]DX_i[)/\$(PACKAGE).ps]])[

DX_PS_GOAL = doxygen-ps

doxygen-ps: \$(DX_CLEAN_PS)

]m4_foreach([DX_i], [DX_loop],
[[\$(DX_DOCDIR]DX_i[)/\$(PACKAGE).ps: \$(DX_DOCDIR]DX_i[)/\$(PACKAGE).tag
	\$(DX_V_LATEX)cd \$(DX_DOCDIR]DX_i[)/latex; \\
	rm -f *.aux *.toc *.idx *.ind *.ilg *.log *.out; \\
	\$(DX_LATEX) refman.tex; \\
	\$(DX_MAKEINDEX) refman.idx; \\
	\$(DX_LATEX) refman.tex; \\
	countdown=5; \\
	while \$(DX_EGREP) 'Rerun (LaTeX|to get cross-references right)' \\
	                  refman.log > /dev/null 2>&1 \\
	   && test \$\$countdown -gt 0; do \\
	    \$(DX_LATEX) refman.tex; \\
            countdown=\`expr \$\$countdown - 1\`; \\
	done; \\
	\$(DX_DVIPS) -o ../\$(PACKAGE).ps refman.dvi

]])["]],
[[DX_SNIPPET_ps=""]])
AS_IF([[test $DX_FLAG_pdf -eq 1]],
[[DX_SNIPPET_pdf="## ------------------------------ ##
## Rules specific for PDF output. ##
## ------------------------------ ##

DX_CLEAN_PDF = \$(DX_DOCDIR)/\$(PACKAGE).pdf]dnl
m4_foreach([DX_i], [m4_shift(DX_loop)], [[\\
               \$(DX_DOCDIR]DX_i[)/\$(PACKAGE).pdf]])[

DX_PDF_GOAL = doxygen-pdf

doxygen-pdf: \$(DX_CLEAN_PDF)

]m4_foreach([DX_i], [DX_loop],
[[\$(DX_DOCDIR]DX_i[)/\$(PACKAGE).pdf: \$(DX_DOCDIR]DX_i[)/\$(PACKAGE).tag
	\$(DX_V_LATEX)cd \$(DX_DOCDIR]DX_i[)/latex; \\
	rm -f *.aux *.toc *.idx *.ind *.ilg *.log *.out; \\
	\$(DX_PDFLATEX) refman.tex; \\
	\$(DX_MAKEINDEX) refman.idx; \\
	\$(DX_PDFLATEX) refman.tex; \\
	countdown=5; \\
	while \$(DX_EGREP) 'Rerun (LaTeX|to get cross-references right)' \\
	                  refman.log > /dev/null 2>&1 \\
	   && test \$\$countdown -gt 0; do \\
	    \$(DX_PDFLATEX) refman.tex; \\
	    countdown=\`expr \$\$countdown - 1\`; \\
	done; \\
	mv refman.pdf ../\$(PACKAGE).pdf

]])["]],
[[DX_SNIPPET_pdf=""]])
AS_IF([[test $DX_FLAG_ps -eq 1 -o $DX_FLAG_pdf -eq 1]],
[[DX_SNIPPET_latex="## ------------------------------------------------- ##
## Rules specific for LaTeX (shared for PS and PDF). ##
## ------------------------------------------------- ##

DX_V_LATEX = \$(_DX_v_LATEX_\$(V))
_DX_v_LATEX_ = \$(_DX_v_LATEX_\$(AM_DEFAULT_VERBOSITY))
_DX_v_LATEX_0 = @echo \"  LATEX \" \$][@;

DX_CLEAN_LATEX = \$(DX_DOCDIR)/latex]dnl
m4_foreach([DX_i], [m4_shift(DX_loop)], [[\\
                 \$(DX_DOCDIR]DX_i[)/latex]])[

"]],
[[DX_SNIPPET_latex=""]])

AS_IF([[test $DX_FLAG_doc -eq 1]],
[[DX_SNIPPET_doc="## --------------------------------- ##
## Format-independent Doxygen rules. ##
## --------------------------------- ##

${DX_SNIPPET_html}\
${DX_SNIPPET_chm}\
${DX_SNIPPET_man}\
${DX_SNIPPET_rtf}\
${DX_SNIPPET_xml}\
${DX_SNIPPET_ps}\
${DX_SNIPPET_pdf}\
${DX_SNIPPET_latex}\
DX_V_DXGEN = \$(_DX_v_DXGEN_\$(V))
_DX_v_DXGEN_ = \$(_DX_v_DXGEN_\$(AM_DEFAULT_VERBOSITY))
_DX_v_DXGEN_0 = @echo \"  DXGEN \" \$<;

.PHONY: doxygen-run doxygen-doc \$(DX_PS_GOAL) \$(DX_PDF_GOAL)

.INTERMEDIATE: doxygen-run \$(DX_PS_GOAL) \$(DX_PDF_GOAL)

doxygen-run:]m4_foreach([DX_i], [DX_loop],
                         [[ \$(DX_DOCDIR]DX_i[)/\$(PACKAGE).tag]])[

doxygen-doc: doxygen-run \$(DX_PS_GOAL) \$(DX_PDF_GOAL)

]m4_foreach([DX_i], [DX_loop],
[[\$(DX_DOCDIR]DX_i[)/\$(PACKAGE).tag: \$(DX_CONFIG]DX_i[) \$(pkginclude_HEADERS)
	\$(A""M_V_at)rm -rf \$(DX_DOCDIR]DX_i[)
	\$(DX_V_DXGEN)\$(DX_ENV) DOCDIR=\$(DX_DOCDIR]DX_i[) \$(DX_DOXYGEN) \$(DX_CONFIG]DX_i[)
	\$(A""M_V_at)echo Timestamp >\$][@

]])dnl
[DX_CLEANFILES = \\]
m4_foreach([DX_i], [DX_loop],
[[	\$(DX_DOCDIR]DX_i[)/doxygen_sqlite3.db \\
	\$(DX_DOCDIR]DX_i[)/\$(PACKAGE).tag \\
]])dnl
[	-r \\
	\$(DX_CLEAN_HTML) \\
	\$(DX_CLEAN_CHM) \\
	\$(DX_CLEAN_CHI) \\
	\$(DX_CLEAN_MAN) \\
	\$(DX_CLEAN_RTF) \\
	\$(DX_CLEAN_XML) \\
	\$(DX_CLEAN_PS) \\
	\$(DX_CLEAN_PDF) \\
	\$(DX_CLEAN_LATEX)"]],
[[DX_SNIPPET_doc=""]])
AC_SUBST([DX_RULES],
["${DX_SNIPPET_doc}"])dnl
AM_SUBST_NOTMAKE([DX_RULES])

#For debugging:
#echo DX_FLAG_doc=$DX_FLAG_doc
#echo DX_FLAG_dot=$DX_FLAG_dot
#echo DX_FLAG_man=$DX_FLAG_man
#echo DX_FLAG_html=$DX_FLAG_html
#echo DX_FLAG_chm=$DX_FLAG_chm
#echo DX_FLAG_chi=$DX_FLAG_chi
#echo DX_FLAG_rtf=$DX_FLAG_rtf
#echo DX_FLAG_xml=$DX_FLAG_xml
#echo DX_FLAG_pdf=$DX_FLAG_pdf
#echo DX_FLAG_ps=$DX_FLAG_ps
#echo DX_ENV=$DX_ENV
])