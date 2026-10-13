# Changelog

## [2.0.0](https://github.com/tinmancoding/tasktree/compare/v1.3.0...v2.0.0) (2026-10-13)


### ⚠ BREAKING CHANGES

* **info:** mark is the only status writer; add info ack
* rename binary to tasktree with tt and tt2 install symlinks
* replace v1 core with v2 records skeleton

### Features

* add bootstrap executor ([b6d5b9d](https://github.com/tinmancoding/tasktree/commit/b6d5b9d3e2fc0a1141a79ab7073ae5118754dad8))
* add bootstrap steps to apply ([9dcc1d6](https://github.com/tinmancoding/tasktree/commit/9dcc1d610d0de2eb3b31c9fb1c55dc534ff5a06b))
* add doctor and move commands ([90026c9](https://github.com/tinmancoding/tasktree/commit/90026c9e46418be044d40d8759c64ba859f4d721))
* add M2 tasktree workflows ([4975d08](https://github.com/tinmancoding/tasktree/commit/4975d087675a5c47aa3e77f5b5769abc20060163))
* add tasktree --version and the version command ([5fa09ad](https://github.com/tinmancoding/tasktree/commit/5fa09ade891b9af7cfeb8ef742f701622371da76))
* add tasktree close flow ([9682083](https://github.com/tinmancoding/tasktree/commit/9682083cd911fe2f00a0d578786d25a792e157c6))
* add tasktree workspace lifecycle ([f45d1e0](https://github.com/tinmancoding/tasktree/commit/f45d1e04574b0829d6ebe15f7548049e0c604deb))
* add v2 agent task paths ([4b4845c](https://github.com/tinmancoding/tasktree/commit/4b4845c18930c2cc42aef231455d0d52d9384b80))
* add V2 task record management ([9714ef9](https://github.com/tinmancoding/tasktree/commit/9714ef9453ecebc340a810c439f72094abf1c002))
* **alias:** add alias add, rm, and list ([5b66244](https://github.com/tinmancoding/tasktree/commit/5b66244b3f8bba6addac0aec25c7b6242175ff85))
* **bootstrap:** add, rm, and list bootstrap steps ([b9479e0](https://github.com/tinmancoding/tasktree/commit/b9479e06fc8308a8b92bc0692eed8238d674bc35))
* import V1 tasktree workspaces ([1534dae](https://github.com/tinmancoding/tasktree/commit/1534daebd950e4bcac0619d6068a7c887cebfbeb))
* **info:** mark is the only status writer; add info ack ([8413e2a](https://github.com/tinmancoding/tasktree/commit/8413e2a83a4118c127fce6ae5aef2495646bbfce))
* **list:** filter tasks by exact typed ref with --ref ([b605460](https://github.com/tinmancoding/tasktree/commit/b605460c723e4543f856761e0d3a2cb0d69b0ab7))
* **new:** isolated subtasks take --repos/--branch; templates take overrides ([cdcfd1d](https://github.com/tinmancoding/tasktree/commit/cdcfd1df91e592e7ac0a5150da86d071c0639435))
* replace v1 core with v2 records skeleton ([1f23ff3](https://github.com/tinmancoding/tasktree/commit/1f23ff33317479bdcee7bf2caf7efeb79d17fc23))
* **schema:** V2 JSON Schemas for records, tasktree config and templates ([26437f4](https://github.com/tinmancoding/tasktree/commit/26437f4b9cb451c638ba9b263c00a0f7f90fa1d5))
* snapshot and restore ([4760126](https://github.com/tinmancoding/tasktree/commit/4760126f56e40a9d030f809f3cfcb7bcc808407c))
* **sync:** add semantic record merging and leased hub plumbing ([691be58](https://github.com/tinmancoding/tasktree/commit/691be58e363c60728a5c541451640c5b9a281a7b))
* **sync:** synchronize remote tasks and resolve divergent checkouts ([a0b5436](https://github.com/tinmancoding/tasktree/commit/a0b5436bad7f41806bde392617abac6f2cd146cb))
* **template:** emit JSON from template list and show ([11217cb](https://github.com/tinmancoding/tasktree/commit/11217cbab765acddecbb7c8508073e3d32853f3b))
* **v2:** support hub tasktree definitions and templates ([6d35092](https://github.com/tinmancoding/tasktree/commit/6d35092bed38a22a6756bb181a25af36179ee09d))


### Bug Fixes

* add command descriptions to help ([f368697](https://github.com/tinmancoding/tasktree/commit/f368697ff7bea126b0394a5c6f3fe770c4774f57))
* **add:** name the pinned ref in the add message for --mode pin ([7d9aafc](https://github.com/tinmancoding/tasktree/commit/7d9aafcb4422facf8b46828a5cf4e1f4258be280))
* **add:** one K2 resolver for checkout add and definition add -t ([9124c6a](https://github.com/tinmancoding/tasktree/commit/9124c6aa5d36bb7a7f5e96d3f635993de1faa4e0))
* align add branch resolution messages ([d4d2ea7](https://github.com/tinmancoding/tasktree/commit/d4d2ea73c4bb29c8b3e93612587b18ca52fa95dc))
* align definition, history, doctor, and template tables ([b1ffd3f](https://github.com/tinmancoding/tasktree/commit/b1ffd3fc5cd6b630ad70a43ab420bc2905458072))
* align info repository table ([2ffc0d0](https://github.com/tinmancoding/tasktree/commit/2ffc0d0556f73a366c1cde2187198d2e63b77d1e))
* align loss checks and V1 registry parsing ([710de73](https://github.com/tinmancoding/tasktree/commit/710de734ef720859ff14e769664a54b05748fd2b))
* classify command errors by type ([3158102](https://github.com/tinmancoding/tasktree/commit/3158102d773fa532a28ff8daa05d404df9458249))
* **cli:** commit reports the short sha and the local hub ([ce84970](https://github.com/tinmancoding/tasktree/commit/ce849701ef17bb2c0dc0638aa8ed5eb23ff12e4e))
* **cli:** typed exit codes for not-found, usage, and missing checkouts ([eb8de32](https://github.com/tinmancoding/tasktree/commit/eb8de320efc27c270150e450345406f3eee24ca4))
* **commit:** gitlink rewind hints name the override of their own command ([ea50f98](https://github.com/tinmancoding/tasktree/commit/ea50f9895f450956bd870bf5705887b640648d94))
* complete info and setup error handling ([09ac82b](https://github.com/tinmancoding/tasktree/commit/09ac82b1528d68751369a6c7c033925c81a26571))
* correct record rendering and command output ([e558d8c](https://github.com/tinmancoding/tasktree/commit/e558d8c29d1a1aa38ec8d6d1041564aa3dcbc570))
* count unpushed parent commits in push warning ([7b195c4](https://github.com/tinmancoding/tasktree/commit/7b195c474b00b0b583ae598e8bda73701e180e2d))
* detect isolated branch namespace conflicts ([6abf57a](https://github.com/tinmancoding/tasktree/commit/6abf57a2054a7c7d9b120c857dbdde7f3a2b7b2f))
* doctor --fix never deletes hidden clones without --prune-clones ([8313ed1](https://github.com/tinmancoding/tasktree/commit/8313ed1da5393cd68e04882bd0fe12aeb536ca74))
* **doctor:** never offer to delete the clone of an incomplete import ([260378a](https://github.com/tinmancoding/tasktree/commit/260378ad279af280ae3da31947ba9859e0605e48))
* **doctor:** re-point a hand-moved checkout instead of deleting its git directories ([e07d999](https://github.com/tinmancoding/tasktree/commit/e07d999db34b854b4e308494e172c9cc493ef775))
* enforce safe structural workspace targeting ([6eddfe5](https://github.com/tinmancoding/tasktree/commit/6eddfe5bc779250d0617557b7a06aaa91065a062))
* fetch local parent commits before reopening subtasks ([329bce9](https://github.com/tinmancoding/tasktree/commit/329bce961dfed0ae72d9389d4c18fb2c43c9ffe7))
* **gitlinks:** retry skipped applies and gate accidental rewinds ([40fd298](https://github.com/tinmancoding/tasktree/commit/40fd29827c454c366dd90892500c5648d02b5d80))
* **gitx:** explicit transfers have no wall clock, only stall detection ([b793b57](https://github.com/tinmancoding/tasktree/commit/b793b57dab78923c0ce0b57a24112fd9ce3b37ec))
* harden records and git process handling ([caf887c](https://github.com/tinmancoding/tasktree/commit/caf887c476525c6392a6ac2a283148faedfbce55))
* harden V2 commits and import batches ([ea86f83](https://github.com/tinmancoding/tasktree/commit/ea86f834b8011e5fc5a15292a1044a9dc5e50f79))
* **hub:** detect and safely repair interrupted sync trees ([7ded326](https://github.com/tinmancoding/tasktree/commit/7ded3262880a2fdb2524dd6fa3aaa94507c1d9a3))
* **import:** log the resolved author on the import entry ([da2872b](https://github.com/tinmancoding/tasktree/commit/da2872b5e1fff4ef886b8a512f0865ebe997cc80))
* **import:** stop pointing at a V2 prune command in the completion message ([254b713](https://github.com/tinmancoding/tasktree/commit/254b713eeda7d88967cc1cd6ac401df25a6358b4))
* info shows the record when the recorded checkout is missing ([1c68693](https://github.com/tinmancoding/tasktree/commit/1c68693bbffd8dfc7b8c32e84ad0da2b91e26083))
* **info:** let info set parent --clear break an existing parent cycle ([876345d](https://github.com/tinmancoding/tasktree/commit/876345d65a27652ba735530d2371de540be39246))
* journal and harden live workspace imports ([320bceb](https://github.com/tinmancoding/tasktree/commit/320bcebf025a69e445e3bd0b5f7b4e2fec5d5787))
* **json:** print empty lists as [] in every --json document ([c05fed9](https://github.com/tinmancoding/tasktree/commit/c05fed9aa7406baa3a561c5730e33e74b7373f96))
* keep task branches orphaned from records ([2905fb6](https://github.com/tinmancoding/tasktree/commit/2905fb66be4912413715439920eaca221ef5ad5c))
* **mark:** survive a deleted working directory during a cascade ([afee588](https://github.com/tinmancoding/tasktree/commit/afee58889896af55268c769da7dea1704ffc09ca))
* **mark:** treat marking the current status as a no-op ([7c24423](https://github.com/tinmancoding/tasktree/commit/7c24423d05e947876ce72e12f7c52368dccb56f6))
* **open:** a live checkout is located without fetching or merging ([5a83694](https://github.com/tinmancoding/tasktree/commit/5a83694fdc12d383b80d2cb5f3d8363b13dca0d6))
* **open:** leave an unavailable gitlink uninitialized instead of failing ([a1a58bc](https://github.com/tinmancoding/tasktree/commit/a1a58bc76ca13b314f3398326b6ef04ab67ceed5))
* preserve isolated checkout after bootstrap failure ([8960609](https://github.com/tinmancoding/tasktree/commit/896060921ab91f348ff3bedf99521b46ef67eaf5))
* preserve verbose git output for tasktrees ([a514e58](https://github.com/tinmancoding/tasktree/commit/a514e58837a26c2345b702281ea1c9ff188dbfba))
* prevent workspace teardown data loss ([24c8388](https://github.com/tinmancoding/tasktree/commit/24c8388c29b7c9626a6de7b75b0d82388db4e3b5))
* print gate failure details once ([4d9d721](https://github.com/tinmancoding/tasktree/commit/4d9d72142fee6ef16e1c593a40aea5056f16fa0d))
* **record:** preserve one-sided bytes and audit all merge conflicts ([48259dc](https://github.com/tinmancoding/tasktree/commit/48259dcfb6b0dfa8b71ea644aa79effcdc4bb6b0))
* reject cycles during focused target resolution ([24205c2](https://github.com/tinmancoding/tasktree/commit/24205c29a0c9963e5e5160c8eb3eb907e6418e1d))
* **resolve:** merge task paths against their common ancestor ([da23ed6](https://github.com/tinmancoding/tasktree/commit/da23ed680b2c8af1ebaea6cf9b19a2df0c651698))
* **resolve:** report nothing to resolve and what a one-sided merge merged ([a35a6f7](https://github.com/tinmancoding/tasktree/commit/a35a6f77b705f66ec9c1919ac6a5674b4f3fcdad))
* scope isolated checkout failure cleanup ([f154382](https://github.com/tinmancoding/tasktree/commit/f154382bde853667083e643938b2c18176075101))
* serialize task collection and hierarchy mutations ([4268346](https://github.com/tinmancoding/tasktree/commit/42683463e6e54b09d7833eca30413b9b72a8a36c))
* **setup:** --remote on an existing hub pushes like config set remote ([4a84ca1](https://github.com/tinmancoding/tasktree/commit/4a84ca1797338de92276b69537907b851d336918))
* skip machine config during tasktree root discovery ([00c50aa](https://github.com/tinmancoding/tasktree/commit/00c50aa15365f5f39a1203bb0c7af1dc01a525d5))
* structural commits stage only the paths the tool changed ([b4bb3a7](https://github.com/tinmancoding/tasktree/commit/b4bb3a772f9ed62829bc90e05e9257df5434a776))
* surface malformed targeted records ([830ba71](https://github.com/tinmancoding/tasktree/commit/830ba71609ec9dfbae76c5b961ac61984f816b56))
* **sync:** bound transports and push immutable leased snapshots ([d504ed2](https://github.com/tinmancoding/tasktree/commit/d504ed2fda69fccfdfc95bdbbc7b7f53f18b8eb3))
* **sync:** regenerate task.md for checkouts whose record changed after a main merge ([8e68121](https://github.com/tinmancoding/tasktree/commit/8e68121aa168bd1bbff59ee2754f4e256e87351f))
* **sync:** retain repo tips ahead of resolved gitlinks ([a299fd8](https://github.com/tinmancoding/tasktree/commit/a299fd8781e987642cd3c218968aa74f067f6b4c))
* **target:** TASKTREE_TASK naming a shared subtask keeps the tasktree root ([86de5d1](https://github.com/tinmancoding/tasktree/commit/86de5d1302ccf514366fdd48ef4f43f70e708b2c))
* terminate bootstrap process groups on cancellation ([a9e5ece](https://github.com/tinmancoding/tasktree/commit/a9e5ece6fc002af8c4d362bf0a9122fe78260e47))
* treat empty .gitmodules as no submodules ([a31960e](https://github.com/tinmancoding/tasktree/commit/a31960e8e39926c250075bc114e6ee85e25745dc))
* **v2:** preserve deleted checkout module loss ([b7deb8c](https://github.com/tinmancoding/tasktree/commit/b7deb8c1902c46435956649a8e14b6e762c3e43d))
* validate every handle with one rule ([a11c8ff](https://github.com/tinmancoding/tasktree/commit/a11c8ff60e02959b636cda398b003e5929e29831))


### Code Refactoring

* rename binary to tasktree with tt and tt2 install symlinks ([b03815f](https://github.com/tinmancoding/tasktree/commit/b03815fe4b4fe263445fcce4247f144946d274e0))

## [1.3.0](https://github.com/tinmancoding/tasktree/compare/v1.2.0...v1.3.0) (2026-05-15)


### Features

* --chdir (-C) CLI argument ([78756a5](https://github.com/tinmancoding/tasktree/commit/78756a597ecf7133635dd2330597b714df8f80d4))
* additional sources ([cb2729c](https://github.com/tinmancoding/tasktree/commit/cb2729c642452f7b4f97a939a4091ad2dfd3a367))
* templates ([9be52b9](https://github.com/tinmancoding/tasktree/commit/9be52b908eff52016c0d984e17fe4f689b46bc3e))


### Bug Fixes

* renderTemplate other source types ([eda3693](https://github.com/tinmancoding/tasktree/commit/eda36934e2389ebe5798bd81499f935b30f35864))

## [1.2.0](https://github.com/tinmancoding/tasktree/compare/v1.1.0...v1.2.0) (2026-04-16)


### Features

* annotations ([e6b164b](https://github.com/tinmancoding/tasktree/commit/e6b164b063716f660c0cce3fa00f1f7853178c35))

## [1.1.0](https://github.com/tinmancoding/tasktree/compare/v1.0.1...v1.1.0) (2026-04-08)


### Features

* declarative Tasktree.yml format with apply and migrate commands ([6c06932](https://github.com/tinmancoding/tasktree/commit/6c06932bbf0c53472bfc5d0b78acb2aed51c0fee))
* new CLI UX for the tasktree add command ([f2940c0](https://github.com/tinmancoding/tasktree/commit/f2940c0c16a3017cb9d237b52e9c64c45b22d62b))
* prune ([4ff8196](https://github.com/tinmancoding/tasktree/commit/4ff8196dbd47ac9e8fc714d531c93fbf857d8360))
* repo aliases ([aa410c2](https://github.com/tinmancoding/tasktree/commit/aa410c29fb8b3687e1d900d98aba278ac1c296db))
* tasktree list list registered tasktrees ([d121ff5](https://github.com/tinmancoding/tasktree/commit/d121ff541586eef377a63bf1da7de93b3cd8fafc))


### Bug Fixes

* tasktree add --ref branch name should check for remote branch names too ([e4aeb93](https://github.com/tinmancoding/tasktree/commit/e4aeb93438bb36fb0d2902b6ad3ccdbe8de71825))
* use ~/.config/... folder on macos too ([b2fce55](https://github.com/tinmancoding/tasktree/commit/b2fce5524d0a4920eb20f07ef01e7e021f032216))

## [1.0.1](https://github.com/tinmancoding/tasktree/compare/v1.0.0...v1.0.1) (2026-03-25)


### Bug Fixes

* release ([4fae295](https://github.com/tinmancoding/tasktree/commit/4fae295253422195cca4f7cd341971696ed899cb))

## 1.0.0 (2026-03-25)


### Features

* setting up release ([22b47ad](https://github.com/tinmancoding/tasktree/commit/22b47ad6f7571fb9dd2996c188a91a22d10ad0ca))
