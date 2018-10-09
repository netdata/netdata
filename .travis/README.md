# Description of CI build configuration

## Variables needed by travis

- GH_TOKEN - GitHub token with push access to repository
- DOCKER_USERNAME - Username (netdatabot) with write access to docker hub repository
- DOCKER_PASSWORD - Password to docker hub
- encrypted_decb6f6387c4_key - Something to do with package releasing (soon to be deprecated)
- encrypted_decb6f6387c4_iv - Something to do with package releasing (soon to be deprecated)
- OLD_DOCKER_USERNAME - Username used to push images to firehol/netdata # TODO: remove after deprecating that repo
- OLD_DOCKER_PASSWORD - Password used to push images to firehol/netdata # TODO: remove after deprecating that repo

## Stages

### Lint

Stage is executed every time and consists of 4 parallel jobs which check for any deviation from coding conventions.
Checks are performed using:
  - csslint (CSS)
  - eslint (JS)
  - shellcheck (shell scripts)
  - flake8 (python)

Ideally any error should cause whole CI pipeline to fail. Unfortunatelly right now only shellcheck and eslint are 
causing pipeline to fail.

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

### Release

This stage is executed only on "master" brach and allows us to create a new tag just looking at git commit message.
It also has an option to automatically generate changelog based on GitHub labels and sync it with GitHub release.
For the sake of simplicity and to use travis features this stage cannot be integrated with next stage.

### Packages

As a name might suggest, this stage is responsible for creating netdata packages such as:
  - docker images
  - tar repository (soon to be removed)
  - self-extracting package

This stage is separated into 2 parallel jobs. One creating docker images and second one creating github artifacts.
Whole stage is executed only on master branch, but there are also additional parameters assigned to jobs:
  - docker images - execution only on cron job or when there is a tag assigned starting with `v`
  - artifacts - script responsible for creating artifacts doesn't have special conditions, but deployment to GitHub
                releases is done only when there is tag assigned
