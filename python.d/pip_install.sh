#!/bin/bash

PIP=`which pip`

${PIP} install --target="python_modules" yaml

${PIP} install --target="python_modules" MySQL-python || echo "You need to install libmysqlclient-dev and python-dev"
