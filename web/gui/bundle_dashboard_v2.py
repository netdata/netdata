#!/usr/bin/env python3
#
# Copyright: © 2023 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later
'''Bundle the v2 dashboard code into the agent repo.

   This is designed to be run as part of a GHA workflow, but will work fine outside of one.'''

import os
import shutil
import subprocess

from pathlib import Path

os.chdir(Path(__file__).parent.absolute())

BASEDIR = 'v2'

BASEPATH = Path(BASEDIR)

TMPPATH = Path('tmp')

URLSRC = 'https://app.netdata.cloud/agent.tar.gz'

CMAKETEMPLATE = '''
    install(FILES {0} DESTINATION ${{WEB_DEST}}/v2)
    install(FILES {1} DESTINATION ${{WEB_DEST}}/v2/static)
    install(FILES {2} DESTINATION ${{WEB_DEST}}/v2/static/email/img)
    install(FILES {3} DESTINATION ${{WEB_DEST}}/v2/static/img)
    install(FILES {4} DESTINATION ${{WEB_DEST}}/v2/static/img/logos/os)
    install(FILES {5} DESTINATION ${{WEB_DEST}}/v2/static/img/logos/services)
    install(FILES {6} DESTINATION ${{WEB_DEST}}/v2/static/img/mail)
    install(FILES {7} DESTINATION ${{WEB_DEST}}/v2/static/site/pages/holding-page-503)
'''

def copy_dashboard():
    '''Fetch and bundle the dashboard code.'''
    print('Preparing target directory')
    shutil.rmtree(BASEPATH)
    TMPPATH.mkdir()
    print('::group::Fetching dashboard release tarball')
    subprocess.check_call(f'curl -L -o agent.tar { URLSRC }', shell=True)
    print('::endgroup::')
    print('::group::Extracting dashboard release tarball')
    subprocess.check_call(f"tar -xvf agent.tar -C { TMPPATH } --strip-components=1 --exclude='*.br' --exclude='*.gz'", shell=True)
    print('::endgroup::')
    print('Copying files')
    (TMPPATH / 'agent' / BASEDIR).rename(BASEPATH)
    (TMPPATH / 'agent' / 'index.html').rename(Path('./index.html'))
    (TMPPATH / 'agent' / 'registry-access.html').rename('./registry-access.html')
    (TMPPATH / 'agent' / 'registry-alert-redirect.html').rename('./registry-alert-redirect.html')
    (TMPPATH / 'agent' / 'registry-hello.html').rename('./registry-hello.html')
    shutil.copytree(TMPPATH / 'agent' / 'static', Path('./static'), dirs_exist_ok=True)
    shutil.rmtree(TMPPATH)
    print('Copying README.md')
    BASEPATH.joinpath('README.md').symlink_to('../.dashboard-v2-notice.md')
    print('Removing dashboard release tarball')
    BASEPATH.joinpath('..', 'agent.tar').unlink()


def genfilelist(path):
    '''Generate a list of files for the Makefile.'''
    files = [f for f in path.iterdir() if f.is_file() and f.name != 'README.md']
    files = [Path(*f.parts[1:]) for f in files]
    files.sort()
    return '\n'.join([("web/gui/v2/" + str(f)) for f in files])


def write_cmakefile():
    '''Write out the makefile for the dashboard code.'''
    print('Generating cmake file')
    output = CMAKETEMPLATE.format(
        genfilelist(BASEPATH),
        genfilelist(BASEPATH.joinpath('static')),
        genfilelist(BASEPATH.joinpath('static', 'email', 'img')),
        genfilelist(BASEPATH.joinpath('static', 'img')),
        genfilelist(BASEPATH.joinpath('static', 'img', 'logos', 'os')),
        genfilelist(BASEPATH.joinpath('static', 'img', 'logos', 'services')),
        genfilelist(BASEPATH.joinpath('static', 'img', 'mail')),
        genfilelist(BASEPATH.joinpath('static', 'site', 'pages', 'holding-page-503')),
    )

    BASEPATH.joinpath('dashboard_v2.cmake').write_text(output)


def list_changed_files():
    '''Create a list of changed files, and set it in an environment variable.'''
    if 'GITHUB_ENV' in os.environ:
        print('Generating file list for commit.')
        subprocess.check_call('echo "COMMIT_FILES<<EOF" >> $GITHUB_ENV', shell=True)
        subprocess.check_call('git status --porcelain=v1 --no-renames --untracked-files=all | rev | cut -d \' \' -f 1 | rev >> $GITHUB_ENV', shell=True)
        subprocess.check_call('echo "EOF" >> $GITHUB_ENV', shell=True)


copy_dashboard()
write_cmakefile()
list_changed_files()
