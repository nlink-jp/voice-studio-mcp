# voice-studio-mcp

> AIエージェント駆動のラジオドラマ/朗読音声制作のためのローカル音声合成
> MCPサーバー(単一バイナリ、AivisSpeech Engine バックエンド)。

## なぜ作ったか

小説をラジオドラマにする作業は2種類に分かれます。*知的作業*(脚本化・
話者帰属・演技指定)と*機械的作業*(音声合成・リテイク・マスタリング)
です。Claude Code や Cowork のようなAIエージェントは前者を担えますが、
「声」を持ちません。voice-studio-mcp はその声を提供します:
[AivisSpeech Engine](https://github.com/Aivis-Project/AivisSpeech-Engine)
(VOICEVOX互換API、Style-Bert-VITS2系モデル)をラップする完全ローカルの
MCPサーバーで、エージェントが書いた台本を完成音声に変換します。
クラウドAPI・クレデンシャル不要です。

## 特徴

- **規約メタデータ付き話者カタログ** — 各モデルの AIVM マニフェストから
  作者宣言のライセンス全文を自動収集(`declared`)し、人間が確認した
  config 記録を `verified` として区別。`licenses` サブコマンドが宣言を
  確認可能な config スケルトンに変換します。
- **読み辞書** — 作品固有の固有名詞の読みを登録し、キャラクター名の
  誤読を防ぎます。
- **コンテンツハッシュキャッシュ付き一括合成** — 200行の台本のうち
  3行だけ直した場合、再合成されるのはその3行だけです。
- **非同期ジョブ** — 長編はバックグラウンドで合成し、`check_job` で
  進捗を確認します。
- **マスタリング** — ffmpeg による行間ポーズ挿入付き連結、1パス
  ラウドネス正規化(既定 -18 LUFS、設定可)、mp3 / m4b(シーン
  チャプター付き)、クレジットファイル自動生成。
- **エンジンライフサイクル管理** — 起動時に AivisSpeech Engine を
  spawn し終了時に回収(起動済みのエンジンがあれば attach)。

## 動作要件

- macOS(Apple Silicon、v1対象)
- [AivisSpeech](https://aivis-project.com/) インストール済み(エンジン同梱)
- `ffmpeg`(`master` ツールのみ使用): `brew install ffmpeg`
- Go 1.25+(ソースからビルドする場合のみ)

## クイックスタート

```sh
make build                     # → dist/voice-studio-mcp
dist/voice-studio-mcp doctor   # エンジン / ffmpeg / config を診断
```

MCPクライアント(例: Claude Code)への登録:

```json
{
  "mcpServers": {
    "voice-studio": {
      "command": "/path/to/dist/voice-studio-mcp",
      "args": ["serve"]
    }
  }
}
```

設定は `~/.config/voice-studio-mcp/config.toml`
([config.example.toml](config.example.toml) 参照)。すべてに既定値が
あります。

## サブコマンド

| コマンド | 説明 |
|---------|------|
| `serve` | MCP stdioサーバーを起動(サブコマンド省略時の既定) |
| `doctor` | 環境診断(config・エンジン・ffmpeg・workspaceディレクトリ) |
| `licenses` | 音声モデルのライセンス宣言を収集・確認(`--full <uuid>` 全文表示、`--toml` configスケルトン生成) |
| `version` | バージョン表示 |

## ツール

| ツール | 説明 |
|--------|------|
| `get_usage` | 本サーバーの操作マニュアル(ワークスペースモデル・スキーマ・復旧表)。同梱スキルのないクライアントは最初に呼ぶ |
| `list_speakers` | 導入済み音声モデル+スタイル+規約メタデータ |
| `register_dictionary` | 作品固有の読み(カタカナ・アクセント)を登録 |
| `synthesize_script` | 台本JSONLを一括合成 → `wav/<id>.wav`(非同期、`job_id` 返却) |
| `synthesize_line` | 同期の単行合成(リテイク、`style_id` で声の試聴) |
| `check_job` | ジョブ進捗と行単位の失敗情報 |
| `master` | 連結+ラウドネス正規化+mp3/m4b(+チャプター)+クレジット |

ツールの返却はパス・件数・尺のコンパクトなJSONサマリのみで、音声
バイト列は返しません。

## Claude Code スキル(同梱)

リポジトリに **radio-drama** スキルを同梱しています — エージェント
ワークフロー(原稿→キャスティング→辞書→合成→リテイク→マスタリング、
人間チェックポイント必須)の運用形です:

```sh
make install-skill      # → ~/.claude/skills/radio-drama
```

導入後、Claude Code / Cowork に *「/radio-drama samples/manuscript.ja.md」*
のように依頼できます。スキルを別リポジトリでなくここに同梱するのは、
本サーバーのスキーマ・ツールと常に一致させるためです —
`skills/skills_test.go` が整合を機械検査します(ADR-0009)。

### 台本JSONL(canonicalスキーマ)

1行=1発話。このスキーマがエージェント側スキルとの契約です:

```jsonl
{"id":1,"scene":1,"speaker":"narrator","text":"夜のとばりが街を包んでいた。","pause_after_ms":800}
{"id":2,"scene":1,"speaker":"美咲","text":"……本当に、行くの?","style":"悲しみ","intensity":1.4,"speed":0.9}
```

- `id`(必須・一意・>0)— 出力は `wav/<id>.wav`。リテイクは同IDを上書き
- `speaker`(必須)— キャスティング表で解決
- `style` / `intensity`(0〜2)/ `speed`(0.5〜2)/ `volume`(0〜2)—
  演技指定(`volume` は行間の相対バランス用。全体の聴感音量は
  マスタリングの `loudnorm_i` が支配)
- `pause_after_ms` — マスタリング時に挿入する無音(合成キャッシュを
  無効化しない)
- `scene` — m4bのチャプター境界

### キャスティング表(workspace直下の `casting.toml`)

```toml
[characters."narrator"]
speaker_uuid = "..."          # list_speakers で取得
style_id = 888753760          # デフォルトスタイル
credit = "AivisSpeech:Anneli"
license_checked = true        # 人間がモデル規約を確認済み

[characters."美咲"]
speaker_uuid = "..."
style_id = 933744512

[characters."美咲".styles]    # 台本の "style" → スタイルID
"悲しみ" = 933744513
```

### 典型的なエージェントフロー

1. `list_speakers` → キャスティングして `casting.toml` を作成
2. `register_dictionary` → 固有名詞の読み登録
3. 台本JSONLを `<workspace>/script/` に書く
4. `synthesize_script` → `check_job` でポーリング
5. リテイクは `synthesize_line`
6. `master` → mp3/m4b+クレジット

## ワークスペース構成

全ワークスペース系ツールはオプショナルな `workspace_root`(エージェントが
自分の書き込み可能領域 — 例: プロジェクトディレクトリ内 — に用意した
ディレクトリの絶対パス)を受け付けます。書き込みがプロジェクトツリーに
制限されたサンドボックス型 MCP クライアントはこれを使います。サーバーは
ワークスペース外への symlink を決して辿りません(os.Root によるカーネル
強制、ADR-0010)。省略時は既定ルート:

```
~/.voice-studio/<workspace_id>/
├── script/          台本JSONL(エージェントが作成)
├── casting.toml     人物→声のマッピング
├── dict/words.json  登録済み辞書の記録
├── wav/<id>.wav     行単位の合成音声
├── cache/index.json 合成キャッシュインデックス
└── master/          マスター出力+クレジット+tmp
```

## テスト

```sh
make test        # ユニットテスト(AivisSpeech / ffmpeg 不要)
make test-e2e    # ビルド+モックエンジン相手にstdio経由でバイナリを駆動
VOICE_STUDIO_TEST_REAL_ENGINE=1 make test-e2e   # opt-in: 実エンジン
```

## ライセンスに関する注意

サーバーコードはMITです。**音声モデルにはソフトウェアライセンスとは
別の利用規約があります**。確認ワークフロー(ADR-0008):

1. `voice-studio-mcp licenses` — 導入済み各モデルの宣言状況を一覧
2. `voice-studio-mcp licenses --full <speaker_uuid>` — 規約全文を読む
3. `voice-studio-mcp licenses --toml` — `[[speaker_metadata]]` スケルトンを
   生成。`license_url` / `commercial_use` を埋め、承諾したら REVIEW ノートを
   削除
4. 作品ごとに `casting.toml` の `license_checked = true` を立て、生成された
   クレジットファイルを成果物に添付

`list_speakers` は `verified`(人間確認済み)/ `declared`(マニフェストに
宣言あり・未確認)/ `unverified` を返します。公開音声に使ってよいのは
`verified` のモデルだけです。

レジストリは**ユーザーデータ**です: ユーザー config に置かれ、あなたの
マシンの導入モデルとあなた自身の確認結果を反映するもので、共有リポジトリに
コミットしてはいけません。speaker_uuid はモデル固有のグローバルIDなので、
個人の dotfiles 等で自分のマシン間を同期するのは問題ありません。

## ドキュメント

- [`docs/ja/reference/agent-workflow.ja.md`](docs/ja/reference/agent-workflow.ja.md) — エージェント向けワークフロー手順書(Claude Code / Cowork にそのまま渡す。サンプル素材は [`samples/`](samples/))
- [`docs/ja/reference/setup.ja.md`](docs/ja/reference/setup.ja.md) — セットアップガイド(導入→最初の制作→トラブルシューティング)
- [`docs/ja/reference/architecture.ja.md`](docs/ja/reference/architecture.ja.md) — アーキテクチャ概観と設計判断の索引
- [`docs/ja/adr/`](docs/ja/adr/) — 非自明な設計の「なぜ」を記録した ADR 10本
- [`docs/ja/voice-studio-mcp-rfp.ja.md`](docs/ja/voice-studio-mcp-rfp.ja.md) — RFP
- English: [`docs/en/`](docs/en/)(setup / architecture / ADR / RFP)

## License

MIT — [LICENSE](LICENSE) 参照。
