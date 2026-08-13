// SPDX-License-Identifier: GPL-3.0-or-later

// Tests for fopen_secret_write() (src/libnetdata/paths/paths.c).
//
// The helper exists so that files holding secrets - the claim private key, the
// claim token in cloud.conf, claimed_id, bearer tokens - are never left readable
// or writable by the netdata group. Plain fopen() creates 0666 & ~umask, and the
// daemon runs with umask(0007), which yields 0660.
//
// Every test below runs under umask(0022) on purpose: plain fopen() would then
// produce 0644, so an assertion of 0600 fails unless the helper's own fchmod()
// is doing the work. A permissive umask is what makes these tests able to tell
// the helper apart from fopen().

#include "libnetdata/libnetdata.h"

static int failures = 0;

static int expect(bool condition, const char *msg) {
    if(condition)
        return 0;

    fprintf(stderr, "FAIL: %s\n", msg);
    return 1;
}

static mode_t file_mode(const char *filename) {
    struct stat st;
    if(stat(filename, &st) != 0)
        return (mode_t)-1;

    return st.st_mode & 0777;
}

static bool file_contents_equal(const char *filename, const char *expected) {
    char buf[256] = "";
    if(read_txt_file(filename, buf, sizeof(buf)) != 0)
        return false;

    return strcmp(buf, expected) == 0;
}

// A new file must be created 0600 even though umask(0022) would allow 0644.
static void test_creates_private_file(const char *dir) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, sizeof(filename), "%s/created", dir);

    FILE *fp = fopen_secret_write(filename, "w");
    failures += expect(fp != NULL, "fopen_secret_write() must create a new file");
    if(!fp)
        return;

    fprintf(fp, "secret");
    fclose(fp);

    failures += expect(file_mode(filename) == 0600, "a newly created secret must be mode 0600, not 0644");
    failures += expect(file_contents_equal(filename, "secret"), "a newly created secret must hold what was written");

    // control: plain fopen() under the same umask must NOT be 0600 - this is
    // what proves the assertion above is testing the helper and not the umask.
    char control[FILENAME_MAX + 1];
    snprintfz(control, sizeof(control), "%s/control", dir);
    FILE *cfp = fopen(control, "w");
    if(cfp) {
        fclose(cfp);
        failures += expect(file_mode(control) == 0644,
                           "control: plain fopen() under umask(0022) must be 0644, otherwise these tests prove nothing");
    }
}

// A file an older agent left at 0660 must be tightened, not left as it was.
static void test_repairs_wide_permissions(const char *dir) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, sizeof(filename), "%s/preexisting", dir);

    FILE *fp = fopen(filename, "w");
    failures += expect(fp != NULL, "test setup must create the pre-existing file");
    if(!fp)
        return;
    fprintf(fp, "old");
    fclose(fp);

    failures += expect(chmod(filename, 0660) == 0, "test setup must widen the pre-existing file to 0660");
    failures += expect(file_mode(filename) == 0660, "test setup must leave the file at 0660");

    fp = fopen_secret_write(filename, "w");
    failures += expect(fp != NULL, "fopen_secret_write() must reopen an existing file");
    if(!fp)
        return;

    fprintf(fp, "new");
    fclose(fp);

    failures += expect(file_mode(filename) == 0600, "an existing 0660 secret must be repaired to 0600");
    failures += expect(file_contents_equal(filename, "new"), "reopening must truncate like fopen(.., \"w\")");
}

// Truncation must be complete: a shorter secret must not leave a tail of the
// longer one behind. fdopen() does not truncate, so the helper's explicit
// ftruncate() is what makes this hold.
static void test_truncates_completely(const char *dir) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, sizeof(filename), "%s/truncate", dir);

    FILE *fp = fopen_secret_write(filename, "w");
    if(!fp) {
        failures += expect(false, "test setup must create the file to truncate");
        return;
    }
    fprintf(fp, "a-long-previous-secret");
    fclose(fp);

    fp = fopen_secret_write(filename, "w");
    if(!fp) {
        failures += expect(false, "fopen_secret_write() must reopen for truncation");
        return;
    }
    fprintf(fp, "short");
    fclose(fp);

    failures += expect(file_contents_equal(filename, "short"), "a shorter secret must not leave the previous one's tail");
}

// Failure must be reported as NULL with errno set, and must not create anything.
static void test_reports_failure(const char *dir) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, sizeof(filename), "%s/no-such-directory/file", dir);

    errno = 0;
    FILE *fp = fopen_secret_write(filename, "w");
    failures += expect(fp == NULL, "fopen_secret_write() must fail when the directory does not exist");
    failures += expect(errno == ENOENT, "fopen_secret_write() must leave errno set on failure");
    if(fp)
        fclose(fp);
}

// The secret directories are group-writable (cloud.d is 0770), so a group member
// can plant a symlink where a secret is about to be written. The helper must not
// follow it: neither the target's mode nor its contents may change.
static void test_refuses_symlink(const char *dir) {
    char target[FILENAME_MAX + 1];
    char link[FILENAME_MAX + 1];
    snprintfz(target, sizeof(target), "%s/symlink-target", dir);
    snprintfz(link, sizeof(link), "%s/symlink", dir);

    FILE *fp = fopen(target, "w");
    if(!fp) {
        failures += expect(false, "test setup must create the symlink target");
        return;
    }
    fprintf(fp, "victim");
    fclose(fp);

    if(symlink(target, link) != 0) {
        failures += expect(false, "test setup must create the symlink");
        return;
    }

    fp = fopen_secret_write(link, "w");
    failures += expect(fp == NULL, "fopen_secret_write() must refuse a symlinked secret path");
    if(fp)
        fclose(fp);

    failures += expect(file_mode(target) == 0644, "refusing a symlink must not chmod its target");
    failures += expect(file_contents_equal(target, "victim"), "refusing a symlink must not truncate its target");

    unlink(link);
}

// A planted FIFO would either block the open until the attacker reads, or hand
// them the secret. Both must be refused.
static void test_refuses_fifo(const char *dir) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, sizeof(filename), "%s/fifo", dir);

    if(mkfifo(filename, 0600) != 0) {
        failures += expect(false, "test setup must create the fifo");
        return;
    }

    FILE *fp = fopen_secret_write(filename, "w");
    failures += expect(fp == NULL, "fopen_secret_write() must refuse a non-regular secret path");
    if(fp)
        fclose(fp);

    unlink(filename);
}

int main(void) {
    // Permissive on purpose - see the file header.
    umask(0022);

    char dir[] = "/tmp/nd-fopen-secret-write-XXXXXX";
    if(!mkdtemp(dir)) {
        fprintf(stderr, "FAIL: cannot create temp directory: %s\n", strerror(errno));
        return 1;
    }

    test_creates_private_file(dir);
    test_repairs_wide_permissions(dir);
    test_truncates_completely(dir);
    test_reports_failure(dir);
    test_refuses_symlink(dir);
    test_refuses_fifo(dir);

    // best effort cleanup
    DIR *d = opendir(dir);
    if(d) {
        struct dirent *de;
        char buf[FILENAME_MAX + 1];
        while((de = readdir(d))) {
            if(!strcmp(de->d_name, ".") || !strcmp(de->d_name, ".."))
                continue;
            snprintfz(buf, sizeof(buf), "%s/%s", dir, de->d_name);
            unlink(buf);
        }
        closedir(d);
    }
    rmdir(dir);

    if(failures)
        fprintf(stderr, "\n%d check(s) failed\n", failures);
    else
        fprintf(stderr, "fopen_secret_write() tests passed\n");

    return failures ? 1 : 0;
}
