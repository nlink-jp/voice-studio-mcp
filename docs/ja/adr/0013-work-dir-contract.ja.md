# ADR-0013: work dir は呼び出しごとの `work_dir`、既定ルートは持たない

- **Status**: Accepted (2026-09-13)
- **Amends**: [ADR-0010](0010-agent-prepared-workspaces.ja.md)（agent-prepared workspaces）

## 背景

組織 ADR-021（ファイル渡し MCP サーバーの work dir 契約）をこのサーバーに適用する。
ADR-0010 は「サーバーはエージェントが用意した仕事場で働く」を決めたが、引数名を
`workspace_root` とし、**省略時は `~/.voice-studio` にフォールバック**していた。
そこは呼び出し側のファイルツールが開けない場所で、失敗は「合成は成功し、返った
パスだけが開けない」という形でしか現れない。

フリート全体では同じ意味の引数が 3 綴りに割れており、4 ランタイムの実測では MCP の
`roots` も環境変数も半数には届かない。**呼び出しごとの引数だけが共通の経路**である。

## 決定

1. **引数は `work_dir`、4 ツール（synthesize_script / synthesize_line /
   register_dictionary / master）すべてで必須。** 意味は「呼び出し側が読み戻せる
   絶対パス」。ワークスペースは `<work_dir>/<workspace_id>/`。
2. **解決順は 引数 → `_meta["jp.nlink/work_dir"]` → エラー。** 既定ルートは持たない。
   `Manager` の既定ルート操作（`Ensure` / `Root` / `List` / `Delete`）も削除する。
3. **検証は閉じた一覧**（絶対 / `~` 無し / `..` 無し / 存在する dir / 書込可 /
   システム・資格情報の位置でない）。`work_dir_*` の 5 コード。dir は作らない。
4. ワークスペース内のパス封じ込め（`os.Root`）は ADR-0010 のまま変えない。
5. **メディア連鎖では同じ `work_dir` を共有する。** image-forge がページ画像を、
   ここが音声を、video-studio が mux を、1 つのディレクトリの下で行う。
   `<work_dir>/<workspace_id>/` の名前空間共有は意図的な設計である。

## 影響

- **破壊的。** `workspace_root` を送る呼び出しは新しい名前を告げて拒否される
- `internal/workdir` は voice-scribe / pcap-analyzer / gem-scribe / image-forge と
  同一ファイル（移植物）
- `~/.voice-studio` は使われなくなる（既存の中身はそのまま残る）

## 参照

- 組織 ADR-021、voice-scribe ADR-0010（参照実装）、pcap-analyzer-mcp ADR-0008
- [ADR-0010](0010-agent-prepared-workspaces.ja.md) — 本 ADR が引数名と既定を置き換える
