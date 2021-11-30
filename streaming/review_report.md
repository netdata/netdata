With the current code, I encounter a compilation error in Ubuntu 18.04 (bionic) which led me to a compatibility issue. The conculsion is that the current code is compatible with lz4 v1.9.0+. We need to take care of this in netdata installation.

Some more details below,
Current code lz4 dependencies.
1. `LZ4_resetStream_fast()` introduced in lz4 v1.9.0+
2. `LZ4_decoderRingBufferSize()`, `LZ4_DECODER_RING_BUFFER_SIZE` introduced in lz4 v1.8.2+

Compilation error: 
```
autoreconf -ivf
...
checking for LZ4_compress_default in -llz4... yes
checking for LZ4_resetStream_fast in -llz4... no
...

make
...
  CC       streaming/rrdpush.o
  CC       streaming/compression.o
streaming/compression.c: In function ‘lz4_compressor_reset’:
streaming/compression.c:31:13: warning: implicit declaration of function ‘LZ4_resetStream_fast’; did you mean ‘LZ4_resetStreamState’? [-Wimplicit-function-declaration]
             LZ4_resetStream_fast(state->data->stream);
             ^~~~~~~~~~~~~~~~~~~~
             LZ4_resetStreamState
streaming/compression.c: In function ‘create_compressor’:
streaming/compression.c:110:45: warning: implicit declaration of function ‘LZ4_DECODER_RING_BUFFER_SIZE’; did you mean ‘LZ4_STREAM_BUFFER_SIZE’? [-Wimplicit-function-declaration]
     state->data->stream_buffer = callocz(1, LZ4_DECODER_RING_BUFFER_SIZE(LZ4_MAX_MSG_SIZE));
                                             ^~~~~~~~~~~~~~~~~~~~~~~~~~~~
                                             LZ4_STREAM_BUFFER_SIZE
streaming/compression.c: In function ‘create_decompressor’:
streaming/compression.c:330:39: warning: implicit declaration of function ‘LZ4_decoderRingBufferSize’ [-Wimplicit-function-declaration]
     state->data->stream_buffer_size = LZ4_decoderRingBufferSize(LZ4_MAX_MSG_SIZE);
                                       ^~~~~~~~~~~~~~~~~~~~~~~~~
  CC       streaming/sender.o
  CC       streaming/receiver.o
...
streaming/compression.o: In function `lz4_compressor_reset':
/home/user/test_netdata/streaming/compression.c:31: undefined reference to `LZ4_resetStream_fast'
streaming/compression.o: In function `create_compressor':
/home/user/test_netdata/streaming/compression.c:110: undefined reference to `LZ4_DECODER_RING_BUFFER_SIZE'
streaming/compression.o: In function `create_decompressor':
/home/user/test_netdata/streaming/compression.c:330: undefined reference to `LZ4_decoderRingBufferSize'
collect2: error: ld returned 1 exit status
Makefile:4919: recipe for target 'netdata' failed
make[2]: *** [netdata] Error 1
make[2]: Leaving directory '/home/user/test_netdata'
Makefile:6163: recipe for target 'all-recursive' failed
make[1]: *** [all-recursive] Error 1
make[1]: Leaving directory '/home/user/test_netdata'
Makefile:3221: recipe for target 'all' failed
make: *** [all] Error 2
```

### System details
Kernel: 
```
Linux myserverhostname 4.15.0-161-generic #169-Ubuntu SMP Fri Oct 15 13:41:54 UTC 2021 x86_64 x86_64 x86_64 GNU/Linux
```

OS version:
```
Distributor ID: Ubuntu
Description:    Ubuntu 18.04.6 LTS
Release:        18.04
Codename:       bionic
```

LZ4 version details
```
liblz4-1:
  Installed: 0.0~r131-2ubuntu3.1

/usr/lib/x86_64-linux-gnu/liblz4.so.1.7.1
```


### Compression negotiation in streaming
If hop2 (grandparent) doesn't support compression but hop1 has compression enbaled then hop1 sends compressed data to hop2 which cannot decompress them. So the solution is to negotiate compression between agents during streaming.

```
hop2 (compression enabled but does not support the lz4 v1.9.0+ requirements) <- hop1 (supports compression and has it enbaled) <- hop0_compression, hop0_no_compression

2021-11-29 13:00:56: netdata INFO  : WEB_SERVER[static3] : clients wants to STREAM metrics.
2021-11-29 13:00:56: netdata INFO  : STREAM_RECEIVER[child02_nocompression,[0.0.0.0]:53854] : thread created with task id 10411
2021-11-29 13:00:56: netdata INFO  : STREAM_RECEIVER[child02_nocompression,[0.0.0.0]:53854] : set name of thread 10411 to STREAM_RECEIVER
2021-11-29 13:00:56: netdata INFO  : STREAM_RECEIVER[child02_nocompression,[0.0.0.0]:53854] : STREAM child02_nocompression [0.0.0.0]:53854: receive thread created (task id 10411)
2021-11-29 13:00:56: netdata INFO  : STREAM_RECEIVER[child02_nocompression,[0.0.0.0]:53854] : STREAM child02_nocompression [receive from [0.0.0.0]:53854]: client willing to stream metrics for host 'child02_nocompression' with machine_guid 'd12df8f0-2a7c-11ec-9e79-fa163ec93321': update every = 1, history = 3996, memory mode = ram, health auto, tags ''
2021-11-29 13:00:56: netdata INFO  : STREAM_RECEIVER[child02_nocompression,[0.0.0.0]:53854] : STREAM child02_nocompression [receive from [0.0.0.0]:53854]: initializing communication...
2021-11-29 13:00:56: netdata INFO  : STREAM_RECEIVER[child02_nocompression,[0.0.0.0]:53854] : STREAM child02_nocompression [receive from [0.0.0.0]:53854]: Netdata is using the stream version 4.
2021-11-29 13:00:56: netdata INFO  : STREAM_RECEIVER[child02_nocompression,[0.0.0.0]:53854] : Postponing health checks for 60 seconds, on host 'child02_nocompression', because it was just connected.
2021-11-29 13:00:56: netdata INFO  : STREAM_RECEIVER[child02_nocompression,[0.0.0.0]:53854] : STREAM child02_nocompression [receive from [0.0.0.0]:53854]: receiving metrics...
2021-11-29 13:00:56: netdata ERROR : STREAM_RECEIVER[child01,[0.0.0.0]:53852] : Unknown keyword [�ہ]
2021-11-29 13:00:56: netdata ERROR : STREAM_RECEIVER[child01,[0.0.0.0]:53852] : STREAM child01 [receive from [0.0.0.0]:53852]: disconnected (completed 0 updates).
2021-11-29 13:00:56: netdata INFO  : STREAM_RECEIVER[child01,[0.0.0.0]:53852] : STREAM child01 [receive from [0.0.0.0]:53852]: receive thread ended (task id 10410)
2021-11-29 13:00:56: netdata INFO  : STREAM_RECEIVER[child01,[0.0.0.0]:53852] : thread with task id 10410 finished
2021-11-29 13:00:57: netdata ERROR : STREAM_RECEIVER[child02_nocompression,[0.0.0.0]:53854] : Unknown keyword [���]
2021-11-29 13:00:57: netdata ERROR : STREAM_RECEIVER[child02_nocompression,[0.0.0.0]:53854] : STREAM child02_nocompression [receive from [0.0.0.0]:53854]: disconnected (completed 0 updates).
2021-11-29 13:00:57: netdata INFO  : STREAM_RECEIVER[child02_nocompression,[0.0.0.0]:53854] : STREAM child02_nocompression [receive from [0.0.0.0]:53854]: receive thread ended (task id 10411)
2021-11-29 13:00:57: netdata INFO  : STREAM_RECEIVER[child02_nocompression,[0.0.0.0]:53854] : thread with task id 10411 finished
2021-11-29 13:00:57: netdata INFO  : WEB_SERVER[static2] : clients wants to STREAM metrics.
```