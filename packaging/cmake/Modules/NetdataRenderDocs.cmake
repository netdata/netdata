# SPDX-License-Identifier: GPL-3.0-or-later
# Optional target for regenerating integration documentation from metadata.yaml files.
#
# Usage:
#   ninja -C build render-docs
#
# This target is never part of ALL, so it has zero impact on normal builds.
# On first invocation it creates a Python venv in the build directory and
# installs the required packages. Subsequent runs reuse the existing venv.

include_guard()

set(_render_docs_venv_dir "${CMAKE_BINARY_DIR}/_render_docs_venv")
set(_render_docs_venv_stamp "${_render_docs_venv_dir}/.stamp")
set(_render_docs_scripts_dir "${CMAKE_SOURCE_DIR}/integrations")

add_custom_command(
        OUTPUT "${_render_docs_venv_stamp}"
        DEPENDS "${_render_docs_scripts_dir}/pip.sh"
        COMMAND python3 -m venv "${_render_docs_venv_dir}"
        COMMAND "${_render_docs_venv_dir}/bin/pip" install -q jsonschema referencing jinja2 ruamel.yaml
        COMMAND "${CMAKE_COMMAND}" -E touch "${_render_docs_venv_stamp}"
        COMMENT "Creating Python venv for render-docs"
)

add_custom_target(render-docs
        DEPENDS "${_render_docs_venv_stamp}"
        COMMAND "${_render_docs_venv_dir}/bin/python3" "${_render_docs_scripts_dir}/gen_integrations.py"
        COMMAND "${_render_docs_venv_dir}/bin/python3" "${_render_docs_scripts_dir}/gen_docs_integrations.py"
        WORKING_DIRECTORY "${CMAKE_SOURCE_DIR}"
        COMMENT "Generating integration documentation from metadata.yaml files"
)
