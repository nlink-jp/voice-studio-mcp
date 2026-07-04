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

- **規約メタデータ付き話者カタログ** — エンジンAPIはモデルの利用規約を
  返さないため、人手管理のconfigセクションを突合。未確認モデルは
  `unverified` として公開前に警告します。
- **読み辞書** — 作品固有の固有名詞の読みを登録し、キャラクター名の
  誤読を防ぎます。
- **コンテンツハッシュキャッシュ付き一括合成** — 200行の台本のうち
  3行だけ直した場合、再合成されるのはその3行だけです。
- **非同期ジョブ** — 長編はバックグラウンドで合成し、`check_job` で
  進捗を確認します。
- **マスタリング** — ffmpeg による行間ポーズ挿入付き連結、1パス
  ラウドネス正規化(既定 -16 LUFS)、mp3 / m4b(シーンチャプター付き)、
  クレジットファイル自動生成。
- **エンジンライフサイクル管理** — 起動時に AivisSpeech Engine を
  spawn し終了時に回収(起動済みのエンジンがあれば attach)。

## 動作要件

- macOS(Apple Silicon、v1対象)
- [AivisSpeech](https://aivis-project.com/) インストール済み(エンジン同梱)
- `ffmpeg`(`master` ツールのみ使用): `brew install ffmpeg`

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
| `version` | バージョン表示 |

## ツール

| ツール | 説明 |
|--------|------|
| `list_speakers` | 導入済み音声モデル+スタイル+規約メタデータ |
| `register_dictionary` | 作品固有の読み(カタカナ・アクセント)を登録 |
| `synthesize_script` | 台本JSONLを一括合成 → `wav/<id>.wav`(非同期、`job_id` 返却) |
| `synthesize_line` | 同期の単行合成(リテイク、`style_id` で声の試聴) |
| `check_job` | ジョブ進捗と行単位の失敗情報 |
| `master` | 連結+ラウドネス正規化+mp3/m4b(+チャプター)+クレジット |

ツールの返却はパス・件数・尺のコンパクトなJSONサマリのみで、音声
バイト列は返しません。

### 台本JSONL(canonicalスキーマ)

1行=1発話。このスキーマがエージェント側スキルとの契約です:

```jsonl
{"id":1,"scene":1,"speaker":"narrator","text":"夜のとばりが街を包んでいた。","pause_after_ms":800}
{"id":2,"scene":1,"speaker":"美咲","text":"……本当に、行くの?","style":"悲しみ","intensity":1.4,"speed":0.9}
```

- `id`(必須・一意・>0)— 出力は `wav/<id>.wav`。リテイクは同IDを上書き
- `speaker`(必須)— キャスティング表で解決
- `style` / `intensity`(0〜2)/ `speed`(0.5〜2)— 演技指定
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
別の利用規約があります**: 音声を公開する前に各モデル(AivisHub等)の
規約を確認し、結果を `casting.toml`(`license_checked`)と
`[[speaker_metadata]]` に記録し、生成されたクレジットファイルを
成果物に添付してください。

## ドキュメント

- `docs/ja/voice-studio-mcp-rfp.ja.md` — RFP(設計判断の正)
- `docs/en/voice-studio-mcp-rfp.md` — English RFP

## License

MIT — [LICENSE](LICENSE) 参照。
