# Description of CI build configuration

## Variables needed by travis

- GITHUB_TOKEN - GitHub token with push access to repository
- DOCKER_USERNAME - Username (netdatabot) with write access to docker hub repository
- DOCKER_PWD - Password to docker hub
- encrypted_8daf19481253_key - key needed by openssl to decrypt GCS credentials file
- encrypted_8daf19481253_iv - IV needed by openssl to decrypt GCS credentials file
- COVERITY_SCAN_TOKEN - Token to allow coverity test analysis uploads
- SLACK_USERNAME - This is required for the slack notifications triggered by travis pipeline
- SLACK_CHANNEL - This is the channel that Travis will be posting messages
- SLACK_NOTIFY_WEBHOOK_URL - This is the incoming URL webhook as provided by slack integration. Visit Apps integration in slack to generate the required hook
- SLACK_BOT_NAME - This is the name your bot will appear with on slack

## CI workflow details
Our CI pipeline is designed to help us identify and mitigate risks at all stages of implementation.
To accommodate this need, we used [Travis CI](http://www.travis-ci.com) as our CI/CD tool.
Our main areas of concern are:
1) Only push code that is working. That means fail fast so that we can improve before we reach the public

2) Reduce the time to market to minimum, by streamlining the release process.
   That means a lot of testing, a lot of consistency checks, a lot of validations

3) Generated artifacts consistency. We should not allow broken software to reach the public.
   When this happens, it's embarassing and we struggle to eliminate it.

4) We are an innovative company, so we love to automate :)


Having said that, here's a brief introduction to Netdata's improved CI/CD pipeline with Travis.
Our CI/CD lifecycle contains three different execution entry points:
1) A user opens a pull request to netdata/master: Travis will run a pipeline on the branch under that PR
2) A merge or commit happens on netdata/master. This will trigger travis to run, but we have two distinct cases in this scenario:
   a) A user merges a pull request to netdata/master: Travis will run on master, after the merge.
   b) A user runs a commit/merge with a special keyword (mentioned later).
      This triggers a release for either minor, major or release candidate versions, depending the keyword
3) A scheduled job runs on master once per day: Travis will run on master at the scheduled interval

To accommodate all three entry points our CI/CD workflow has a set of steps that run on all three entry points.
Once all these steps are successfull, then our pipeline executes another subset of steps for entry points 2 and 3.
In travis terms the "steps" are "Stages" and within each stage we execute a set of activities called "jobs" in travis.

### Always run: Stages that running on all three execution entry points

## Code quality, linting, syntax, code style
At this early stage we iterate through a set of basic quality control checks:
- Shell checking: Run linters for our various BASH scripts
- Checksum validators: Run validators to ensure our installers and documentation are in sync
- Dashboard validator: We provide a pre-generated dashboard.js script file that we need to make sure its up to date. We validate that.

## Build process
At this stage, basically, we build :-)
We do a baseline check of our build artifacts to guarantee they are not broken
Briefly our activities include:
- Verify docker builds successfully
- Run the standard netdata installer, to make sure we build & run properly
- Do the same through 'make dist', as this is our stable channel for our kickstart files

## Artifacts validation
At this point we know our software is building, we need to go through the a set of checks, to guarantee
that our product meets certain epxectations. At the current stage, we are focusing on basic capabilities
like installing in different distributions, running the full lifecycle of install-run-update-install and so on.
We are still working on enriching this with more and more use cases, to get us closer to achieving full stability of our software.
Briefly we currently evaluate the following activities:
- Basic software unit testing
- Non containerized build and install on ubuntu 14.04
- Non containerized build and install on ubuntu 18.04
- Running the full netdata lifecycle (install, update, uninstall) on ubuntu 18.04
- Build and install on CentOS 6
- Build and install on CentOS 7
(More to come)

### Nightly operations: Stages that run daily under cronjob
The nightly stages are related to the daily nightly activities, that produce our daily latest releases.
We also maintain a couple of cronjobs that run during the night to provide us with deeper insights,
like for example coverity scanning or extended kickstart checksum checks

## Nightly operations
At this stage we run scheduled jobs and execute the nightly changelog generator, coverity scans,
labeler for our issues and extended kickstart files checksum validations.

## Nightly release
During this stage we are building and publishing latest docker images, prepare the nightly artifacts
and deploy them (the artifacts) to our google cloud service provider.


### Publishing
Publishing is responsible for executing the major/minor/patch releases and is separated
in two stages: packaging preparation process and publishing.

## Packaging for release
During packaging we are preparing the release changelog information and run the labeler.

## Publish for release
The publishing stage is the most complex part in publishing. This is the stage were we generate and publish docker images,
prepare the release artifacts and get ready with the release draft.

### Package Management workflows
As part of our goal to provide the best support to our customers, we have created a set of CI workflows to automatically produce
DEB and RPM for multiple distributions. These workflows are implemented under the templated stages '_DEB_TEMPLATE' and '_RPM_TEMPLATE'.
We currently plan to actively support the following Operating Systems, with a plan to further expand this list following our users needs.

### Operating systems supported
The following distributions are supported
- Debian versions
  - Buster (TBD - not released yet, check [debian releases](https://www.debian.org/releases/) for details)
  - Stretch
  - Jessie
  - Wheezy

- Ubuntu versions
  - Disco
  - Cosmic
  - Bionic
  - artful

- Enterprise Linux versions (Covers Redhat, CentOS, and Amazon Linux with version 6)
  - Version 8 (TBD)
  - Version 7
  - Version 6

- Fedora versions
  - Version 31 (TBD)
  - Version 30
  - Version 29
  - Version 28

- OpenSuSE versions
  - 15.1
  - 15.0

- Gentoo distributions
  - TBD

### Architectures supported
We plan to support amd64, x86 and arm64 architectures. As of June 2019 only amd64 and x86 will become available, as we are still working on solving issues with the architecture.

The Package deployment can be triggered manually by executing an empty commit with the following message pattern: `[Package PACKAGE_TYPE PACKAGE_ARCH] DESCRIBE_THE_REASONING_HERE`.
Travis Yaml configuration allows the user to combine package type and architecture as necessary to regenerate the current stable release (For example tag v1.15.0 as of 4th of May 2019)
Sample patterns to trigger building of packages for all AMD64 supported architecture:
- '[Package AMD64 RPM]': Build & publish all amd64 available RPM packages
- '[Package AMD64 DEB]': Build & publish all amd64 available DEB packages
