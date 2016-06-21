#!/bin/bash

# FIXME
# 1. make sure you are in the proper directory
# 2. In fedora 24, the python modules names are not known

PIP=`which pip`

${PIP} install --target="python_modules" yaml

${PIP} install --target="python_modules" MySQL-python || echo "You need to install libmysqlclient-dev and python-dev"
