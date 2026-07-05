# エージェント向けワークフロー手順書(Claude Code / Cowork)

小説・物語の原稿からラジオドラマ/朗読音声を制作するときの、エージェント用
標準手順。**この文書はエージェントに直接読ませる前提で書かれている**
(ユーザーは §1 の依頼文をコピペするだけでよい)。同梱の multi-actor-narration
スキル(audio-drama フォーマット)は本手順書を正として作られている。

## 1. ユーザー向け: 依頼文テンプレート

MCP サーバー `voice-studio` を登録済みの Claude Code / Cowork に、
次のように依頼する(原稿ファイルは適宜差し替え):

> `samples/manuscript.ja.md` の原稿をラジオドラマ音声にしてください。
> 手順は voice-studio-mcp の `docs/ja/reference/agent-workflow.ja.md` に
> 従ってください。workspace_id は `my-drama` で。
> キャスティングは決定前に私に確認してください。

## 2. エージェント向け: 標準手順(10ステップ)

### Step 1 — 環境確認

`list_speakers` を呼ぶ。**自分のファイル書き込みがサンドボックス化されて
いる場合**(プロジェクトツリーのみ書き込み可など)は、プロジェクト内に
ワークスペース用ディレクトリを作り、その絶対パスを以後の全ワークスペース
系ツールに `workspace_root` として渡す(ADR-0010。コール単位パラメータで
あり、サーバー状態にはならない)。エラー(`engine_unavailable`)ならユーザーに
AivisSpeech の導入状況を確認(セットアップは
[setup.ja.md](setup.ja.md) 参照)。話者一覧と各 `license.status` を控える。

### Step 2 — 原稿の分析と脚本化

原稿を読み、以下を行う:

- **シーン分割**: 場面転換で `scene` 番号を割り当てる(m4b のチャプター境界になる)
- **セリフ抽出と話者帰属**: 「」内の発話者を文脈から特定。**確信が持てない
  帰属はユーザーに確認する**(誤帰属は完成後に発覚すると手戻りが大きい)
- **地の文のナレーション化**: 語り手(ナレーター)のセリフとして扱う。
  長すぎる地の文は聴きやすい長さ(1行 100〜150 字目安)に分割する

### Step 3 — キャスティング(人間チェックポイント①)

`list_speakers` の結果から配役案を作り、**ユーザーに提示して承認を得る**:

- ナレーター: 落ち着いたスタイルの声
- 各キャラクター: 性別・年齢感・性格に合う声とスタイル
- `license.status` が `verified` 以外のモデルを使う場合はその旨を明示し、
  規約確認をユーザーに促す。`declared` なら宣言済みライセンス名を添え、
  `voice-studio-mcp licenses --full <uuid>` で全文を読み `licenses --toml`
  で config 記録を作る手順を案内する(ADR-0008)

承認後、workspace 直下に `casting.toml` を書く(形式は README 参照)。
確認が取れたキャラクターのみ `license_checked = true` にする。

### Step 4 — 読み辞書の作成

原稿から誤読しそうな語を抽出する: 人名・地名・造語・当て字・特殊な読みの
熟語。読み(**カタカナ**)とアクセント位置を決め、`register_dictionary` で
一括登録する。自信がない読みはユーザーに確認する。

### Step 5 — 台本 JSONL の作成

workspace の `script/` に台本を書く。1 行 = 1 発話:

```jsonl
{"id":1,"scene":1,"speaker":"ナレーター","text":"...","speed":0.95,"pause_after_ms":800}
{"id":2,"scene":1,"speaker":"美咲","text":"...","style":"悲しみ","intensity":1.4}
```

演技指定の目安:

| パラメータ | 目安 |
|-----------|------|
| `intensity`(感情の強さ 0–2) | 平静 1.0 / 抑えた感情 0.7–0.9 / 高ぶり 1.3–1.6 / 激情 1.7–2.0 |
| `speed`(話速 0.5–2) | ナレーション 0.9–1.0 / 通常会話 1.0 / 焦り・興奮 1.05–1.2 / 回想・余韻 0.85–0.95 |
| `volume`(相対バランス 0–2) | 通常 1.0 / ささやき 0.6–0.8 / 叫び 1.2–1.5(全体音量は master が正規化する) |
| `pause_after_ms` | 会話の間 300–600 / 段落の切れ目 700–1000 / シーン転換 1200–2000 |

`id` は 1 から通し番号。**後から行を挿入する場合も既存 id は変えない**
(キャッシュとリテイクの安定性のため。挿入は末尾の空き番号か 10 刻み採番で)。

### Step 6 — 声のオーディション(任意)

配役に迷いがある場合、`synthesize_line` に `style_id` を直接指定して
同一テキストを数話者で合成し、ユーザーに聴き比べてもらう。

### Step 7 — 一括合成

`synthesize_script` を呼ぶ。`invalid_script` / `casting_unresolved` が
返ったら details の指摘(全件入っている)を**一括で**修正して再実行する。
成功したら `job_id` を控え、`check_job` を state が `running` でなくなる
までポーリング(数十行なら 5–10 秒間隔、長編は 30 秒間隔で十分)。

- `failed > 0`: failures の各 code を見て対処(下の分岐表)。該当行を
  修正して `synthesize_script` を再実行(キャッシュにより失敗行だけ走る)
- `job_not_found`: サーバー再起動があった。そのまま再実行してよい

### Step 8 — 実聴レビュー(人間チェックポイント②)

行 WAV(`wav/<id>.wav`)またはこの段階で一度 `master` した音声を
ユーザーに聴いてもらい、指摘を反映する:

- 誤読 → Step 4 の辞書に追加し、該当行を `synthesize_line force=true`
- 演技が合わない → 該当行の `intensity`/`speed`/`style` を変えて台本を更新し
  再合成(変更行だけ走る)
- 間が悪い → `pause_after_ms` の調整(**再合成は走らない**。master だけやり直す)

### Step 9 — マスタリング

`master` を呼ぶ。配信向けは `format: "mp3"`、オーディオブック向けは
`format: "m4b"`(scene 境界がチャプターになる)。返却の
`unverified_models` が空でない場合は**公開前にユーザーへ警告**する。

### Step 10 — 納品

ユーザーに以下を報告して完了:

- マスターファイルのパスと尺
- クレジット(`credits_path` の内容)と、成果物への添付が必要なこと
- 未確認ライセンスの警告(あれば)

## 3. エラー分岐表

| code | エージェントのアクション |
|------|------------------------|
| `invalid_script` | details.errors を全件修正して再実行 |
| `casting_unresolved` | details の unmapped_speakers/styles を casting.toml に追加 |
| `engine_unavailable` | ユーザーに AivisSpeech の状態を確認([setup.ja.md](setup.ja.md) §7) |
| `engine_request_failed` | 該当行のテキストに異常がないか確認(空・制御文字等)。稀な一時失敗は再実行 |
| `job_not_found` | `synthesize_script` を再実行(キャッシュで差分のみ) |
| `master_incomplete` | details.missing_line_ids を合成してから再実行 |
| `ffmpeg_not_found` | ユーザーに `brew install ffmpeg` を案内 |
| `path_not_allowed` | パスは workspace 相対で指定し直す |

## 4. してはいけないこと

- 台本の途中行の `id` を振り直す(キャッシュ全滅+リテイク履歴の混乱)
- `license_checked` を確認なしに `true` にする(規約確認は人間の責務)
- 音声バイトの取得を試みる(ツールはパスしか返さない。再生はユーザーの環境で)
- 全体音量を `volume` で調整する(master の `loudnorm_i` の仕事。
  [ADR-0006](../adr/0006-mastering-pipeline.ja.md))

## 5. サンプル素材

リポジトリの [`samples/`](../../../samples/) に一式がある:

- `manuscript.ja.md` — 短編サンプル原稿(このワークフローの入力)
- `script/yoiyami.jsonl` — それを台本化した例(Step 2/5 の出力見本)
- `casting.example.toml` — キャスティング表の見本

動作確認だけしたい場合は Step 2–5 を飛ばし、samples の台本と casting を
workspace にコピーして Step 7 から始めればよい。
