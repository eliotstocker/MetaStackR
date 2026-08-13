## [1.4.4](https://github.com/eliotstocker/MetaStackR/compare/core-v1.4.3...core-v1.4.4) (2026-08-13)


### Bug Fixes

* **gitlab:** resolve submodule paths via .gitmodules, route dynamic VCS client, and update pointers immediately on child PR merge ([b596f94](https://github.com/eliotstocker/MetaStackR/commit/b596f944f939bf2725bcc149f7d817a0031113c9))

## [1.4.3](https://github.com/eliotstocker/MetaStackR/compare/core-v1.4.2...core-v1.4.3) (2026-08-13)


### Bug Fixes

* **gitlab:** overwrite initial comment and purge duplicate comments on merge requests ([704dad1](https://github.com/eliotstocker/MetaStackR/commit/704dad19fe3914f1defa6200d96a0dff23f99fe6))

## [1.4.2](https://github.com/eliotstocker/MetaStackR/compare/core-v1.4.1...core-v1.4.2) (2026-08-13)


### Bug Fixes

* **extension:** remove set to auto-merge button from gitlab popout ([d2222f2](https://github.com/eliotstocker/MetaStackR/commit/d2222f234af3a153e177d68ac3761fffd27301a1))

## [1.4.1](https://github.com/eliotstocker/MetaStackR/compare/core-v1.4.0...core-v1.4.1) (2026-08-13)


### Bug Fixes

* **extension:** apply popout style guide strictly to GitLab while preserving GitHub Primer style on GitHub ([de280d3](https://github.com/eliotstocker/MetaStackR/commit/de280d30a91f89588578f0f894892e0accd4bdb2))

# [1.4.0](https://github.com/eliotstocker/MetaStackR/compare/core-v1.3.0...core-v1.4.0) (2026-08-13)


### Bug Fixes

* **extension:** restore matrix table structure and apply screenshot style guide strictly to popped-out policy card ([fed2f83](https://github.com/eliotstocker/MetaStackR/commit/fed2f83dad9a97c48d44e20fef4b6ef9d3f81209))


### Features

* **gitlab:** update auto-merge policy widget to match GitLab native style and enforce MR terminology across GitLab platform ([e84b5ea](https://github.com/eliotstocker/MetaStackR/commit/e84b5ea31797e2da68bd5f599db8d48256422818))

# [1.3.0](https://github.com/eliotstocker/MetaStackR/compare/core-v1.2.1...core-v1.3.0) (2026-08-13)


### Features

* **gitlab:** add dynamic submodule counts to status description and target_url redirect link ([56c07dd](https://github.com/eliotstocker/MetaStackR/commit/56c07dd07529490d5354da67f60c867f8945de94))

## [1.2.1](https://github.com/eliotstocker/MetaStackR/compare/core-v1.2.0...core-v1.2.1) (2026-08-13)


### Bug Fixes

* **gitlab:** assign full markdown matrix table body in EnsureRootPRComment and format GitLab MR URLs ([c27baf4](https://github.com/eliotstocker/MetaStackR/commit/c27baf48043cabb4b3d9fbe20f0f92d7926d3c75))

# [1.2.0](https://github.com/eliotstocker/MetaStackR/compare/core-v1.1.12...core-v1.2.0) (2026-08-13)


### Bug Fixes

* **oauth:** register wildcard path for /oauth/gitlab/login route ([69dd63d](https://github.com/eliotstocker/MetaStackR/commit/69dd63d7b4b12ef421fd6ef2daa9e3e1ecda3265))
* **server:** fix build compilation errors in server.go for Lambda build ([d29a15c](https://github.com/eliotstocker/MetaStackR/commit/d29a15ceaf987c73d851f400c9f6b0021d40e81a))


### Features

* **gitlab:** add GitLab theme matching, URL matching, OAuth token persistence & status re-verification ([81872c9](https://github.com/eliotstocker/MetaStackR/commit/81872c974c2a6ad7cd22cc164a1b6a2bb3f4703e))
* **oauth:** add /oauth/gitlab/login endpoint for automated redirect with configured client_id ([f58d7f3](https://github.com/eliotstocker/MetaStackR/commit/f58d7f3e443231383b38a4bf41bc1fbebddb7570))
* **oauth:** add automatic server-side OAuth token refresh using refresh_token ([839fa6a](https://github.com/eliotstocker/MetaStackR/commit/839fa6ac262bfc32413e4b6000204818d58fd258))
* **oauth:** store user OAuth App tokens in user_vcs_tokens table and strip local CLI token transmission ([b7280b1](https://github.com/eliotstocker/MetaStackR/commit/b7280b1e79012a807f3f91c6762c9253149923f2))

## [1.1.12](https://github.com/eliotstocker/MetaStackR/compare/core-v1.1.11...core-v1.1.12) (2026-08-11)


### Bug Fixes

* **chrome:** fix ReferenceError tabContainer is not defined in click handler ([a701780](https://github.com/eliotstocker/MetaStackR/commit/a701780ea78dc80a0a7e99800cb47f26f9cbaedb))

## [1.1.11](https://github.com/eliotstocker/MetaStackR/compare/core-v1.1.10...core-v1.1.11) (2026-08-11)


### Bug Fixes

* **chrome:** set subTab href to #metastackr and apply gitlab pajama tab styling ([de3c665](https://github.com/eliotstocker/MetaStackR/commit/de3c665525500e327288ecacf041b4e410bc77f0)), closes [#metastackr](https://github.com/eliotstocker/MetaStackR/issues/metastackr)

## [1.1.10](https://github.com/eliotstocker/MetaStackR/compare/core-v1.1.9...core-v1.1.10) (2026-08-11)


### Bug Fixes

* **chrome:** remove redundant tabList click listener causing self-closing race condition ([1085b28](https://github.com/eliotstocker/MetaStackR/commit/1085b28408af5462eb8546c3b292dec6621aeb50))

## [1.1.9](https://github.com/eliotstocker/MetaStackR/compare/core-v1.1.8...core-v1.1.9) (2026-08-11)


### Bug Fixes

* **chrome:** de-select all native li and a tab items when activating metastackr tab ([31a20c4](https://github.com/eliotstocker/MetaStackR/commit/31a20c48161a27231b52236bc3883871d1f18e5b))

## [1.1.8](https://github.com/eliotstocker/MetaStackR/compare/core-v1.1.7...core-v1.1.8) (2026-08-11)


### Bug Fixes

* **chrome:** add gitlab tab class selectors and SPA route watcher for tab exit ([1c434a3](https://github.com/eliotstocker/MetaStackR/commit/1c434a391e4e3fd37b6a531beb456b939842048e))

## [1.1.7](https://github.com/eliotstocker/MetaStackR/compare/core-v1.1.6...core-v1.1.7) (2026-08-11)


### Bug Fixes

* **chrome:** refine gitlab pajamas tab layout and capture-phase tab switching ([1e5e098](https://github.com/eliotstocker/MetaStackR/commit/1e5e098921d09135476f222a77d54157c470bc27))

## [1.1.6](https://github.com/eliotstocker/MetaStackR/compare/core-v1.1.5...core-v1.1.6) (2026-08-11)


### Bug Fixes

* **server:** only post/update MR note and do not modify MR description body ([b704e94](https://github.com/eliotstocker/MetaStackR/commit/b704e94af69e55d8048711460d756ec840e6103b))

## [1.1.5](https://github.com/eliotstocker/MetaStackR/compare/core-v1.1.4...core-v1.1.5) (2026-08-11)


### Bug Fixes

* **server:** match MetaStackr note header string to prevent infinite webhook loop ([5e570c5](https://github.com/eliotstocker/MetaStackR/commit/5e570c569879cb42f36c912d6e413de929ab3fb9))

## [1.1.4](https://github.com/eliotstocker/MetaStackR/compare/core-v1.1.3...core-v1.1.4) (2026-08-11)


### Bug Fixes

* **server:** deduplicate and edit gitlab MR notes in-place via PUT ([081a36c](https://github.com/eliotstocker/MetaStackR/commit/081a36c116a900c19192e3a99e31345916c3a07f))

## [1.1.3](https://github.com/eliotstocker/MetaStackR/compare/core-v1.1.2...core-v1.1.3) (2026-08-11)


### Bug Fixes

* **server:** send Authorization Bearer header for GitLab OAuth tokens ([2ae3db9](https://github.com/eliotstocker/MetaStackR/commit/2ae3db94c92c36a7ed36428d726e3490e70d840c))

## [1.1.2](https://github.com/eliotstocker/MetaStackR/compare/core-v1.1.1...core-v1.1.2) (2026-08-11)


### Bug Fixes

* **server:** use VCSForRepo for gitlab MR comment and check run dispatch ([a90ac61](https://github.com/eliotstocker/MetaStackR/commit/a90ac61e380c2b81ab7deee9ea80e2ca932fa92e))

## [1.1.1](https://github.com/eliotstocker/MetaStackR/compare/core-v1.1.0...core-v1.1.1) (2026-08-11)


### Bug Fixes

* **server:** allow per-repo UUID secret tokens and automated gitlab webhooks ([c8af992](https://github.com/eliotstocker/MetaStackR/commit/c8af992aff64b6b23d6d65a83f26193d5b464956))

# [1.1.0](https://github.com/eliotstocker/MetaStackR/compare/core-v1.0.0...core-v1.1.0) (2026-08-11)


### Features

* **server:** add GET /oauth/gitlab/callback handler for GitLab OAuth flow ([d38dba8](https://github.com/eliotstocker/MetaStackR/commit/d38dba8274398dfbe65e887ba7d542adb08fb25d))

# 1.0.0 (2026-08-11)


### Bug Fixes

* **ci:** set tagFormat to core-v${version} in root .releaserc.json ([4542fa1](https://github.com/eliotstocker/MetaStackR/commit/4542fa175fdf5aee9bb643b74bc84eff8664fd60))


### Features

* **core:** initial MetaStackr CLI engine, landing website, and CI/CD infrastructure ([5eb9234](https://github.com/eliotstocker/MetaStackR/commit/5eb9234bd73560b40a88b1c595a6ccfcce88e20a))
* **extension:** Chrome browser extension for GitHub PR submodule synchronization matrix ([ff190b3](https://github.com/eliotstocker/MetaStackR/commit/ff190b3cf19f61737d426b7e38ea99f45ae3c345))
* **gitlab:** native GitLab support, VCS provider detection, and policy config ([#1](https://github.com/eliotstocker/MetaStackR/issues/1)) ([14a007e](https://github.com/eliotstocker/MetaStackR/commit/14a007e9f8de94620acc4d4fefbc2ac5c1ea817b))
* **policy:** policy rules engine, git meta config CLI, and submodule_changes_only rule ([3d669b9](https://github.com/eliotstocker/MetaStackR/commit/3d669b941d1f3287d08874a234ff7cf09f0b0d79))
* **server:** backend webhook engine, GitHub App authentication, and database persistence ([144cf18](https://github.com/eliotstocker/MetaStackR/commit/144cf1889e3723c4df72ff89b76c1c504d9f8a3c))

# 1.0.0 (2026-08-11)


### Features

* **core:** initial MetaStackr CLI engine, landing website, and CI/CD infrastructure ([5eb9234](https://github.com/eliotstocker/MetaStackR/commit/5eb9234bd73560b40a88b1c595a6ccfcce88e20a))
* **extension:** Chrome browser extension for GitHub PR submodule synchronization matrix ([ff190b3](https://github.com/eliotstocker/MetaStackR/commit/ff190b3cf19f61737d426b7e38ea99f45ae3c345))
* **gitlab:** native GitLab support, VCS provider detection, and policy config ([#1](https://github.com/eliotstocker/MetaStackR/issues/1)) ([14a007e](https://github.com/eliotstocker/MetaStackR/commit/14a007e9f8de94620acc4d4fefbc2ac5c1ea817b))
* **policy:** policy rules engine, git meta config CLI, and submodule_changes_only rule ([3d669b9](https://github.com/eliotstocker/MetaStackR/commit/3d669b941d1f3287d08874a234ff7cf09f0b0d79))
* **server:** backend webhook engine, GitHub App authentication, and database persistence ([144cf18](https://github.com/eliotstocker/MetaStackR/commit/144cf1889e3723c4df72ff89b76c1c504d9f8a3c))

# 1.0.0 (2026-08-09)


### Features

* **core:** initial MetaStackr CLI engine, landing website, and CI/CD infrastructure ([5eb9234](https://github.com/eliotstocker/MetaStackR/commit/5eb9234bd73560b40a88b1c595a6ccfcce88e20a))
* **extension:** Chrome browser extension for GitHub PR submodule synchronization matrix ([ff190b3](https://github.com/eliotstocker/MetaStackR/commit/ff190b3cf19f61737d426b7e38ea99f45ae3c345))
* **policy:** policy rules engine, git meta config CLI, and submodule_changes_only rule ([3d669b9](https://github.com/eliotstocker/MetaStackR/commit/3d669b941d1f3287d08874a234ff7cf09f0b0d79))
* **server:** backend webhook engine, GitHub App authentication, and database persistence ([144cf18](https://github.com/eliotstocker/MetaStackR/commit/144cf1889e3723c4df72ff89b76c1c504d9f8a3c))

## [1.6.1](https://github.com/eliotstocker/MetaStackR/compare/v1.6.0...v1.6.1) (2026-08-08)


### Bug Fixes

* **extension:** surgical DOM updates to prevent policy rules panel from disappearing during polling ([4b07543](https://github.com/eliotstocker/MetaStackR/commit/4b075437b421feaf23dc9fac939ec2fcb100d878))

# [1.6.0](https://github.com/eliotstocker/MetaStackR/compare/v1.5.4...v1.6.0) (2026-08-08)


### Features

* **policy:** add submodule_changes_only auto-merge policy rule defaulting to true ([71d457b](https://github.com/eliotstocker/MetaStackR/commit/71d457ba131f140cd5fb8c9fdfc04c0c50dc8040))

## [1.5.4](https://github.com/eliotstocker/MetaStackR/compare/v1.5.3...v1.5.4) (2026-08-08)


### Bug Fixes

* **server:** ignore EventTypeCheckStatus webhooks to prevent self-reinforcing rate limit loop ([cca0f2e](https://github.com/eliotstocker/MetaStackR/commit/cca0f2e37b32cff4e7565be0799e6ac85489c9b6))

## [1.5.3](https://github.com/eliotstocker/MetaStackR/compare/v1.5.2...v1.5.3) (2026-08-08)


### Bug Fixes

* **server:** parse .gitmodules to map repo full names to relative submodule tree paths ([b9727eb](https://github.com/eliotstocker/MetaStackR/commit/b9727ebe8175230ae96b0a3066a7ae399c45e3c5))

## [1.5.2](https://github.com/eliotstocker/MetaStackR/compare/v1.5.1...v1.5.2) (2026-08-08)


### Bug Fixes

* **worker:** dynamically query submodule main HEADs and treat pointer alignment errors as hard failures ([f518147](https://github.com/eliotstocker/MetaStackR/commit/f51814758d9d68909d4f4933363105d4caf8869d))

## [1.5.1](https://github.com/eliotstocker/MetaStackR/compare/v1.5.0...v1.5.1) (2026-08-08)


### Bug Fixes

* **extension:** add live status re-fetch on tab click and active tab auto-polling ([fbd558a](https://github.com/eliotstocker/MetaStackR/commit/fbd558a39330411e86749fd6013c13b94ccef5c6))

# [1.5.0](https://github.com/eliotstocker/MetaStackR/compare/v1.4.0...v1.5.0) (2026-08-08)


### Features

* **cli:** convert settings command to native git config syntax (git meta config) ([d28b667](https://github.com/eliotstocker/MetaStackR/commit/d28b6675b5b2f987e124f8d7a11c94ddfe6f536c))

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
