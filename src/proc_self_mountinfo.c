// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

// ----------------------------------------------------------------------------
// taken from gnulib/mountlist.c

#ifndef ME_REMOTE
/* A file system is "remote" if its Fs_name contains a ':'
   or if (it is of type (smbfs or cifs) and its Fs_name starts with '//')
   or Fs_name is equal to "-hosts" (used by autofs to mount remote fs).  */
# define ME_REMOTE(Fs_name, Fs_type)            \
    (strchr (Fs_name, ':') != NULL              \
     || ((Fs_name)[0] == '/'                    \
         && (Fs_name)[1] == '/'                 \
         && (strcmp (Fs_type, "smbfs") == 0     \
             || strcmp (Fs_type, "cifs") == 0)) \
     || (strcmp("-hosts", Fs_name) == 0))
#endif

#define ME_DUMMY_0(Fs_name, Fs_type)            \
  (strcmp (Fs_type, "autofs") == 0              \
   || strcmp (Fs_type, "proc") == 0             \
   || strcmp (Fs_type, "subfs") == 0            \
   /* for Linux 2.6/3.x */                      \
   || strcmp (Fs_type, "debugfs") == 0          \
   || strcmp (Fs_type, "devpts") == 0           \
   || strcmp (Fs_type, "fusectl") == 0          \
   || strcmp (Fs_type, "mqueue") == 0           \
   || strcmp (Fs_type, "rpc_pipefs") == 0       \
   || strcmp (Fs_type, "sysfs") == 0            \
   /* FreeBSD, Linux 2.4 */                     \
   || strcmp (Fs_type, "devfs") == 0            \
   /* for NetBSD 3.0 */                         \
   || strcmp (Fs_type, "kernfs") == 0           \
   /* for Irix 6.5 */                           \
   || strcmp (Fs_type, "ignore") == 0)

/* Historically, we have marked as "dummy" any file system of type "none",
   but now that programs like du need to know about bind-mounted directories,
   we grant an exception to any with "bind" in its list of mount options.
   I.e., those are *not* dummy entries.  */
# define ME_DUMMY(Fs_name, Fs_type)		\
  (ME_DUMMY_0 (Fs_name, Fs_type) || strcmp (Fs_type, "none") == 0)

// ----------------------------------------------------------------------------

// find the mount info with the given major:minor
// in the supplied linked list of mountinfo structures
struct mountinfo *mountinfo_find(struct mountinfo *root, unsigned long major, unsigned long minor) {
    struct mountinfo *mi;

    for(mi = root; mi ; mi = mi->next)
        if(unlikely(mi->major == major && mi->minor == minor))
            return mi;

    return NULL;
}

// find the mount info with the given filesystem and mount_source
// in the supplied linked list of mountinfo structures
struct mountinfo *mountinfo_find_by_filesystem_mount_source(struct mountinfo *root, const char *filesystem, const char *mount_source) {
    struct mountinfo *mi;
    uint32_t filesystem_hash = simple_hash(filesystem), mount_source_hash = simple_hash(mount_source);

    for(mi = root; mi ; mi = mi->next)
        if(unlikely(mi->filesystem
                && mi->mount_source
                && mi->filesystem_hash == filesystem_hash
                && mi->mount_source_hash == mount_source_hash
                && !strcmp(mi->filesystem, filesystem)
                && !strcmp(mi->mount_source, mount_source)))
            return mi;

    return NULL;
}

struct mountinfo *mountinfo_find_by_filesystem_super_option(struct mountinfo *root, const char *filesystem, const char *super_options) {
    struct mountinfo *mi;
    uint32_t filesystem_hash = simple_hash(filesystem);

    size_t solen = strlen(super_options);

    for(mi = root; mi ; mi = mi->next)
        if(unlikely(mi->filesystem
                && mi->super_options
                && mi->filesystem_hash == filesystem_hash
                && !strcmp(mi->filesystem, filesystem))) {

            // super_options is a comma separated list
            char *s = mi->super_options, *e;
            while(*s) {
                e = s + 1;
                while(*e && *e != ',') e++;

                size_t len = e - s;
                if(unlikely(len == solen && !strncmp(s, super_options, len)))
                    return mi;

                if(*e == ',') s = ++e;
                else s = e;
            }
        }

    return NULL;
}

static void mountinfo_free(struct mountinfo *mi) {
    freez(mi->root);
    freez(mi->mount_point);
    freez(mi->mount_options);
    freez(mi->persistent_id);
/*
    if(mi->optional_fields_count) {
        int i;
        for(i = 0; i < mi->optional_fields_count ; i++)
            free(*mi->optional_fields[i]);
    }
    free(mi->optional_fields);
*/
    freez(mi->filesystem);
    freez(mi->mount_source);
    freez(mi->super_options);
    freez(mi);
}

// free a linked list of mountinfo structures
void mountinfo_free_all(struct mountinfo *mi) {
    while(mi) {
        struct mountinfo *t = mi;
        mi = mi->next;

        mountinfo_free(t);
    }
}

static char *strdupz_decoding_octal(const char *string) {
    char *buffer = strdupz(string);

    char *d = buffer;
    const char *s = string;

    while(*s) {
        if(unlikely(*s == '\\')) {
            s++;
            if(likely(isdigit(*s) && isdigit(s[1]) && isdigit(s[2]))) {
                char c = *s++ - '0';
                c <<= 3;
                c |= *s++ - '0';
                c <<= 3;
                c |= *s++ - '0';
                *d++ = c;
            }
            else *d++ = '_';
        }
        else *d++ = *s++;
    }
    *d = '\0';

    return buffer;
}

static inline int is_read_only(const char *s) {
    if(!s) return 0;

    size_t len = strlen(s);
    if(len < 2) return 0;
    if(len == 2) {
        if(!strcmp(s, "ro")) return 1;
        return 0;
    }
    if(!strncmp(s, "ro,", 3)) return 1;
    if(!strncmp(&s[len - 3], ",ro", 3)) return 1;
    if(strstr(s, ",ro,")) return 1;
    return 0;
}

// read the whole mountinfo into a linked list
struct mountinfo *mountinfo_read(int do_statvfs) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/self/mountinfo", netdata_configured_host_prefix);
    procfile *ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
    if(unlikely(!ff)) {
        snprintfz(filename, FILENAME_MAX, "%s/proc/1/mountinfo", netdata_configured_host_prefix);
        ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return NULL;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return NULL;

    struct mountinfo *root = NULL, *last = NULL, *mi = NULL;

    unsigned long l, lines = procfile_lines(ff);
    for(l = 0; l < lines ;l++) {
        if(unlikely(procfile_linewords(ff, l) < 5))
            continue;

        mi = mallocz(sizeof(struct mountinfo));

        unsigned long w = 0;
        mi->id = str2ul(procfile_lineword(ff, l, w)); w++;
        mi->parentid = str2ul(procfile_lineword(ff, l, w)); w++;

        char *major = procfile_lineword(ff, l, w), *minor; w++;
        for(minor = major; *minor && *minor != ':' ;minor++) ;

        if(unlikely(!*minor)) {
            error("Cannot parse major:minor on '%s' at line %lu of '%s'", major, l + 1, filename);
            freez(mi);
            continue;
        }

        *minor = '\0';
        minor++;

        mi->flags = 0;

        mi->major = str2ul(major);
        mi->minor = str2ul(minor);

        mi->root = strdupz(procfile_lineword(ff, l, w)); w++;
        mi->root_hash = simple_hash(mi->root);

        mi->mount_point = strdupz_decoding_octal(procfile_lineword(ff, l, w)); w++;
        mi->mount_point_hash = simple_hash(mi->mount_point);

        mi->persistent_id = strdupz(mi->mount_point);
        netdata_fix_chart_id(mi->persistent_id);
        mi->persistent_id_hash = simple_hash(mi->persistent_id);

        mi->mount_options = strdupz(procfile_lineword(ff, l, w)); w++;

        if(unlikely(is_read_only(mi->mount_options)))
            mi->flags |= MOUNTINFO_READONLY;

        // count the optional fields
/*
        unsigned long wo = w;
*/
        mi->optional_fields_count = 0;
        char *s = procfile_lineword(ff, l, w);
        while(*s && *s != '-') {
            w++;
            s = procfile_lineword(ff, l, w);
            mi->optional_fields_count++;
        }

/*
        if(unlikely(mi->optional_fields_count)) {
            // we have some optional fields
            // read them into a new array of pointers;

            mi->optional_fields = mallocz(mi->optional_fields_count * sizeof(char *));

            int i;
            for(i = 0; i < mi->optional_fields_count ; i++) {
                *mi->optional_fields[wo] = strdupz(procfile_lineword(ff, l, w));
                wo++;
            }
        }
        else
            mi->optional_fields = NULL;
*/

        if(likely(*s == '-')) {
            w++;

            mi->filesystem = strdupz(procfile_lineword(ff, l, w)); w++;
            mi->filesystem_hash = simple_hash(mi->filesystem);

            mi->mount_source = strdupz_decoding_octal(procfile_lineword(ff, l, w)); w++;
            mi->mount_source_hash = simple_hash(mi->mount_source);

            mi->super_options = strdupz(procfile_lineword(ff, l, w)); w++;

            if(unlikely(is_read_only(mi->super_options)))
                mi->flags |= MOUNTINFO_READONLY;

            if(unlikely(ME_DUMMY(mi->mount_source, mi->filesystem)))
                mi->flags |= MOUNTINFO_IS_DUMMY;

            if(unlikely(ME_REMOTE(mi->mount_source, mi->filesystem)))
                mi->flags |= MOUNTINFO_IS_REMOTE;

            // mark as BIND the duplicates (i.e. same filesystem + same source)
            if(do_statvfs) {
                struct stat buf;
                if(unlikely(stat(mi->mount_point, &buf) == -1)) {
                    mi->st_dev = 0;
                    mi->flags |= MOUNTINFO_NO_STAT;
                }
                else {
                    mi->st_dev = buf.st_dev;

                    struct mountinfo *mt;
                    for(mt = root; mt; mt = mt->next) {
                        if(unlikely(mt->st_dev == mi->st_dev && !(mt->flags & MOUNTINFO_IS_SAME_DEV))) {
                            if(strlen(mi->mount_point) < strlen(mt->mount_point))
                                mt->flags |= MOUNTINFO_IS_SAME_DEV;
                            else
                                mi->flags |= MOUNTINFO_IS_SAME_DEV;
                        }
                    }
                }
            }
            else {
                mi->st_dev = 0;
            }
        }
        else {
            mi->filesystem = NULL;
            mi->filesystem_hash = 0;

            mi->mount_source = NULL;
            mi->mount_source_hash = 0;

            mi->super_options = NULL;

            mi->st_dev = 0;
        }

        // check if it has size
        if(do_statvfs && !(mi->flags & MOUNTINFO_IS_DUMMY)) {
            struct statvfs buff_statvfs;
            if(unlikely(statvfs(mi->mount_point, &buff_statvfs) < 0)) {
                mi->flags |= MOUNTINFO_NO_STAT;
            }
            else if(unlikely(!buff_statvfs.f_blocks /* || !buff_statvfs.f_files */)) {
                mi->flags |= MOUNTINFO_NO_SIZE;
            }
        }

        // link it
        if(unlikely(!root))
            root = mi;
        else
            last->next = mi;

        last = mi;
        mi->next = NULL;

/*
#ifdef NETDATA_INTERNAL_CHECKS
        fprintf(stderr, "MOUNTINFO: %ld %ld %lu:%lu root '%s', persistent id '%s', mount point '%s', mount options '%s', filesystem '%s', mount source '%s', super options '%s'%s%s%s%s%s%s\n",
             mi->id,
             mi->parentid,
             mi->major,
             mi->minor,
             mi->root,
             mi->persistent_id,
                (mi->mount_point)?mi->mount_point:"",
                (mi->mount_options)?mi->mount_options:"",
                (mi->filesystem)?mi->filesystem:"",
                (mi->mount_source)?mi->mount_source:"",
                (mi->super_options)?mi->super_options:"",
                (mi->flags & MOUNTINFO_IS_DUMMY)?" DUMMY":"",
                (mi->flags & MOUNTINFO_IS_BIND)?" BIND":"",
                (mi->flags & MOUNTINFO_IS_REMOTE)?" REMOTE":"",
                (mi->flags & MOUNTINFO_NO_STAT)?" NOSTAT":"",
                (mi->flags & MOUNTINFO_NO_SIZE)?" NOSIZE":"",
                (mi->flags & MOUNTINFO_IS_SAME_DEV)?" SAMEDEV":""
        );
#endif
*/
    }

/* find if the mount options have "bind" in them
    {
        FILE *fp = setmntent(MOUNTED, "r");
        if (fp != NULL) {
            struct mntent mntbuf;
            struct mntent *mnt;
            char buf[4096 + 1];

            while ((mnt = getmntent_r(fp, &mntbuf, buf, 4096))) {
                char *bind = hasmntopt(mnt, "bind");
                if(unlikely(bind)) {
                    struct mountinfo *mi;
                    for(mi = root; mi ; mi = mi->next) {
                        if(unlikely(strcmp(mnt->mnt_dir, mi->mount_point) == 0)) {
                            fprintf(stderr, "Mount point '%s' is BIND\n", mi->mount_point);
                            mi->flags |= MOUNTINFO_IS_BIND;
                            break;
                        }
                    }

#ifdef NETDATA_INTERNAL_CHECKS
                    if(unlikely(!mi)) {
                        error("Mount point '%s' not found in /proc/self/mountinfo", mnt->mnt_dir);
                    }
#endif
                }
            }
            endmntent(fp);
        }
    }
*/

    procfile_close(ff);
    return root;
}
