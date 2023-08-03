#include <errno.h>
#include <string.h>
#include <stdio.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/mman.h>
#include <sys/types.h>

#define _GNU_SOURCE         /* See feature_test_macros(7) */
#define __USE_GNU
#include <fcntl.h>
#include <unistd.h>

void test_sync_file_range(char *output, char *text, size_t length)
{
    int fd = open (output, O_WRONLY | O_CREAT | O_APPEND, 0660);
    if (fd < 0 ) {
        perror("Cannot get page size");
        return;
    }

    int i;
    size_t offset = 0;
    for ( i = 0 ; i < 10000; i++ )  {
        write(fd, text, length);
        sync_file_range(fd, offset, length, SYNC_FILE_RANGE_WRITE);
        offset += length;
    }

    close(fd);
    sleep(5);
}

// test based on IBM example https://www.ibm.com/support/knowledgecenter/en/ssw_ibm_i_71/apis/msync.htm
void test_msync(char *output, char *text, size_t length)
{
    int pagesize = sysconf(_SC_PAGE_SIZE);
    if (pagesize < 0) {
        perror("Cannot get page size");
        return;
    }

    int fd = open(output, (O_CREAT | O_TRUNC | O_RDWR), (S_IRWXU | S_IRWXG | S_IRWXO));
    if (fd < 0 ) {
        perror("Cannot open file");
        return;
    }

    off_t lastoffset = lseek( fd, pagesize, SEEK_SET);
    ssize_t written = write(fd, " ", 1);
    if ( written != 1 ) {
        perror("Write error. ");
        close(fd);
        return;
    }

    off_t my_offset = 0;
    void *address = mmap(NULL, pagesize, PROT_WRITE, MAP_SHARED, fd, my_offset);

    if ( address == MAP_FAILED ) {
        perror("Map error. ");
        close(fd);
        return;
    }

    (void) strcpy( (char*) address, text);

    if ( msync( address, pagesize, MS_SYNC) < 0 ) {
        perror("msync failed with error:");
    }

    close(fd);
    sleep(5);
}

void test_synchronization(char *output, char *text, size_t length, int (*fcnt)(int))
{
    int fd = open (output, O_WRONLY | O_CREAT | O_APPEND, 0660);
    if (fd < 0 ) {
        perror("Cannot get page size");
        return;
    }

    int i;
    for ( i = 0 ; i < 10000; i++ ) 
        write(fd, text, length);

    fcnt(fd);
    close(fd);

    sleep(5);
}

void remove_files(char **files) {
    size_t i = 0;
    while (files[i])  {
        unlink(files[i]);
        i++;
    }
}

int main()
{
    char *default_text = { "This is a simple example to test a PR. The sleep is used to create different peaks on charts.\n" };
    char *files[] = { "fsync.txt", "fdatasync.txt", "syncfs.txt", "msync.txt", "sync_file_range.txt", NULL }; 
    size_t length = strlen(default_text);
    test_synchronization(files[0], default_text, length, fsync);
    test_synchronization(files[1], default_text, length, fdatasync);
    test_synchronization(files[2], default_text, length, syncfs);

    test_msync(files[3], default_text, length);

    test_sync_file_range(files[4], default_text, length);

    sync();

    remove_files(files);

    return 0;
}
