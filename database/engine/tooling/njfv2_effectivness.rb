#!/usr/bin/env ruby
require 'kaitai/struct/struct'
require 'tty-cursor' #gem install tty-cursor
require 'pastel'
require_relative 'generated_parsers/netdata_journalfile_v2.rb'

$nid = ENV['NETDATA_INSTALL_DIR']

fa = []
pdg_percent = []
pdg_total = 0
total_bytes = 0

# Get list of all njfv2 files relative to ENV var NETDATA_INSTALL_DIR or / if not defined
dirs = Dir["#{$nid}/netdata/var/cache/netdata/dbengine*"]
dirs.map! { |d| d + '/*.njfv2'}
dirs.each { |dir|
    fa.concat Dir[dir]
}

def to_mb bytes
    (bytes.to_f / (1024^2)).round(2)
end

p = Pastel.new

puts p.bold "Stats Per File\n=============="
fa.each_with_index { |f, i|
    print "  Processing (#{i}/#{fa.size})\t\"#{f}\""
    $ndjfv2 = NetdataJournalfileV2.from_file(f)
    fsize = $ndjfv2._io.size

    print TTY::Cursor.clear_line

    total_bytes += fsize

    padding = $ndjfv2.padding.size
    STDERR.puts "WARNING Padding contains non 0 bytes \"#{f}\"" if $ndjfv2.padding.bytes.any? {|byte| byte != 0 }
    padding += 4096 - $ndjfv2.journal_v2_header._io.pos

    pdg_total += padding
    pdg_percent.append ((padding.to_f / fsize)*100).round(2)
    print p.bold ("  #{("%0.2f"%pdg_percent.last).rjust(6)} %")
    puts " is padding in \"#{p.italic(f)}\""
}

puts p.bold "\nTotals:\n=============="
puts "  AVG % per file:\t#{(pdg_percent.sum(0.0) / pdg_percent.size).round(2)} %"
puts "  Padding total:\t#{p.bold(to_mb(pdg_total))} MB out of total #{p.bold(to_mb(total_bytes))} MB"
puts "  Padding total:\t#{((to_mb(pdg_total)/to_mb(total_bytes))*100).round(2)} %"
