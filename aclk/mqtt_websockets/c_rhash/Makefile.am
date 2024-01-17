crhash_includedir = -I$(abs_top_srcdir)/include

dist_doc_DATA = LICENSE \
    README.md \
    $(NULL)

CRHASH_FILES = src/c_rhash.c \
    src/c_rhash.h \
    $(NULL)

lib_LIBRARIES = libcrhash.a

libcrhash_a_SOURCES = $(CRHASH_FILES)

libcrhash_a_CFLAGS = $(crhash_includedir)

bin_PROGRAMS = test
test_LDADD = libcrhash.a

test_SOURCES = src/tests.c
test_CFLAGS = $(crhash_includedir)
