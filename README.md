# c-rbuf [![run-tests](https://github.com/underhood/c-rbuf/workflows/run-tests/badge.svg)](https://github.com/underhood/c-rbuf/actions)

Simple C ringbuffer implementation with API allowing usage without intermediate buffer. Meant to be as simple as possible to use and integrate. You can copy the files into your project, use it as a static lib, or shared lib depending on what is most convenient for you.

Usage without intermediate buffer means when reading from file descriptor/socket you can insert data directly into the ringbuffer:
```C
char *ptr;
size_t bytes;
rbuf_t buff = rbuf_create(12345);
ptr = rbuf_get_linear_insert_range(buff, &ret);
read(fd, ptr, bytes);
```

**Multiple tail support:** Only single tail is supported but multiple tail support might be implemented in the future.

**Thread Safety:** Currently you will have to ensure only a single thread accesses the buffer at a time yourself.

## License

The Project is released under LGPL v3 license. See [License](LICENSE)
