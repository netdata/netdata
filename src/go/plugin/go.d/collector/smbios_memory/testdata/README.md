# SMBIOS fixture provenance

`system-memory.bin` is a synthetic SMBIOS 3.3 table with one Type 16 system-memory
array and sixteen Type 17 devices, each 64 GiB, two ranks, DDR4, with rated and
configured speed 3200 MT/s. This reproduces the population and encoding shape of
the decoded Linux server capture supplied for issue #23206. Manufacturer, part,
serials and handles are synthetic. It is not a raw capture from that server.

The wire layout follows DMTF DSP0134 3.8 sections 5.2, 7.17 and 7.18. The SMBIOS 3
entry point advertises a maximum larger than the actual table, which ends with
Type 127. Tests also construct a checksummed SMBIOS 2 entry point, vary documented
size/speed encodings and array classification, and damage lengths and counts.
