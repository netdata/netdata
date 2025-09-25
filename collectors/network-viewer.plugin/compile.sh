#!/usr/bin/env bash

# stop on errors
set -e

# generate vmlinux.h
bpftool btf dump file /sys/kernel/btf/vmlinux format c > vmlinux.h

# compile the ebpf program
clang -O2 -Wall -Wextra -target bpf -D__TARGET_ARCH_x86 -I/usr/include/bpf -g -c network-viewer.bpf.c -o network-viewer.bpf.o

# compile the loader program
gcc -o network-viewer.plugin network-viewer.c -I../ -I../../ -lbpf -lelf

# allow the program to run
sudo chmod 4750 network-viewer.plugin
sudo setcap cap_bpf,cap_sys_ptrace,cap_sys_admin=eip network-viewer.plugin
