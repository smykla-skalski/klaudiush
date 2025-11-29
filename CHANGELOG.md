# Changelog

All notable changes to klaudiush will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.5.0](https://github.com/smykla-labs/klaudiush/compare/v1.4.0...v1.5.0) (2025-11-29)

### Features

* **exceptions:** add bypass workflow ([#92](https://github.com/smykla-labs/klaudiush/issues/92)) ([cb2a575](https://github.com/smykla-labs/klaudiush/commit/cb2a5755eb15ec7b812cc36826fbd07410fbfd88))

### Bug Fixes

* **git:** clear GIT_INDEX_FILE in test isolation ([#90](https://github.com/smykla-labs/klaudiush/issues/90)) ([1bfa84d](https://github.com/smykla-labs/klaudiush/commit/1bfa84d73493da872e6ad43c2e81b0cbab96fdf0))
* **git:** isolate tests from parent worktree ([#89](https://github.com/smykla-labs/klaudiush/issues/89)) ([5175c4c](https://github.com/smykla-labs/klaudiush/commit/5175c4c5cf7c6e3e7ddbce77e085b67f1d851aeb))
* **git:** support worktrees in DiscoverRepository ([#83](https://github.com/smykla-labs/klaudiush/issues/83)) ([30e7db1](https://github.com/smykla-labs/klaudiush/commit/30e7db1706b2c8c01d66748ce5847cdbaf4ebe93))
* **markdown:** disable MD013 for .md/.mdx files ([#81](https://github.com/smykla-labs/klaudiush/issues/81)) ([abd1139](https://github.com/smykla-labs/klaudiush/commit/abd113920111a19f0d342e2dbd190ff473219d74))
* **shellscript:** apply config exclude_rules ([#87](https://github.com/smykla-labs/klaudiush/issues/87)) ([c14252c](https://github.com/smykla-labs/klaudiush/commit/c14252c30459a594ea0701ebde42d064bf55de16))
* **shellscript:** handle fragment edits correctly ([#86](https://github.com/smykla-labs/klaudiush/issues/86)) ([f848ec2](https://github.com/smykla-labs/klaudiush/commit/f848ec2a54aaff49bc5a50934295c78a70b63014))
* **shellscript:** skip parse errors in fragments ([#88](https://github.com/smykla-labs/klaudiush/issues/88)) ([9bcef49](https://github.com/smykla-labs/klaudiush/commit/9bcef493c70bd8ddc148f9bb9597951a418907a6))

### Code Refactoring

* **commit:** use custom conventional parser ([#82](https://github.com/smykla-labs/klaudiush/issues/82)) ([b6c8bb0](https://github.com/smykla-labs/klaudiush/commit/b6c8bb0212e9350a8e542321076649a7843858bf))
* **deps:** replace go-isatty with x/term ([#85](https://github.com/smykla-labs/klaudiush/issues/85)) ([8ffd19b](https://github.com/smykla-labs/klaudiush/commit/8ffd19b95d849a293ee845eb8e38636b364d3130))

## [1.4.0](https://github.com/smykla-labs/klaudiush/compare/v1.3.0...v1.4.0) (2025-11-29)

### Features

* **git:** validate remote exists for git fetch ([#79](https://github.com/smykla-labs/klaudiush/issues/79)) ([bfae7cd](https://github.com/smykla-labs/klaudiush/commit/bfae7cdb9c8b3d4d05c77b13f74e52df2a393cc6))

### Bug Fixes

* **git:** support `-C` flag for git worktrees ([#75](https://github.com/smykla-labs/klaudiush/issues/75)) ([711a25b](https://github.com/smykla-labs/klaudiush/commit/711a25bbd0a96e5d3a6ce429d5f64fdad07510e6))
* **linters:** improve markdown fragment handling ([#74](https://github.com/smykla-labs/klaudiush/issues/74)) ([56617b6](https://github.com/smykla-labs/klaudiush/commit/56617b60a3c1d5fdb93fdafde7db323090ce362b))

## [1.3.0](https://github.com/smykla-labs/klaudiush/compare/v1.2.0...v1.3.0) (2025-11-28)

### Features

* **plugin:** add security improvements ([#72](https://github.com/smykla-labs/klaudiush/issues/72)) ([2a5aa8c](https://github.com/smykla-labs/klaudiush/commit/2a5aa8cb73032c07aabed2717243611bce240f60))
* **rules:** add dynamic validation rules engine ([#55](https://github.com/smykla-labs/klaudiush/issues/55)) ([541e573](https://github.com/smykla-labs/klaudiush/commit/541e5730c739c02728dc7c2c3ac194f1d0a9bd52))
* **validators:** add GitHub issue validator ([#57](https://github.com/smykla-labs/klaudiush/issues/57)) ([0bd2b65](https://github.com/smykla-labs/klaudiush/commit/0bd2b65e1ff6b6bdf1e7871c691030f3689a4d16))

### Bug Fixes

* **linters:** disable MD013 for markdown fragments ([#59](https://github.com/smykla-labs/klaudiush/issues/59)) ([d78f978](https://github.com/smykla-labs/klaudiush/commit/d78f9786a886405429e0ef6d1a20cebf4fb3824c))
* **linters:** enhance markdown fragment errors ([#54](https://github.com/smykla-labs/klaudiush/issues/54)) ([793980b](https://github.com/smykla-labs/klaudiush/commit/793980b370fe6ef80a164caa1e3b16862697b769))
* **linters:** handle MD047 for line-0 fragments ([#58](https://github.com/smykla-labs/klaudiush/issues/58)) ([c402a1d](https://github.com/smykla-labs/klaudiush/commit/c402a1d4371fe75b04e13effe6d3168310d84e30))
* **validators:** align PR errors with policy ([#56](https://github.com/smykla-labs/klaudiush/issues/56)) ([8ea3140](https://github.com/smykla-labs/klaudiush/commit/8ea314050a93c1a863d8629199f11f5be2fe382a))

## [1.2.0](https://github.com/smykla-labs/klaudiush/compare/v1.1.1...v1.2.0) (2025-11-28)

### Features

* **validator:** add backtick escaping validation ([#46](https://github.com/smykla-labs/klaudiush/issues/46)) ([f2e0968](https://github.com/smykla-labs/klaudiush/commit/f2e0968c38401656de4032d60e133e1d7ad36efc))
* **validator:** add error refs for validators ([#50](https://github.com/smykla-labs/klaudiush/issues/50)) ([07c8aae](https://github.com/smykla-labs/klaudiush/commit/07c8aae87d8124c83e56147f481514b1f77f2c00))
* **validator:** validate PR merge commit messages ([#41](https://github.com/smykla-labs/klaudiush/issues/41)) ([d1e0b71](https://github.com/smykla-labs/klaudiush/commit/d1e0b711f6b4cc41a214309f8a67d5ebc04c5c16))

### Bug Fixes

* **linters:** improve markdownlint-cli2 output ([#51](https://github.com/smykla-labs/klaudiush/issues/51)) ([b201e6b](https://github.com/smykla-labs/klaudiush/commit/b201e6b12d0fca99796e9e533cf0e1e8b4d6b5b6))
* **parser:** fix strict trailer validation errors ([#47](https://github.com/smykla-labs/klaudiush/issues/47)) ([f19e0ec](https://github.com/smykla-labs/klaudiush/commit/f19e0ecff176e5af7f0e1eede4d46252aac2f952))
* **validator:** prevent markdown validator from treating # in code blocks as headers ([#35](https://github.com/smykla-labs/klaudiush/issues/35)) ([68cb540](https://github.com/smykla-labs/klaudiush/commit/68cb540f290d276e28de757e0b630cc32ec32728))
* **validator:** trim HEREDOC trailing newlines ([#38](https://github.com/smykla-labs/klaudiush/issues/38)) ([7e67def](https://github.com/smykla-labs/klaudiush/commit/7e67defc12b91f8298a7a8eb7421e5cc97da2ca2))
* **validator:** use rune count for commit titles ([#45](https://github.com/smykla-labs/klaudiush/issues/45)) ([b9d7339](https://github.com/smykla-labs/klaudiush/commit/b9d7339962e7c8b5d793aba3d516e00529691d8f))

### Code Refactoring

* **errors:** migrate to cockroachdb/errors ([#52](https://github.com/smykla-labs/klaudiush/issues/52)) ([ac30eaf](https://github.com/smykla-labs/klaudiush/commit/ac30eaf17e8156f7bab45537b85d190458d66702))
* **validator:** unified error reference ([#34](https://github.com/smykla-labs/klaudiush/issues/34)) ([e21a483](https://github.com/smykla-labs/klaudiush/commit/e21a483670ac104b82277f524fea8eb7698b2be8))

## [1.1.1](https://github.com/smykla-labs/klaudiush/compare/v1.1.0...v1.1.1) (2025-11-27)

### Bug Fixes

* **ci:** address Copilot review suggestions for Claude review workflow ([#30](https://github.com/smykla-labs/klaudiush/issues/30)) ([0690734](https://github.com/smykla-labs/klaudiush/commit/0690734fd7bc2dcdcc219052183ea9e7a0119f1b)), closes [#28](https://github.com/smykla-labs/klaudiush/issues/28)

## [1.1.0](https://github.com/smykla-labs/klaudiush/compare/v1.0.2...v1.1.0) (2025-11-26)

### Features

* **dispatcher:** add parallel validator execution ([525a592](https://github.com/smykla-labs/klaudiush/commit/525a59241d24cab3a6af70cc2b7123c613b59491))
* **git:** add request-scoped git caching ([bdfa248](https://github.com/smykla-labs/klaudiush/commit/bdfa248682bca41cbeda32a478388e84ceed1d2b))
* **plugin:** add extensible plugin system with Go/exec/gRPC loaders ([#20](https://github.com/smykla-labs/klaudiush/issues/20)) ([2ed6811](https://github.com/smykla-labs/klaudiush/commit/2ed68115448d7a66d80f1b068e06327d7640555a))
* **validator:** add enhanced error reporting ([2d94303](https://github.com/smykla-labs/klaudiush/commit/2d94303335f55f3e2f7686e843bf1fd6b0a342d2))
* **validator:** add secrets detection validator ([7df5898](https://github.com/smykla-labs/klaudiush/commit/7df589896f27e6a66a50a866bf778dbf063af33b))

### Bug Fixes

* **security:** add regex anchors to prevent ReDoS ([6a62e6d](https://github.com/smykla-labs/klaudiush/commit/6a62e6df86369b04b35e9433f5739431d8061940)), closes [#123](https://github.com/smykla-labs/klaudiush/issues/123)
* **validator:** use appropriate error codes for commit validation ([#29](https://github.com/smykla-labs/klaudiush/issues/29)) ([89a66e7](https://github.com/smykla-labs/klaudiush/commit/89a66e70d4c6352af1c8ec356adcb42f24690ffa)), closes [#23](https://github.com/smykla-labs/klaudiush/issues/23)

## [1.0.2](https://github.com/smykla-labs/klaudiush/compare/v1.0.1...v1.0.2) (2025-11-25)

### Bug Fixes

* **deps:** update module github.com/charmbracelet/huh to v0.8.0 ([#7](https://github.com/smykla-labs/klaudiush/issues/7)) ([67d8f7c](https://github.com/smykla-labs/klaudiush/commit/67d8f7c78740f4591d6cb2c9900c7e7be87f1a33))

## [1.0.1](https://github.com/smykla-labs/klaudiush/compare/v1.0.0...v1.0.1) (2025-11-25)

### Bug Fixes

* **deps:** update module go.uber.org/mock to v0.6.0 ([#8](https://github.com/smykla-labs/klaudiush/issues/8)) ([fb575e1](https://github.com/smykla-labs/klaudiush/commit/fb575e1e244c074a6ab59def5964be30f9fe9b8b))

## 1.0.0 (2025-11-25)

### ⚠ BREAKING CHANGES

* **validators:** - Markdown formatting violations now block writes (previously warnings only)
- Users must fix markdown errors before files can be written

Signed-off-by: Bart Smykla <bartek@smykla.com>

### Features

* **ci:** add pre-commit hook for linting ([58a652e](https://github.com/smykla-labs/klaudiush/commit/58a652ed086c76566f64be7d41bd49a698117f05))
* **ci:** add test hooks for commit and push ([59d6f44](https://github.com/smykla-labs/klaudiush/commit/59d6f44ae72df251aaa72028d55cd362ebedc5db))
* **cli:** add interactive init command ([31f8284](https://github.com/smykla-labs/klaudiush/commit/31f82846547462f0535e4001163438b8f334e2bf))
* **cli:** rename binary to chook, improve install ([641c21d](https://github.com/smykla-labs/klaudiush/commit/641c21de5862f9198bf1d402a60cf37a69aebb17))
* **commit:** allow revert commits without type(scope) ([44ad885](https://github.com/smykla-labs/klaudiush/commit/44ad885aa3da7db40fff91a8bd2ba87e21b3085a))
* **config:** add configuration schema foundation ([531e5ab](https://github.com/smykla-labs/klaudiush/commit/531e5ab8e359fcba4ec493de72bb34dd10701717))
* **config:** add loading and validation ([7f7570e](https://github.com/smykla-labs/klaudiush/commit/7f7570ec81fcc3692e0dce4afdcfdef9cbeedd4e))
* **config:** add markdownlint rule configuration ([737d3e3](https://github.com/smykla-labs/klaudiush/commit/737d3e32d6791cb00544af697dbb781f5ba375a3))
* **config:** add multi-source provider ([72b6dc5](https://github.com/smykla-labs/klaudiush/commit/72b6dc5210ebb9d8e4936884ce2c9e8536f3d01e))
* **config:** add validator factory system ([b2f69e8](https://github.com/smykla-labs/klaudiush/commit/b2f69e834ca7fa95779a28492a490da634d0fcae))
* **config:** complete validator integration ([086ac9a](https://github.com/smykla-labs/klaudiush/commit/086ac9aef049690a5bfc6cfdd8e94ca20e729ace))
* **config:** integrate config into git validators ([5e20797](https://github.com/smykla-labs/klaudiush/commit/5e20797c3801ce855fcbe48aa0a37ea2978d6575))
* **config:** integrate config system into main ([70ff6c9](https://github.com/smykla-labs/klaudiush/commit/70ff6c9fe2bdfab2040b350d9b9b43bcfa03d23c))
* **config:** integrate validators with config ([450bbd7](https://github.com/smykla-labs/klaudiush/commit/450bbd7eb8124ef1915d752554d7716328171aa2))
* **dev:** add mise for tool version management ([c30b883](https://github.com/smykla-labs/klaudiush/commit/c30b883f9be882f1a3e0157904c24ca49a6af0fe))
* **doctor:** add auto-fix system ([2034ee0](https://github.com/smykla-labs/klaudiush/commit/2034ee02258b3cc1380f8cd8e007ac01b2468749))
* **doctor:** add CLI command ([276cb5f](https://github.com/smykla-labs/klaudiush/commit/276cb5f1f1e1a521f4d81644327ed2f8f1b015cf))
* **doctor:** add Phase 1 core infrastructure ([247dede](https://github.com/smykla-labs/klaudiush/commit/247dede17f3c3a5a4dd270b61916a2dcaafa0170))
* **doctor:** add settings parser for phase 2 ([7aaca82](https://github.com/smykla-labs/klaudiush/commit/7aaca82b901fa22d21c241893fc3e25970396679))
* **doctor:** implement core checkers ([1938abd](https://github.com/smykla-labs/klaudiush/commit/1938abd7e7fb23eba074f35c3381883ae171d3fd))
* **enums:** add enumer code generation for type-safe enums ([296b1fb](https://github.com/smykla-labs/klaudiush/commit/296b1fb5046d07c8babe53ca2d636a31eaee2e59))
* **git:** add go-git v6 SDK foundation ([65fbc9b](https://github.com/smykla-labs/klaudiush/commit/65fbc9bf7a1829c7047db5001d155afd76d91f2c))
* **git:** add SDK/CLI factory pattern ([91cda94](https://github.com/smykla-labs/klaudiush/commit/91cda94c279019b1d08bd4ec62aeb6b059998f07))
* **git:** block --no-verify flag in git commits ([d60d4c2](https://github.com/smykla-labs/klaudiush/commit/d60d4c2916fd36d1695bbefca55bdd59339a8600))
* **github:** add SDK client for GitHub API ([e5dbd46](https://github.com/smykla-labs/klaudiush/commit/e5dbd4677b8590553dddabd533210b8a83bcd965))
* **git:** use SDK implementation by default ([8a11e3a](https://github.com/smykla-labs/klaudiush/commit/8a11e3a217bb973d4830818c0348a732c078a57c))
* **golangci:** enable additional linters and all gocritic checks ([43598e2](https://github.com/smykla-labs/klaudiush/commit/43598e29a563a9058bdb2740263a85660a449886))
* **golangci:** enable modernize and err113 linters ([5a7b6af](https://github.com/smykla-labs/klaudiush/commit/5a7b6af0b075aafa269240afbe74eb7a878effc8))
* **hooks:** implement CommitValidator with dependency injection ([6632a1f](https://github.com/smykla-labs/klaudiush/commit/6632a1f397464ce028184fa148903d96f08640a0)), closes [#123](https://github.com/smykla-labs/klaudiush/issues/123)
* **hooks:** implement GitPushValidator with remote validation ([c88de94](https://github.com/smykla-labs/klaudiush/commit/c88de9430c917607bf54f2b70cf980943a7ef635))
* **hooks:** initialize claude-hooks Go rewrite ([deac74b](https://github.com/smykla-labs/klaudiush/commit/deac74bfa0e1114f5a9830c60465359e2efe579c))
* **hooks:** migrate from custom scripts to lefthook ([eaa7c60](https://github.com/smykla-labs/klaudiush/commit/eaa7c60b7be46b94870a01b7d3eb19dc87bdd65b))
* **init:** add charmbracelet/huh TUI ([0cf9115](https://github.com/smykla-labs/klaudiush/commit/0cf9115952e8c30682cb8e61ac7b144ce4c259a4))
* **lint:** add extensive linter configuration ([d92238e](https://github.com/smykla-labs/klaudiush/commit/d92238e4275560276ac6cd1142c63ba4c44d24e0))
* **lint:** enhance linter configuration for nil safety ([0e38d82](https://github.com/smykla-labs/klaudiush/commit/0e38d82a4f3109459e87c44ef324e2945572359e))
* **linters:** add ShellChecker tests and TfLinter ([db684fb](https://github.com/smykla-labs/klaudiush/commit/db684fb9d0dd8a0ab8470d04c9ef089268d64e14))
* **linters:** add tests and migrate validators ([a56f178](https://github.com/smykla-labs/klaudiush/commit/a56f1783fef4ce2b463e079c0fb56f2e464f29f0))
* **linters:** add typed linter abstractions ([2ffe806](https://github.com/smykla-labs/klaudiush/commit/2ffe8064a1e053a62a437240841f027fba4a79f8))
* **linters:** implement output parsing ([#1](https://github.com/smykla-labs/klaudiush/issues/1)) ([e031f11](https://github.com/smykla-labs/klaudiush/commit/e031f11eff254fd9371cf1ce1bdb77caefc30851))
* **markdown:** add table formatting suggestions ([b0f98a3](https://github.com/smykla-labs/klaudiush/commit/b0f98a332ad56de7d308316f214cf0d78e282b95))
* **parser:** add file write detection ([52378d8](https://github.com/smykla-labs/klaudiush/commit/52378d89d5b11349e08f9932fcecc30d72add2c2))
* **parser:** add git command parsing ([a7f3ae8](https://github.com/smykla-labs/klaudiush/commit/a7f3ae840bec8c4b179ee9ad2fe3fb79da158895))
* **parser:** add mvdan.cc/sh dependency ([12af6c1](https://github.com/smykla-labs/klaudiush/commit/12af6c1100c29bbaec31f077806be760e0915c2b))
* **parser:** implement Bash parser core ([968d797](https://github.com/smykla-labs/klaudiush/commit/968d797e0abee542a76f10633708816ec2945419))
* **pr:** improve supporting documentation validation ([e9561bb](https://github.com/smykla-labs/klaudiush/commit/e9561bbfe1b0adfbe7c96b77537912bdfd19f15f))
* **testing:** add testscript integration tests ([a663a7d](https://github.com/smykla-labs/klaudiush/commit/a663a7d3424b3fd364e46293c702b2408af47b08))
* **testing:** add uber-go/mock for mocks ([75dd9d9](https://github.com/smykla-labs/klaudiush/commit/75dd9d9cb542e03310246275bb999230edbcd969))
* **validator:** add file commit support ([d96732b](https://github.com/smykla-labs/klaudiush/commit/d96732beb9379655dea575f28441b1612c171e9c))
* **validators:** add branch name validator ([13724d8](https://github.com/smykla-labs/klaudiush/commit/13724d82015f4caa9b7287ce4259524d2d6c8ae7))
* **validators:** add composable action support ([44942b4](https://github.com/smykla-labs/klaudiush/commit/44942b4c5b0f28681579d23dde6e090ed47fe9cd))
* **validators:** add forbidden patterns config ([2a70e03](https://github.com/smykla-labs/klaudiush/commit/2a70e0386116bb2355910a1db14fdc1931c7ec53))
* **validators:** add GitHub workflow validator ([f98de57](https://github.com/smykla-labs/klaudiush/commit/f98de571748f3d30b96623016b2e339f154f6e3d))
* **validators:** add heading context preamble ([68aa2e0](https://github.com/smykla-labs/klaudiush/commit/68aa2e0f20342fab51975c83c8d49a55e6d8abac))
* **validators:** add list context preamble ([96acd5b](https://github.com/smykla-labs/klaudiush/commit/96acd5bf03ec3a3e5c23a45d3c519ac71c76a253))
* **validators:** add markdown format validator ([7821f33](https://github.com/smykla-labs/klaudiush/commit/7821f33ac81c5ce6b778d1292ef1507afa668be4))
* **validators:** add notification bell handler ([3c17f4f](https://github.com/smykla-labs/klaudiush/commit/3c17f4f37432fefccd5b52d8974aa56cbddbfc8e))
* **validators:** add shellscript validator ([2d04348](https://github.com/smykla-labs/klaudiush/commit/2d04348847ac07dc240e6a7a123431366e42bdf1))
* **validators:** add Terraform format validator ([771fa3f](https://github.com/smykla-labs/klaudiush/commit/771fa3fbafe5d91e7100b524842f4f89ba587581))
* **validators:** configurable PR markdown rules ([fda88d0](https://github.com/smykla-labs/klaudiush/commit/fda88d013e98ee6a49d0b614c9d0f2cb4ab49cb1))
* **validators:** enforce mandatory commit scopes ([bec535b](https://github.com/smykla-labs/klaudiush/commit/bec535be3e8bfa4406d48e79bc74937c60024b5e))
* **validators:** exempt revert titles from length ([55000bd](https://github.com/smykla-labs/klaudiush/commit/55000bd3aa93f12b61e28c70d0252af8619a4a60))
* **validators:** extend markdown validation ([48a233a](https://github.com/smykla-labs/klaudiush/commit/48a233a8892c26f83af66326cfe1875134970e8e))
* **validators:** implement GitAddValidator ([51dd1c5](https://github.com/smykla-labs/klaudiush/commit/51dd1c54e1f5c24aaf9d699b2b6e1a611dc5cdc8))
* **validators:** implement PRValidator ([b94609d](https://github.com/smykla-labs/klaudiush/commit/b94609d3fd398a987f87f6d575d980323162d0a9))
* **validators:** migrate to PreToolUse hooks ([52245bf](https://github.com/smykla-labs/klaudiush/commit/52245bfb54ea82eeef61ac98dfb53a7a919c2192))
* **validators:** register MarkdownValidator ([41d94e4](https://github.com/smykla-labs/klaudiush/commit/41d94e443857a011a7027e12d2cde7241aed9655))
* **validators:** register new validators ([96e17bf](https://github.com/smykla-labs/klaudiush/commit/96e17bf04d5e4e4b7cce74048af3ce8742532726))
* **validators:** use typed linter interfaces ([4104c64](https://github.com/smykla-labs/klaudiush/commit/4104c64548a0ac422d10cd3dd3b3dd1c4f337409))
* **validators:** validate only edit fragments ([bc5bd9d](https://github.com/smykla-labs/klaudiush/commit/bc5bd9df37e6eb2e3d813b5f121a59b4e30517b8))
* **validator:** validate bash heredoc file writes ([209dd5f](https://github.com/smykla-labs/klaudiush/commit/209dd5fdc4cb3520b753eae2b54696a865f15572))

### Bug Fixes

* **commit:** handle git -C option in validation ([7245e2e](https://github.com/smykla-labs/klaudiush/commit/7245e2eba68a4217d749b32e3d8391a1b5d2c47b))
* **config:** merge markdownlint rules properly ([5c16210](https://github.com/smykla-labs/klaudiush/commit/5c162106d25419305eba701eeaefd609025ca432))
* **hooks:** correct lefthook configuration ([e1852ae](https://github.com/smykla-labs/klaudiush/commit/e1852ae1b191d7bf0973e65dc76eb5332f5c6d9e))
* **lint:** resolve all linting issues ([cd38591](https://github.com/smykla-labs/klaudiush/commit/cd385913850ac95277257de3c835a2cecc2cc693))
* **lint:** resolve golangci-lint v2 issues ([f9698c6](https://github.com/smykla-labs/klaudiush/commit/f9698c6df7f084850dc7c92d46d881a160c34b73))
* **logger:** use local timezone in log timestamps ([10c59d3](https://github.com/smykla-labs/klaudiush/commit/10c59d31698205d11699923ff31142104f01e6fc))
* **logger:** write logs to file only, not stderr ([12d36d9](https://github.com/smykla-labs/klaudiush/commit/12d36d9ebe5e4dd2571e8c16422c617797fe24bf))
* **mdtable:** prevent false positives in spacing detection ([d9bae1f](https://github.com/smykla-labs/klaudiush/commit/d9bae1fc7e986450a692fbbd5003bb4d24824d77))
* **notification:** handle all notification events with bell ([6795dba](https://github.com/smykla-labs/klaudiush/commit/6795dbad0db7f5fc0f15517157d16f0edae89877))
* **parser:** extract heredoc content from commit messages ([23fd58d](https://github.com/smykla-labs/klaudiush/commit/23fd58dce535665c2e2de45d788e4ae2691a4887))
* **predicates:** handle command chains in git predicates ([607e398](https://github.com/smykla-labs/klaudiush/commit/607e398b1bacc0cfc4ca3c8ec7c3a9572a831067))
* **predicates:** handle git global options in matching ([ca725b9](https://github.com/smykla-labs/klaudiush/commit/ca725b93388550bed80d4102059359cb03f56fb5))
* **task:** preserve tmp/ directory in clean task ([397f55a](https://github.com/smykla-labs/klaudiush/commit/397f55a729a835fdb2df34d22990edef34bf286a))
* **validator:** allow naked URLs in PR bodies ([d39207d](https://github.com/smykla-labs/klaudiush/commit/d39207d623e277872af640209fcec96edb933d7c))
* **validator:** block attribution footers ([1db6453](https://github.com/smykla-labs/klaudiush/commit/1db6453cfc8ffbb3ffb07fda7f104f0501b2dc9f))
* **validator:** relax markdownlint for PR bodies ([e561424](https://github.com/smykla-labs/klaudiush/commit/e56142406b675aea33db0c302b98b7f31053aff8))
* **validators:** allow CLAUDE file references ([aaa2580](https://github.com/smykla-labs/klaudiush/commit/aaa2580e8eed956d51f65710518d0d1066b13dab))
* **validators:** avoid markdown fragment warnings ([2ac6853](https://github.com/smykla-labs/klaudiush/commit/2ac685302f1795e014da99b6c0396d050589f116))
* **validators:** clean temp paths from errors ([7295d92](https://github.com/smykla-labs/klaudiush/commit/7295d9297a1ce5760d06f419277d0d7a7ff5fcfe))
* **validators:** disable MD041 for fragments ([0442534](https://github.com/smykla-labs/klaudiush/commit/044253433a9199e6fd0b096c8335618338896869))
* **validators:** disable MD047 for mid-file edits ([6751df6](https://github.com/smykla-labs/klaudiush/commit/6751df629df894f4249b76819269b88d4bea9430))
* **validators:** handle chained git add commands ([b7a0f73](https://github.com/smykla-labs/klaudiush/commit/b7a0f7378d9291e58a3e8dd7ad0dbc168b34311e))
* **validators:** improve Edit and AI validation ([0dc6ea6](https://github.com/smykla-labs/klaudiush/commit/0dc6ea658cf7c891eab9dfe3cc0e3b31e4dd873d))

### Code Refactoring

* **commit:** adopt go-conventionalcommits ([e1e4b25](https://github.com/smykla-labs/klaudiush/commit/e1e4b253ab7e83cadd5f6e34083bf2629d3d7e1d))
* **config:** migrate to koanf library ([61a95dc](https://github.com/smykla-labs/klaudiush/commit/61a95dcd328f5a487f289526844fe154661fb494))
* **config:** remove build-time signoff ([f240b34](https://github.com/smykla-labs/klaudiush/commit/f240b346b41c91049cc31208749454c0706fe2af))
* **dispatcher:** improve error formatting ([569e854](https://github.com/smykla-labs/klaudiush/commit/569e8542db4fd02cf1d85f7431f3a5a8a391c3d3))
* **exec:** create command execution abstractions ([a04c617](https://github.com/smykla-labs/klaudiush/commit/a04c617962d73b95557dd8ef65b57195818a474a))
* **exec:** eliminate nil safety warnings ([70c668d](https://github.com/smykla-labs/klaudiush/commit/70c668dae0d1f794e069ac958348267889e7b5e3))
* **hooks:** improve lefthook config with v2 features ([06cea2d](https://github.com/smykla-labs/klaudiush/commit/06cea2d493550f7b686316252a2f2b41d962d30d))
* **logger:** migrate to slog ([d48987b](https://github.com/smykla-labs/klaudiush/commit/d48987b06d43b9d8d84ed69fbb3f23615015d5a8))
* **logging:** improve log readability ([8241fd8](https://github.com/smykla-labs/klaudiush/commit/8241fd8b6dcae55f1173db9f63f2f123b76847c5))
* **parser:** simplify flag parsing ([28d7aae](https://github.com/smykla-labs/klaudiush/commit/28d7aae69ecea0707e55f9fd4844695274b2d37c))
* **project:** rename to klaudiush ([c626a16](https://github.com/smykla-labs/klaudiush/commit/c626a1699821352c8c77df7cc613a631209fcda6))
* **validators:** enhance branch validator ([5cc88c3](https://github.com/smykla-labs/klaudiush/commit/5cc88c33a8a77703834c38dd9bcd852ec448d83a))
* **validators:** fix lint issues in validators ([a4c05dd](https://github.com/smykla-labs/klaudiush/commit/a4c05ddf42761849737ed093e31691bca07a6a1f))
* **validators:** migrate terraform validator to exec pkg ([d4f15cd](https://github.com/smykla-labs/klaudiush/commit/d4f15cdf5cb53e9f677746ee5e4d199fb66de7e7))
* **validators:** migrate workflow validator to exec package ([7b308c4](https://github.com/smykla-labs/klaudiush/commit/7b308c4fa2b09091d0ca8e49e237e454f2c8640a))
* **validators:** reduce cognitive complexity ([443115f](https://github.com/smykla-labs/klaudiush/commit/443115fbf139fe9e5a7f17561f1e0c956d27fc10))
* **validators:** replace string builders with templates ([125499b](https://github.com/smykla-labs/klaudiush/commit/125499ba8303f4305c212d1c62e147dce7bb7033))
* **validators:** thread context for timeouts ([ccc8735](https://github.com/smykla-labs/klaudiush/commit/ccc87356161f7d2170cc8a77548a14a2aaf0e1a0))
