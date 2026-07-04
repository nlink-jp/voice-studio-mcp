# samples/ — workflow sample set / ワークフロー確認用サンプル

End-to-end sample material for the agent workflow described in
[`docs/en/reference/agent-workflow.md`](../docs/en/reference/agent-workflow.md)
([日本語](../docs/ja/reference/agent-workflow.ja.md)).

| File | Role |
|------|------|
| `manuscript.ja.md` | Sample manuscript (the workflow's input; CC0) |
| `script/yoiyami.jsonl` | Reference script JSONL — what the agent should produce from the manuscript |
| `casting.example.toml` | Casting table template (**replace UUIDs/style ids via `list_speakers` first**) |

## Quick check without the authoring steps / 台本化を省いた動作確認

```sh
mkdir -p ~/.voice-studio/sample/script
cp samples/script/yoiyami.jsonl ~/.voice-studio/sample/script/
cp samples/casting.example.toml ~/.voice-studio/sample/casting.toml
# casting.toml の speaker_uuid / style_id を list_speakers の実値に書き換えてから:
#   synthesize_script {workspace_id:"sample", script_path:"script/yoiyami.jsonl"}
#   check_job → master {workspace_id:"sample", script_path:"script/yoiyami.jsonl", format:"mp3"}
```

To exercise the full workflow (script conversion included), hand
`manuscript.ja.md` to an agent with the prompt template in the workflow
guide §1.
