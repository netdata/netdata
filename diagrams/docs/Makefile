DRAWIO ?= /opt/drawio/drawio
DRAWIO_FLAGS = --export --format svg --transparent --embed-svg-images

SOURCES := $(wildcard *.drawio)
TARGETS := $(patsubst %.drawio,%.svg,$(SOURCES))

.PHONY:
all: svg

.PHONY:
svg: $(TARGETS)

.PHONY:
clean:
	rm $(TARGETS)

$(TARGETS): %.svg: %.drawio
	$(DRAWIO) $(DRAWIO_FLAGS) --output $@ $<
