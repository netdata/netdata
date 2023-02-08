ksy_compiler = kaitai-struct-compiler

target_dir = generated_parsers

targets = $(target_dir)/netdata_datafile.rb $(target_dir)/netdata_journalfile_v2.rb $(target_dir)/netdata_datafile.py $(target_dir)/netdata_journalfile_v2.py

all: $(targets)

$(target_dir)/netdata_%.rb: ../%.ksy
	$(ksy_compiler) --outdir $(target_dir) -t ruby $<

$(target_dir)/netdata_%.py: ../%.ksy
	$(ksy_compiler) --outdir $(target_dir) -t python $<

clean:
	rm -f $(targets)
