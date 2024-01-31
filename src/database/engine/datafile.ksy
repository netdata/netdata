meta:
  id: netdata_datafile
  endian: le

seq:
  - id: hdr
    type: header
    size: 4096
  - id: extents
    type: extent
    repeat: eos

types:
  header:
    seq:
      - id: magic
        contents: "netdata-data-file"
      - id: reserved
        size: 15
      - id: version
        contents: "1.0"
      - id: reserved1
        size: 13
      - id: tier
        type: u1
  extent_page_descr:
    seq:
      - id: type
        type: u1
        enum: page_type
      - id: uuid
        size: 16
      - id: page_len
        type: u4
      - id: start_time_ut
        type: u8
      - id: end_time_ut
        type: u8
    enums:
      page_type:
        0: metrics
        1: tier
  extent_header:
    seq:
      - id: payload_length
        type: u4
      - id: compression_algorithm
        type: u1
        enum: compression_algos
      - id: number_of_pages
        type: u1
      - id: page_descriptors
        type: extent_page_descr
        repeat: expr
        repeat-expr: number_of_pages
    enums:
      compression_algos:
        0: rrd_no_compression
        1: rrd_lz4
  extent_trailer:
    seq:
      - id: crc32_checksum
        type: u4
  extent:
    seq:
      - id: header
        type: extent_header
      - id: payload
        size: header.payload_length
      - id: trailer
        type: extent_trailer
      - id: padding
        size: (((_io.pos + 4095) / 4096) * 4096) - _io.pos
        # the extent size is made to always be a multiple of 4096
