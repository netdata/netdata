# Description of CI build configuration

## Variables needed by travis

- GITHUB_TOKEN - GitHub token with push access to repository
- DOCKER_USERNAME - Username (netdatabot) with write access to docker hub repository
- DOCKER_PASSWORD - Password to docker hub
- encrypted_decb6f6387c4_key - Something to do with package releasing (soon to be deprecated)
- encrypted_decb6f6387c4_iv - Something to do with package releasing (soon to be deprecated)
- OLD_DOCKER_USERNAME - Username used to push images to firehol/netdata # TODO: remove after deprecating that repo
- OLD_DOCKER_PASSWORD - Password used to push images to firehol/netdata # TODO: remove after deprecating that repo

## Stages

### Test

Unit tests and coverage tests are executed here. Stage consists of 2 parallel jobs:
  - C tests - executed every time
  - coverity test - executed only when pipeline was triggered from cron

### Build

Stage is executed every time and consists of 5 parallel jobs which execute containerized and non-containerized
installations of netdata. Jobs are run on following operating systems:
  - OSX
  - ubuntu 14.04
  - ubuntu 16.04 (containerized)
  - CentOS 6 (containerized)
  - CentOS 7 (containerized)
  - alpine (containerized)

### Packaging

This stage is executed only on "master" brach and allows us to create a new tag just looking at git commit message.
It executes one script called `releaser.sh` which is responsible for creating a release on GitHub by using
[hub](https://github.com/github/hub). This script is also executing other scripts which can also be used in other
CI jobs:
  - `tagger.sh`
  - `generate_changelog.sh`
  - `build.sh`
  - `create_artifacts.sh`

Alternatively new release can be also created by pushing new tag to master branch.

##### tagger.sh

Script responsible to find out what will be the next tag based on a keyword in last commit message. Keywords are:
 - `[netdata patch release]` to bump patch number
 - `[netdata minor release]` to bump minor number
 - `[netdata major release]` to bump major number
 - `[netdata release candidate]` to create a new release candidate (appends or modifies suffix `-rcX` of previous tag)
All keywords MUST be surrounded with square brackets.
Tag is then stored in `GIT_TAG` variable.

##### generate_changelog.sh

Automatic changelog generator which updates our CHANGELOG.md file based on GitHub features (mostly labels and pull
requests). Internally it uses
[github-changelog-generator](https://github.com/github-changelog-generator/github-changelog-generator) and more
information can be found on that project site.

##### build.sh and create_artifacts.sh

Scripts used to build new container images and provide release artifacts (tar.gz and makeself archives)

### Nightlies

##### Tarball and self-extractor build AND Nightly docker images

As names might suggest those two jobs are responsible for nightly netdata package creation and are run every day (in
cron). Combined they produce:
  - docker images
  - tar.gz archive (soon to be removed)
  - self-extracting package

This is achieved by running 2 scripts described earlier:
  - `create_artifacts.sh`
  - `build.sh`

##### Changelog generation

This job is responsible for regenerating changelog every day by executing `generate_changelog.sh` script. This is done
only once a day due to github rate limiter.

##### Labeler

Once a day we are doing automatic label assignment by executing `labeler.sh`. This script is a temporary workaround until
we start using GitHub Actions. For more information what it is currently doing go to its code.
