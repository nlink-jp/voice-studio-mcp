# 共通制作パイプライン（voice-studio）

4種の音声解説ワークフロー（対談 / パネル / ブリーフィング / 教材）が
共有する **voice-studio MCP** の制作パイプライン。各 FORMAT.md は「資料→台本」
の変換部分だけを固有に持ち、合成〜納品のメカニクスはこのファイルを参照する。

使用ツール: `list_speakers` `register_dictionary` `synthesize_script`
`synthesize_line` `check_job` `master`

### ワークスペース

制作状態はすべて `<workspace_root>/<workspace_id>/` に置かれる。台本 JSONL は
`<workspace>/script/`、キャストは `<workspace>/casting.toml`、音声は
`<workspace>/wav/<id>.wav`、成果物は `<workspace>/master/` に出る。

- `workspace_id`: 作品（回・エピソード）ごとに1つ。`[a-zA-Z0-9_-]{1,64}`。
- `workspace_root`（各ワークスペース系ツールで**任意**）: **自分が書き込める
  絶対パスのディレクトリ**を自前のファイルツールで先に作り、以降**すべての
  呼び出しで同じ値を渡す**。省略するとサーバ既定の `~/.voice-studio` を使う
  （サーバと自分が無制限のファイルビューを共有している場合のみ機能する）。
- サーバはワークスペース外を読み書きしない（カーネルで強制。ワークスペース内
  から外を指すシンボリックリンクは `path_not_allowed` で拒否される）。

> Cowork など、サーバと共有するファイルビューが限定される環境では、**プロジェクト
> 配下（例: ユーザが選択したフォルダ）に `workspace_root` を作って毎回渡す**こと。
> 既定の `~/.voice-studio` に依存しないため、環境をまたいでも動く。

---

## P0. 環境チェックとワークスペース準備（最初に必ず）

`list_speakers` を呼ぶ。`engine_unavailable` が返ったら **停止**し、
AivisSpeech エンジンの起動方法をユーザに案内する。各話者の styles と
`license.status` を控える。話者一覧のスナップショットと役割別の推奨は
[VOICE-CATALOG.md](./VOICE-CATALOG.md) を参照。

あわせて `workspace_id` を決め（作品ごとに1つ、`[a-zA-Z0-9_-]{1,64}`）、
**書き込める絶対パスの `workspace_root` ディレクトリを自前で作成**する
（プロジェクト配下やユーザ選択フォルダを推奨）。以降 `synthesize_script` /
`synthesize_line` / `register_dictionary` / `master` などすべての呼び出しで
`workspace_root` と `workspace_id` を同じ値で渡す。既定 `~/.voice-studio` に
頼れるのはサーバと無制限のファイルビューを共有している場合のみ。

## P1. 資料の取り込みと構成

入力タイプ別の読み取り・要点抽出・構成の手順は
[SOURCE-INGEST.md](./SOURCE-INGEST.md) に集約。各フォーマットの FORMAT.md が
定義する「番組構成」に沿って、抽出した要点を並べ替える。

## P2. キャスティング（人間チェックポイント — 必須）

`list_speakers` の結果からキャスティング案を作り、**`casting.toml` を書く前に
ユーザへ提示して承認を得る**。

- ナレーター/司会は落ち着いた安定声、他の役は性別・年齢・キャラに合わせる。
  モデル名は当てにならないことがある。迷ったら P5 でオーディション。
- `license.status` が `verified` でない場合はその旨を伝える。**商用配信では
  `commercial_use = false`（Non-Commercial 系）や ShareAlike 義務のある
  CC BY-SA を除外**。各役の `credit` は必ず控え、`license_checked` は
  ユーザ確認後にのみ true にする。

承認後 `casting.toml` を `<workspace>/casting.toml` に書く（各 FORMAT.md の
`casting.template.toml` を雛形に）。ファイルは自前のツールで `workspace_root`
配下に直接書き込む。

## P3. 発音辞書

誤読しそうな語（固有名詞・専門用語・略語・英字）を抽出。読みをユーザまたは
資料で確認し `register_dictionary` を呼ぶ。読みは**カタカナ**、
`accent_type` は下がる直前のモーラ位置（0=平板）。
専門文書ほど用語が多いので、辞書登録は品質の要。

## P4. 台本 JSONL を書く

スキーマと演出パラメータは [SCRIPT-SCHEMA.md](./SCRIPT-SCHEMA.md)。
`<workspace>/script/` に 1 行 1 発話。id は 10 刻み（挿入余地）。
**既存 id は絶対にリナンバーしない**（合成キャッシュとリテイクのキー）。

## P5. ボイスオーディション（任意）

`synthesize_line` に明示 `style_id` を与え、同じ試聴用テキストを複数候補声で
レンダリングしてユーザに選んでもらう。司会と解説役の声質差を確認するのに有効。

## P6. バッチ合成

`synthesize_script` を実行。`invalid_script` / `casting_unresolved` は
**details のエラーを一度に全部直して**再実行。`check_job` を state が
`running` を抜けるまでポーリング（短編 5–10 秒 / 長編 30 秒目安）。
失敗行は[エラー対処](#エラー対処)に従い修正して再実行（キャッシュ差分合成）。

## P7. 試聴レビュー（人間チェックポイント — 必須）

`wav/<id>.wav` か暫定 `master` でユーザに聴いてもらい反映:

- 誤読 → 辞書を追加し `synthesize_line force=true` で再合成
- 掛け合い/口調のズレ → `intensity`/`speed`/`style` を調整し差分再合成
- 間・テンポ → `pause_after_ms` を調整（**再合成不要、再マスターのみ**）

## P8. マスタリング

`master` を `format: "mp3"`（配信）か `"m4b"`（章付き。scene が章になる）で実行。
`unverified_models` が非空なら**公開前にユーザへ警告**。

## P9. 納品

master のパスと尺、`credits_path` の内容（**配布物に必ず同梱**）、
ライセンス警告を報告。

---

## エラー対処

| code | 対応 |
|------|------|
| `invalid_script` | details.errors を全件直して再実行 |
| `casting_unresolved` | 未マップの speaker/style を casting.toml に追加 |
| `engine_unavailable` | AivisSpeech の起動 / engine.command を確認 |
| `engine_request_failed` | 行テキストを確認。一時的なら再試行 |
| `job_not_found` | サーバ再起動。synthesize_script を再実行（キャッシュ差分） |
| `master_incomplete` | details.missing_line_ids を先に合成 |
| `ffmpeg_not_found` | `brew install ffmpeg` |
| `path_not_allowed` | ワークスペース相対パス、または有効な `workspace_root`（絶対パス）を渡す。外を指すシンボリックリンクは不可 |
| `invalid_workspace_id` | `[a-zA-Z0-9_-]{1,64}` に合わせる |

## 禁止事項

- 既存 script id のリナンバー（キャッシュ消失・リテイク混乱）
- ユーザ確認なしの `license_checked = true`
- 音声バイト列の取得（ツールはパスを返す。再生はユーザの仕事）
- `volume` で全体音量を調整（それは `master.loudnorm_i`）
- 2 つの人間チェックポイント（キャスティング承認・試聴レビュー）の省略
