all: libringbuffer.a libringbuffer.so build/ringbuffer_test.o
	gcc -o test build/ringbuffer_test.o libringbuffer.a

libringbuffer.so: build/ringbuffer.o
	gcc -shared -o libringbuffer.so build/ringbuffer.o

libringbuffer.a: build/ringbuffer.o
	ar rcs libringbuffer.a build/ringbuffer.o

build/ringbuffer.o: src/ringbuffer.c src/ringbuffer_internal.h include/ringbuffer.h
	gcc -o build/ringbuffer.o -c src/ringbuffer.c -Iinclude -fPIC

build/ringbuffer_test.o: libringbuffer.a tests/ringbuffer_test.c
	gcc -o build/ringbuffer_test.o -c tests/ringbuffer_test.c -Iinclude

clean:
	rm -f build/*
	rm -f libringbuffer.a
	rm -f libringbuffer.so
	rm -f test
