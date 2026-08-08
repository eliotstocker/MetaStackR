# [1.4.0](https://github.com/eliotstocker/MetaStackR/compare/v1.3.0...v1.4.0) (2026-08-08)


### Features

* **cli:** add git-meta settings command to inspect and update policy rules ([7e71241](https://github.com/eliotstocker/MetaStackR/commit/7e71241f5ca7f68ba43d18e2648a9042003ecef1))

# [1.3.0](https://github.com/eliotstocker/MetaStackR/compare/v1.2.3...v1.3.0) (2026-08-08)


### Features

* add auto-merge policy rules, token caching, pointer alignment, and settings UI ([22197d3](https://github.com/eliotstocker/MetaStackR/commit/22197d3e5146fc4ab9cdb4911513ba97222e77d7))

## [1.2.3](https://github.com/eliotstocker/MetaStackR/compare/v1.2.2...v1.2.3) (2026-08-05)


### Bug Fixes

* lots of extension tweaks and more ([7c49b35](https://github.com/eliotstocker/MetaStackR/commit/7c49b3501e3e432c089dade4543cd40e44e2c43f))

## [1.2.2](https://github.com/eliotstocker/MetaStackR/compare/v1.2.1...v1.2.2) (2026-08-05)


### Bug Fixes

* **extension:** insert MetaStackr panel directly before active tab bucket below tab bar ([9602e0c](https://github.com/eliotstocker/MetaStackR/commit/9602e0c3f57550ff3171486b1de73f887545eaba))
* **extension:** position MetaStackr panel inside Layout-main below PR header ([52baf21](https://github.com/eliotstocker/MetaStackR/commit/52baf219717c8b25f86af8fb97c808c5f85f4974))
* **extension:** support MetaStackr tab and panel across all PR sub-pages (Files, Commits, Checks, Conversation) ([5bad438](https://github.com/eliotstocker/MetaStackR/commit/5bad438634f51d9bf1ec3038f7cc2933dc67d5f5))
* **extension:** target turbo-frame below header for clean tab navigation and matrix rendering ([d708d33](https://github.com/eliotstocker/MetaStackR/commit/d708d33c26c3eb05e72e8b41fe0494ec39760d62))

## [1.2.1](https://github.com/eliotstocker/MetaStackR/compare/v1.2.0...v1.2.1) (2026-08-05)


### Bug Fixes

* **extension:** query status API with PR number and remove 3-item hardcoded fallback ([c3cc96f](https://github.com/eliotstocker/MetaStackR/commit/c3cc96f70452564cd80ea2577682b8f9e90c5000))

# [1.2.0](https://github.com/eliotstocker/MetaStackR/compare/v1.1.5...v1.2.0) (2026-08-05)


### Features

* **db,server:** add head_sha column to meta_prs and dynamic installation ID lookup ([f18f0dc](https://github.com/eliotstocker/MetaStackR/commit/f18f0dcb8bf1d03654d31444a480fad3907559b8))

## [1.1.5](https://github.com/eliotstocker/MetaStackR/compare/v1.1.4...v1.1.5) (2026-08-05)


### Bug Fixes

* **server:** resolve installation ID via GitHub App JWT when zero ([2a02405](https://github.com/eliotstocker/MetaStackR/commit/2a024056be7ff8d7c824777252b11c1d944fdaa3))

## [1.1.4](https://github.com/eliotstocker/MetaStackR/compare/v1.1.3...v1.1.4) (2026-08-05)


### Bug Fixes

* **server:** dynamically fetch and save parent MetaPR HeadSHA from GitHub API if empty ([92a07f8](https://github.com/eliotstocker/MetaStackR/commit/92a07f88461b8408127b488df98a341692e7fd63))

## [1.1.3](https://github.com/eliotstocker/MetaStackR/compare/v1.1.2...v1.1.3) (2026-08-05)


### Bug Fixes

* **server:** refresh child PRs after synthesis and trigger check run update on initial child creation ([3413d02](https://github.com/eliotstocker/MetaStackR/commit/3413d0259fe978e5a76cbae8094375718a65b0a0))

## [1.1.2](https://github.com/eliotstocker/MetaStackR/compare/v1.1.1...v1.1.2) (2026-08-04)


### Bug Fixes

* **server:** associate parent meta-repo HeadSHA with MetaPR to fix HTTP 422 on check runs ([8361780](https://github.com/eliotstocker/MetaStackR/commit/83617809e0e12ea850c1aaffd3af2b3d468ee14f))

## [1.1.1](https://github.com/eliotstocker/MetaStackR/compare/v1.1.0...v1.1.1) (2026-08-04)


### Bug Fixes

* **server:** log signature mismatch warning instead of dropping webhooks with 401 ([6a7587a](https://github.com/eliotstocker/MetaStackR/commit/6a7587a8a2643c5e5f56d8b94b13e6ed7b793459))

# [1.1.0](https://github.com/eliotstocker/MetaStackR/compare/v1.0.1...v1.1.0) (2026-08-04)


### Features

* **core:** implement S2S GitHub App authentication and migrate infrastructure to eu-west-1 ([c1c5d8f](https://github.com/eliotstocker/MetaStackR/commit/c1c5d8f98894afb6391db7a5b8b0c6faf8a660a0))
* **core:** include unit test and extension background script ([31ff072](https://github.com/eliotstocker/MetaStackR/commit/31ff072a61e4da038a6727901b453504842f6cd2))

## [1.0.1](https://github.com/eliotstocker/MetaStackR/compare/v1.0.0...v1.0.1) (2026-08-04)


### Bug Fixes

* **ci:** configure S3 remote backend for terraform state ([eb6f03c](https://github.com/eliotstocker/MetaStackR/commit/eb6f03cd5f954afaea2998161e5bd38087518289))

# 1.0.0 (2026-08-04)


### Bug Fixes

* **ci:** track package-lock.json and upgrade node version to 22 in github actions ([70e7da2](https://github.com/eliotstocker/MetaStackR/commit/70e7da26c5cdefa8dfd59ca44175b7593f7a2337))
* **ci:** unignore cmd directory in .gitignore and track Go source files ([d84aec2](https://github.com/eliotstocker/MetaStackR/commit/d84aec256f124e685de3a66d62d2785e64093652))
* **gitutils:** handle GitHub API 422 validation error gracefully during re-init ([d74556b](https://github.com/eliotstocker/MetaStackR/commit/d74556b5b621314f0e3946c351cad7c4f81d9660))


### Features

* **cli:** display gradient ASCII logo on help and version commands ([1cc03b4](https://github.com/eliotstocker/MetaStackR/commit/1cc03b4d4a4f513a6f31da6c8085810cce75b0fd))
* **cli:** dynamically format help and version usage as 'git meta' when executed via git ([057f3e9](https://github.com/eliotstocker/MetaStackR/commit/057f3e9991ce8369cd898c5c8ef0128c6aee6cae))
* **cli:** match ASCII logo color gradient with website theme ([#00](https://github.com/eliotstocker/MetaStackR/issues/00)FF88 to #FACC15) ([f00541a](https://github.com/eliotstocker/MetaStackR/commit/f00541adc15bcdb71bc4a5a620a3213eafed24d6)), closes [#00FF88](https://github.com/eliotstocker/MetaStackR/issues/00FF88) [#FACC15](https://github.com/eliotstocker/MetaStackR/issues/FACC15)
* **cli:** update ASCII logo to match website artwork 1:1 ([89ef18f](https://github.com/eliotstocker/MetaStackR/commit/89ef18fee6e637c57804c7b0c9b33d44ae271cde))
* complete MetaStackr core orchestration, CI/CD, and pages DNS ([c58774b](https://github.com/eliotstocker/MetaStackR/commit/c58774b31680f4fbb24b79db781863f8c40d7a74))
* **core:** setup multi-component semantic release and install script ([d585a55](https://github.com/eliotstocker/MetaStackR/commit/d585a55d602d288fd638355f17b0e1982fafa2a9))
* **web:** add install.sh to website and feature one-liner installer in hero section ([41364e0](https://github.com/eliotstocker/MetaStackR/commit/41364e0dd2c20db6e2e8730ad4a7e54256724e7c))
