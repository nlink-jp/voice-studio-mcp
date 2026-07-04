# voice-studio-mcp セットアップガイド

ゼロから「最初のラジオドラマ音声」ができるまでの手順。

## 1. 前提条件

| 要件 | 内容 |
|------|------|
| OS | macOS 13+ / Apple Silicon(v1 対象) |
| AivisSpeech | https://aivis-project.com/ から .dmg を導入。**初回は GUI を一度起動**してセットアップ(デフォルト音声モデルの準備)を完了させること |
| ffmpeg | `brew install ffmpeg`(`master` ツールのみ使用) |
| Go 1.23+ | ソースからビルドする場合のみ |

> AivisSpeech の GUI を一度も起動していないと、エンジンがモデル未取得の
> まま起動に失敗することがある。`doctor` で確認できる。

## 2. ビルドと診断

```sh
git clone https://github.com/nlink-jp/voice-studio-mcp.git
cd voice-studio-mcp
make build            # → dist/voice-studio-mcp(darwin では自動 codesign)
dist/voice-studio-mcp doctor
```

`doctor` の見方:

```
ok config: built-in defaults (no config.toml found)   ← config なしでも動く
ok engine command: /Applications/AivisSpeech.app/...  ← AivisSpeech 検出
ok engine: not running at http://127.0.0.1:10101 (serve will spawn it)
ok ffmpeg: ffmpeg
ok workspace dir: /Users/you/.voice-studio
```

`NG` が出た行のメッセージに対処方法が書かれている(AivisSpeech 未導入、
ffmpeg 不在など)。

## 3. 設定(任意)

すべて既定値で動くため、config なしで開始できる。カスタマイズする場合:

```sh
mkdir -p ~/.config/voice-studio-mcp
cp config.example.toml ~/.config/voice-studio-mcp/config.toml
```

よく触る項目:

- `master.loudnorm_i` — 完成音声の聴感音量(既定 -18 LUFS。大きいと感じたら
  -19〜-20 へ)
- `synthesis.concurrency` — 並列合成数(CPU 推論のため既定 1。M3 以上なら
  2 を試す価値あり)
- `[[speaker_metadata]]` — 導入した音声モデルの規約記録(§6)

複数設定は `--config <path>` で切り替え。

## 4. MCP クライアントへの登録

### Claude Code

```sh
claude mcp add voice-studio -- /path/to/dist/voice-studio-mcp serve
```

またはプロジェクトの `.mcp.json`:

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

### Claude Desktop / Cowork

`claude_desktop_config.json` の `mcpServers` に同じエントリを追加。

> 初回接続時はエンジンの起動待ち(モデルロード)で initialize の応答まで
> 数十秒〜数分かかることがある。2 回目以降は数秒。

## 5. 最初の制作(手動ウォークスルー)

エージェントに任せる場合はこの節は不要(ツール説明だけで進められる)が、
仕組みの理解には一度手で辿るのが早い。

```sh
# 1. ワークスペースの入力を用意
mkdir -p ~/.voice-studio/demo/script
cat > ~/.voice-studio/demo/casting.toml <<'EOF'
[characters."ナレーター"]
speaker_uuid = "<list_speakers で取得>"
style_id = 888753760
credit = "AivisSpeech:Anneli"
license_checked = true
EOF

cat > ~/.voice-studio/demo/script/demo.jsonl <<'EOF'
{"id":1,"speaker":"ナレーター","text":"これは最初のテストです。","pause_after_ms":500}
{"id":2,"speaker":"ナレーター","text":"うまく聞こえていますか。"}
EOF
```

MCP クライアントから(またはエージェントへの指示として):

1. `list_speakers` — speaker_uuid / style_id を確認して casting.toml に反映
2. `register_dictionary` — 固有名詞があれば読みを登録
3. `synthesize_script` `{workspace_id: "demo", script_path: "script/demo.jsonl"}`
4. `check_job` — state が done になるまでポーリング
5. `master` `{workspace_id: "demo", script_path: "script/demo.jsonl", format: "mp3"}`
6. `~/.voice-studio/demo/master/demo.mp3` を再生

## 6. 音声モデルの追加と規約記録

1. AivisSpeech の GUI(設定 → 音声合成モデルの管理)で
   [AivisHub](https://hub.aivis-project.com/) からモデルを追加。
2. **モデルページの利用規約を読み**、公開用途で使えるか確認する。
3. 確認結果を config に記録:

```toml
[[speaker_metadata]]
speaker_uuid = "..."         # list_speakers で確認
name = "モデル名"
license = "ACML 1.0"
license_url = "https://hub.aivis-project.com/aivm-models/..."
credit = "AivisSpeech:モデル名"
commercial_use = true
```

4. 作品で使う際は casting.toml の `license_checked = true` を立てる。
   立てないと `master` が `unverified_models` として警告する(意図的な摩擦)。

## 7. トラブルシューティング

| 症状 | 対処 |
|------|------|
| initialize がタイムアウト | 初回モデルロード中。`engine.startup_timeout_seconds` を延ばす。GUI を一度起動してモデル取得を済ませる |
| `engine_unavailable: engine command not found` | AivisSpeech 未導入、または非標準パス → `engine.command` を設定 |
| `ffmpeg_not_found` | `brew install ffmpeg` または `master.ffmpeg_path` に絶対パス |
| `job_not_found` | サーバー再起動でジョブは消える。`synthesize_script` を再実行(キャッシュで差分のみ合成) |
| `master_incomplete` | details の missing_line_ids を `synthesize_script` で合成してから再実行 |
| 完成音声が大きい/小さい | `master.loudnorm_i` を調整(-18 既定、下げるほど小さい)。行間バランスは台本の `volume` |
| 特定の固有名詞を誤読 | `register_dictionary` で読み(カタカナ)+アクセント位置を登録し、該当行を `synthesize_line force=true` で作り直し |
| GUI のエンジンと衝突しないか | しない。起動済みエンジンには attach し、所有権を取らない(ADR-0002) |

## 8. アンインストール

```sh
rm -rf ~/.voice-studio          # 全ワークスペース(音声・キャッシュ)
rm -rf ~/.config/voice-studio-mcp
```

エンジンのユーザー辞書に登録した語は AivisSpeech 側に残る(GUI の
辞書設定から削除可能)。
