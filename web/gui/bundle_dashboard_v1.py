#!/usr/bin/env python3
#
# Copyright: © 2021 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

import os
import shutil
import subprocess
import sys

from pathlib import Path

os.chdir(Path(__file__).parent.absolute())

BASEPATH = Path('v1')

URLTEMPLATE = 'https://github.com/netdata/dashboard/releases/download/{0}/dashboard.tar.gz'

CMAKETEMPLATE = '''
    install(FILES {0} DESTINATION ${{WEB_DEST}})
    install(FILES {1} DESTINATION ${{WEB_DEST}}/css)
    install(FILES {2} DESTINATION ${{WEB_DEST}}/fonts)
    install(FILES {3} DESTINATION ${{WEB_DEST}}/images)
    install(FILES {4} DESTINATION ${{WEB_DEST}}/lib)
    install(FILES {5} DESTINATION ${{WEB_DEST}}/static/css)
    install(FILES {6} DESTINATION ${{WEB_DEST}}/static/js)
    install(FILES {7} DESTINATION ${{WEB_DEST}}/static/media)
    install(FILES web/gui/v1/index.html DESTINATION ${WEB_DEST}/v1)
'''

def copy_dashboard(tag):
    '''Fetch and bundle the dashboard code.'''
    print('Preparing target directory')
    shutil.rmtree(BASEPATH)
    BASEPATH.mkdir()
    print('::group::Fetching dashboard release tarball')
    subprocess.check_call('curl -L -o dashboard.tar.gz ' + URLTEMPLATE.format(tag), shell=True)
    print('::endgroup::')
    print('::group::Extracting dashboard release tarball')
    subprocess.check_call('tar -xvzf dashboard.tar.gz -C ' + str(BASEPATH) + ' --strip-components=1', shell=True)
    print('::endgroup::')
    print('Copying README.md')
    BASEPATH.joinpath('README.md').symlink_to('../.dashboard-notice.md')
    print('Removing dashboard release tarball')
    BASEPATH.joinpath('..', 'dashboard.tar.gz').unlink()


def genfilelist(path):
    '''Generate a list of files for the Makefile.'''
    files = [f for f in path.iterdir() if f.is_file() and f.name != 'README.md']
    files = [Path(*f.parts[1:]) for f in files]
    files.sort()
    return '\n'.join([("web/gui/v1/" + str(f)) for f in files])


def write_cmakefile():
    '''Write out the cmake file for the dashboard code.'''
    print('Generating cmake file')
    output = CMAKETEMPLATE.format(
        genfilelist(BASEPATH),
        genfilelist(BASEPATH.joinpath('css')),
        genfilelist(BASEPATH.joinpath('fonts')),
        genfilelist(BASEPATH.joinpath('images')),
        genfilelist(BASEPATH.joinpath('lib')),
        genfilelist(BASEPATH.joinpath('static', 'css')),
        genfilelist(BASEPATH.joinpath('static', 'js')),
        genfilelist(BASEPATH.joinpath('static', 'media')),
    )

    BASEPATH.joinpath('dashboard_v1.cmake').write_text(output)


def list_changed_files():
    '''Create a list of changed files, and set it in an environment variable.'''
    if 'GITHUB_ENV' in os.environ:
        print('Generating file list for commit.')
        subprocess.check_call('echo "COMMIT_FILES<<EOF" >> $GITHUB_ENV', shell=True)
        subprocess.check_call('git status --porcelain=v1 --no-renames --untracked-files=all | rev | cut -d \' \' -f 1 | rev >> $GITHUB_ENV', shell=True)
        subprocess.check_call('echo "EOF" >> $GITHUB_ENV', shell=True)


copy_dashboard(sys.argv[1])
write_cmakefile()
list_changed_files()
