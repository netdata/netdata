Doxygen related files
=====================

`doxygen/` contains all doxygen related files.
This includes
- Doxyfile: The doxygen configuration file.
- Doxygen pages. Markdown files which are added to doxygen output.

## Build doxygen documentation

Doxygen is included into the build chain. To build the default configuration run
```
./autogen.sh && ./configure && make doxygen-doc
```
This builds LaTeX and HTML documentation into:
- `doc/html/index.html`
- `doc/netdata.pdf`

For more advanced usage you can configure doxygen with `./configure`
For example pdf generation can be disabled with `./configure --disable-doxygen-pdf`. 
For a complete list run `./configure --help`.

These build targets are added to `make`:
- doxygen-doc: Generate all doxygen documentation.
- doxygen-run: Run doxygen, which will generate some of the documentation (HTML, CHM, CHI, MAN, RTF, XML) but will not do the post processing required for the rest of it (PS, PDF).
- doxygen-ps: Generate doxygen PostScript documentation.
- doxygen-pdf: Generate doxygen PDF documentation.
