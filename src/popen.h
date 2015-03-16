#include <stdio.h>
#include <unistd.h>
#include <sys/types.h>

#ifndef NETDATA_POPEN_H
#define NETDATA_POPEN_H 1

#define PIPE_READ 0
#define PIPE_WRITE 1

extern FILE *mypopen(const char *command, pid_t *pidptr);
extern void mypclose(FILE *fp);
extern void process_childs(int wait);

#endif /* NETDATA_POPEN_H */
