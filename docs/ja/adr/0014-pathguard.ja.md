# ADR-0014: パスの判定は nlink-jp/pathguard に任せる — 写しを持たない

- Status: Accepted
- Date: 2026-09-22

## Context

ADR-0013 以来、`work_dir` の検証は `internal/workdir` にあった。voice-scribe（組織 ADR-021 の参照実装）
からの写しで、同じ写しがほかの 7 サーバーにもあった。どの写しも場所を**名前で**比べていた。APFS は既定で
大文字小文字を区別しないので、`~/.SSH`、`/USR/local` のような綴りが同じ場所を指しながら検査を通った。
ホームディレクトリが分からないときは、資格情報の位置の検査がすべてを通した。

組織はこの判定を 1 つのモジュールにまとめた（`nlink-jp/pathguard`、lib-series）。場所をファイルの実体と、
ディスクと同じやり方で同一視した名前の両方で比べ、まだ存在しない場所もその親の実体で捕まえる。一覧は
gem-agent・lagent と同じものを 1 つ持つ。

## Decision

- `github.com/nlink-jp/pathguard` v0.1.0 を依存に加える。この org の外のコードは入らない。
- `internal/workdir` は**薄いアダプタ**にする。持つのは次だけ:
  - リクエストの `_meta` を文脈から取り出して `pathguard/workdir` の `Resolve` に渡すこと、
  - その `*workdir.Error` を `toolerr` の同じ code・message・details に移すこと、
  - `NewResolver(serverDirs...)` —— このサーバー自身のディレクトリ（今は設定ディレクトリ
    `~/.config/voice-studio-mcp` だけ）を守る場所（`pathguard.ServerDir`）として渡し、`work_dir_required`
    の 1 文を `RequiredHint` で添える。空のパスは何も守らないのではなく、すべての呼び出しを拒ませる
    （設定ディレクトリが決まらないのはホームが分からないときで、そのときは pathguard もすべてを拒む）。
- `Sensitive` は持たない。このサーバーのファイル引数（`script_path`、`casting_path` など）はすべて
  ワークスペース相対で、`os.Root` が封じ込める。`work_dir` の外のパスを読むことが無い。
- 呼び出し箇所（`Resolve`・`Validate`）は変えない。変わるのは組み立ての 1 行（`cmd/tools_registry.go` の
  `workDirResolver`）と、ゼロ値で組み立てていたテストだけである。
- 判定そのもののテストは pathguard にある。ここに残すのはアダプタのテスト（`_meta` の取り出し、エラーの写し、
  守る場所、ゼロ値が拒むこと）と、既存の契約テスト・配線のテストである。

## Consequences

`work_dir` の検査が変わる（CHANGELOG に書く）:

- **新たに拒む**: ランタイムと同じ一覧のうち、自分のホームにある本物の場所（`~/.kube`、`~/.config/gh`、
  `~/.azure`、`~/.terraform.d`、`~/.gemini`、`~/.config/mcp-bridge`、`~/.netrc`、`~/.npmrc`、`~/.pypirc`、
  `~/.git-credentials`、`~/.vault-token`、`~/.docker/config.json`、`~/.claude.json`、`~/.bash_history`、
  `~/.zsh_history`）。床のどの場所についても、大文字小文字の違い・リンク・ファームリンクなど、あらゆる綴り。
  それらのディレクトリの直下にあるリンクの指す先。`$HOME` がアカウントのホームと違うときは、両方を守る。
  Linux の `/etc`。
- **ホームが分からなければ、どの `work_dir` も拒む**。以前は通していた。
- `work_dir_denied` の `details` に `reason` が加わる。
- 1 回の検査は約 2 ms（pathguard の実測）。合成の時間に比べて無視できる。

写しを持たないので、判定の修正は pathguard のリリースと、ここでの依存の更新 1 行になる。

## Amendment (2026-09-22): 実際に使うディレクトリも判定する

`work_dir` だけを検査していたので、`work_dir=~/.config` と `workspace_id=gh` でワークスペースが `~/.config/gh` になり、
音声がそこへ書かれた。ADR-0013 の頃からの穴で、image-forge の独立レビューで見つかった。`workspace.NewManager(check)`
は判定を必須の引数として受け取り、`EnsureUnder` は `<work_dir>/<workspace_id>` を作る前・使う前に
`workdir.Resolver.CheckBeneath`（pathguard v0.2.0）で判定する。配線は `newToolDeps`。判定の無い Manager はすべての
ワークスペースを拒む。pathguard v0.2.0 は NUL バイトを含むパスも拒む。

## Amendment (2026-09-22, v0.6.1): ワークスペースが読むファイルはすべて読む前に判定する

`script_path` と `casting_path` はワークスペース相対で `os.Root` 越しに読むが、床には掛けていなかった。ワークスペースは
`CheckBeneath` を通っても床の場所を含み得る（`.env`、このサーバーの設定ディレクトリ、`~/.ssh` 内のリンクが同期フォルダを
指すならその行き先）。そうしたファイルは台本・キャスティング表として読まれ、中身が解析エラーに出た（`unknown keys:
[SECRET]`、`invalid character 'S'`）。答えも、無いときの「not found」と違った。slack-mcp-extender と chrome-pilot-mcp の
独立レビューで見つかった「存在で答えが変わる」型を、HOME を一時ディレクトリにしたテストで実測して見つけた（20 組中 12 組。
仕掛けたリンクで外へ出る 8 組は `os.Root` が存在に関係なく拒んでいた）。

- ワークスペースは、読む前・探す前にすべての読み取り（`ReadFile`・`Stat`・`VerifyRegular`）を判定する
  （`Workspace.judge`、internal/workspace/manager.go）—— pathguard の Local 方針とこのサーバー自身のディレクトリ。
  `workspace.NewManager(check, floor)` は床を必須の引数として受け取り（`workdir.Resolver.LocalPath`。無い Manager は
  すべてのワークスペースを拒む）、床の無い Workspace はすべての読み取りを拒む。pathguard がパス上のリンクを自分で
  辿るので置き場所を別に求める必要は無く、拒否は渡されたとおりのパスだけを名指す。
- 最初の版はツール側で `script_path` と `casting_path` を判定した。その独立レビューが、判定を通らない 3 つ目の
  読み取りを見つけた: `master` は台本の各行の `wav/<id>.wav` を読み、そこに仕掛けたリンクがワークスペース内の床の場所に
  届いた（あれば「not a RIFF/WAVE stream (36 bytes)」、無ければ `missing_line_ids`。合成キャッシュの `cached` 数も同様）。
  3 つの読み取りのうち 1 つが漏れたのはクラスなので、判定をすべての読み取りが通る 1 か所へ移した。拒まれた
  `wav/<id>.wav` は、ファイルが無いときと同じく「無い」と数える。
- `TestExistenceIsNotRevealed` は、同じパスをファイルがある状態と消した状態で `synthesize_script`・`synthesize_line`・
  `master` を呼び、答え全体を比べ、ファイルの中身が一切出ないことを確かめる（答えを変えない読み取りは見えない）。
  `TestMasterDoesNotReadAFloorFileThroughAWav` が wav の場合を、`TestEveryReadIsJudgedBeforeItLooks`
  （internal/workspace）が 3 つの読み取りそれぞれと、床の無い Manager・Workspace を固定する。6 つの変異（床を外す・
  各読み取りの判定を外す・床の無い Manager を受け入れる・床をワークスペースへ渡さない）はすべてアサーションで落ちた。
- pathguard 側の既知の限界（次のリリースに向けて記録）:
  - `work_dir` は pathguard/workdir が組織 ADR-022 §4 の順序（not found が denied より先）で検証するので、資格情報の
    ディレクトリを指す `work_dir` は、存在するかどうかで答えが変わる。
  - 非 ASCII 名のリンク先を別の Unicode 正規化で綴ると、同一性で拒むのはそれが存在するときだけになる（pathguard は
    正規化しない）。別の場所に作ったハードリンクを拒むのは、床の場所そのものであるファイル（`~/.netrc`、
    `~/.docker/config.json` など）へのものだけで、それも存在するときだけ。資格情報ディレクトリの中のファイル
    （`~/.ssh/id_rsa`）や `.env` へのハードリンクは拒まない —— ディレクトリはそれ自身の同一性で比べ、中のファイルでは比べない。
- 判定と読み取りは 2 段で、その間にすり替えたリンクは辿られる（check-to-use の競合。ここでは閉じていない。閉じるには、
  開いたものを記述子から判定する必要がある）。

## References

- 組織 ADR-021（ファイル渡し MCP サーバーの work dir 契約）
- ADR-0013（work dir 契約）: 検証の閉じた一覧 —— その実装をここで置き換える
- nlink-jp/pathguard の RFP（`docs/ja/pathguard-rfp.ja.md`）
