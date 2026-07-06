# ADR-0012: 未署名エンジンの起動失敗はローカル自己修復(doctor --fix)で解く

- **Status**: Accepted (2026-07-05)

## Context

AivisSpeech は**コード署名も notarize もされていない** PyInstaller バンドルで
配布される。中身のネイティブ拡張(onnxruntime, numpy 等)は各配布元の
**異なる TeamID の署名**を持ち、外側の実行体は未署名。現代の macOS
(特に arm64)はロードする全 Mach-O に一貫した署名を要求するため、この
「署名のまだら」が**ロード時に Library Validation / cdhash / TeamID 不一致で
kill される**。

現場報告で判明した重要事実:

- 我々は managed モードで engine の `run` を直接 exec する。これは GUI 起動
  時の LaunchServices 承認(quarantine を外す)を迂回するため、承認されて
  いないマシンで失敗する。
- **quarantine を外すだけでは不十分な環境がある**(レベル2)。署名の不整合
  そのものが原因で、`xattr -dr com.apple.quarantine` 後もなお起動しない。
- 手動で **`codesign --force --sign -`(ad-hoc)で全ネストを一貫再署名**して
  ようやく起動した環境がある。ad-hoc に揃えると TeamID 不一致と Library
  Validation 制約が消えるため。

## Decision

配布物を我々が抱える(fork/notarize/再実装)のではなく、**ユーザーマシン上で
署名状態を自己修復する** `doctor --fix` を提供する。これはユーザーが手で
実証した手順の自動化にすぎない。

- **検出(読み取り専用)**: 素の `doctor` は engine サブツリーに quarantine が
  あるか、`codesign --verify` が通るかを検査し「要修復・`doctor --fix` で修復可」
  と報告する(doctor 自体は失敗させない)。
- **修復(`doctor --fix` でのみ、mutating)**:
  1. `xattr -dr com.apple.quarantine <engine-dir>` で quarantine を再帰除去。
  2. サブツリー内の Mach-O を magic bytes で列挙し、**深い順(inside-out)に
     各 `.dylib`/`.so` → `run` の順で `codesign --force --sign -`**。全署名を
     ad-hoc(null TeamID)に揃え、TeamID 不一致と Library Validation を解消。
- **スコープは engine サブツリーのみ**(`engine.command` の親ディレクトリ)。
  我々が exec するのはそこだけで、Electron の GUI .app には触らない。バンドル
  外へ roam しない。
- `--deep` は使わない(Apple 非推奨・ネスト署名が不完全でまだら再発する)。
  inside-out に自前で列挙・署名する。
- supervisor の spawn 早期終了エラーに `doctor --fix` への誘導を載せる。

## Consequences

- **署名問題を最小コストで解決**: fork も notarize も 1GB 配布も LGPL/モデル
  再頒布の宿題も不要。ローカル単一ユーザーの macOS ツールという脅威モデルに
  対して過不足ない。
- ユーザーマシンの `/Applications` 配下を書き換えるため、**opt-in**(`--fix`)。
  素の doctor は読み取り専用を維持。
- AivisSpeech 更新で quarantine/署名が戻るため、更新後は再実行が必要
  (doctor が再検出して案内)。
- 検出は 2500 ファイル規模のサブツリーを walk するが magic bytes 読みのみで
  高速。
- Runner interface 経由で `xattr`/`codesign` を実行し、fake runner で
  「inside-out の順序・force ad-hoc・quarantine 除去・冪等・失敗表面化」を
  ハーメチックにテスト(AivisSpeech/macOS 不要)。

## Alternatives considered

1. **署名オーバーレイ fork**(ADR 検討: AivisSpeech-Engine を自ビルド→自 ID で
   inside-out 署名→notarize→エンジンのみ DL アセット化): 構造的に正しく
   ユーザーマシンを触らないが、PyInstaller notarize のスパイク・1GB 配布・
   LGPL 対応ソース提供・同梱モデル選別・upstream 追随という恒常コストが
   大きい。**将来 upstream が notarize しない/需要が広がった場合の第2段**
   として保留。現段階では自己修復で十分。
2. **フル同梱**(エンジン+モデルを本体に): 上記コスト全部+モデル非商用
   ライセンス衝突。不採用。
3. **CGO で推論を再実装**(image-forge 方式): SBV2 には成熟した C++ 移植が
   なく、日本語 G2P + BERT + SBV2 を自前保守することになる。署名問題の解と
   しては遠回りすぎ。不採用。
4. **quarantine 除去のみ案内**: レベル2環境(署名不整合)を救えない。不採用。
5. **`codesign --deep`**: Apple 非推奨で外→内署名が不完全。不採用。

## 上流への姿勢

fork/notarize を避けた本決定と両立して、**upstream が notarize してくれれば
本修復は不要になる**のが理想。AivisSpeech へ「未署名バンドルがエージェント
spawn 用途で壊れる」旨を礼儀として伝える価値がある。
