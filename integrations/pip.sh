#!/bin/sh

# If you change these dependencies, also update the pip install command in
# packaging/cmake/Modules/NetdataRenderDocs.cmake
exec pip install jsonschema referencing jinja2 ruamel.yaml
