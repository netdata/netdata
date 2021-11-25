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