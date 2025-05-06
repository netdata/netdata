# Makefile for re2c/lemon parser generator

# Check if we have re2c and lemon in the system
HAVE_RE2C := $(shell command -v re2c 2>/dev/null)
HAVE_LEMON := $(shell command -v lemon 2>/dev/null)

# Variables for lemon if we need to download it
LEMON_C = lemon.c
LEMPAR_C = lempar.c
LEMON_URL = https://raw.githubusercontent.com/sqlite/sqlite/master/tool/lemon.c
LEMPAR_URL = https://raw.githubusercontent.com/sqlite/sqlite/master/tool/lempar.c

all: lexer.c parser.c parser.h

# Generate lexer from re2c file
lexer.c: lexer.re
ifndef HAVE_RE2C
	$(error re2c is not installed. Please install re2c to build this project)
else
	re2c -o $@ $<
endif

# Generate lemon parser from grammar
parser.c parser.h: parser.y $(LEMPAR_C)
ifdef HAVE_LEMON
	lemon -s -T$(LEMPAR_C) $<
else
	$(CC) -o lemon $(LEMON_C)
	./lemon -s -T$(LEMPAR_C) $<
endif
	@if [ ! -f parser.c ]; then \
		echo "Error: parser.c was not generated"; \
		exit 1; \
	fi

# Download lemon sources if needed
$(LEMON_C):
ifndef HAVE_LEMON
	@echo "Downloading lemon source..."
	curl -s -o $(LEMON_C) $(LEMON_URL)
endif

# Download lempar.c template
$(LEMPAR_C):
	@echo "Downloading lempar.c template..."
	curl -s -o $(LEMPAR_C) $(LEMPAR_URL)

# Clean rule
clean:
	rm -f lexer.c parser.c parser.h parser.out lemon $(LEMON_C) $(LEMPAR_C)

.PHONY: all clean
