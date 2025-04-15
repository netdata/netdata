CC = gcc
CFLAGS = -Wall -g
LDFLAGS = -lm

all: parser

parser: main.o lexer.yy.o parser.tab.o ast.o
	$(CC) $(CFLAGS) -o parser main.o lexer.yy.o parser.tab.o ast.o $(LDFLAGS)

main.o: main.c parser.tab.h
	$(CC) $(CFLAGS) -c main.c

lexer.yy.o: lexer.yy.c
	$(CC) $(CFLAGS) -c lexer.yy.c

parser.tab.o: parser.tab.c
	$(CC) $(CFLAGS) -c parser.tab.c

ast.o: ast.c ast.h
	$(CC) $(CFLAGS) -c ast.c

lexer.yy.c: lexer.l parser.tab.h
	flex -o lexer.yy.c lexer.l

parser.tab.c parser.tab.h: parser.y
	bison -d parser.y

clean:
	rm -f parser *.o lex.yy.c expr.tab.c expr.tab.h
