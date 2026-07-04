# RFP: voice-studio-mcp

> Generated: 2026-07-04
> Status: Draft

## 1. Problem Statement

When producing radio dramas or narrated audio from novel/story manuscripts, the intellectual work — script conversion, speaker attribution, performance direction — can be handled by Claude Code / Cowork agents, but agents have no "voice." This tool is an MCP server that wraps local TTS engines (AivisSpeech Engine first), providing agents with a speaker catalog (including license metadata), pronunciation dictionary registration, batch script synthesis, and mastering — enabling a fully local, autonomous workflow from manuscript to finished audio. The target user is an individual creator building creative workflows with Claude Code / Cowork (initially the author himself).

Unlike a traditional command-line pipeline (a self-contained CLI with embedded LLM calls), all judgment and creative work stays on the agent side; this tool handles only deterministic heavy lifting (engine management, synthesis, post-processing).

## 2. Functional Specification

### Commands / API Surface

MCP server (stdio transport) exposing 6 tools:

| Tool | Summary |
|------|---------|
| `list_speakers` | Returns speakers + styles from the connected engine, with each voice model's terms of use and credit line included as metadata so the agent can consider licensing during casting |
| `register_dictionary` | Bulk-registers a work-specific pronunciation dictionary (proper nouns, coined words) via the engine's `/user_dict_word` API |
| `synthesize_script` | Batch-synthesizes a script JSONL in the workspace (passed by file path). Line-hash cache re-synthesizes only changed lines. Runs as an async job for long works |
| `synthesize_line` | Single-line synthesis for retakes |
| `check_job` | Returns async job progress and failed lines in structured form |
| `master` | Concatenation, loudness normalization, and mp3/m4b export via ffmpeg. Auto-generates credit text from the terms of the models used |

Following the Go convention: single binary + subcommands (`serve` plus debug subcommands as needed).

### Input / Output

- **Input**:
  - Script JSONL — engine-independent intermediate format. One line = one utterance: `{"id", "scene", "speaker", "text", "style", "intensity", "speed", "pause_after_ms"}`. The canonical schema definition lives in this MCP; the follow-up skill references it
  - Casting table — character name → engine speaker UUID + style ID mapping (with columns for license-check status and credit line)
  - Pronunciation dictionary — list of word / reading / accent entries
- **Output**: per-line WAV files (named by line ID for retakes), master audio (mp3 / m4b), credit text. Tool results never contain audio bytes; they return workspace file paths plus a per-line success/duration summary (token economy)
- **Scoping**: one work = one workspace. `workspace_id` isolates dictionaries, casting tables, synthesis caches, and jobs per work (same approach as data-toolbox-mcp)

### Configuration

- `config.toml` (engine binary path, port, output root, ffmpeg path, etc.), following lite-series config conventions (sectioned TOML)
- `-c` flag to switch between multiple configs (same approach as ask-llm-mcp)

### External Dependencies

- **AivisSpeech Engine** — official binary installed by the user. The MCP server spawns it as a child process, health-checks it, and reaps it on shutdown (default port 10101, VOICEVOX-compatible HTTP API)
- **ffmpeg** — used only by the `master` tool. Must be pre-installed (runtime dependency)
- **Zero Go library dependencies** — net/http + os/exec only. No cloud APIs, no credentials

## 3. Design Decisions

- **Go**: single-binary distribution and the org's zero-external-dependency policy. Ports the data-toolbox-mcp skeleton (workspace scoping, structured errors `{code, message, details}`, surfacing child-process exit status, no internal ID exposure)
- **v1 targets AivisSpeech Engine only**: the API is VOICEVOX-compatible, so the engine abstraction stays in the design while verification is focused on one engine. Adding VOICEVOX ENGINE (wider voice roster) is a future extension
- **MCP manages the engine lifecycle**: spawn if not running, reap on shutdown. Zero preparation work for the agent/user, enabling truly autonomous workflows
- **macOS only (v1)**: engine spawning and model paths are OS-dependent and the primary goal is local macOS production, so v1 focuses on darwin-arm64. Windows / Linux to be considered later
- **Complements existing tools**: the "voice" alongside data-toolbox-mcp (data-analysis hands) and ask-llm-mcp (consultation mouth). The follow-up radio-drama skill (skills-series, separate project) will be its first consumer
- **Explicitly out of scope**: script conversion / voice-script authoring / casting decisions (the agent's job), BGM/SE mixing (post-process in a DAW), voice model training/cloning, cloud TTS, streaming playback, the skills-series skill (separate RFP)

### TTS foundation rationale (researched 2026-07)

Conclusions of a deep-research comparison (25 sources, 24 claims verified 3-0):

- **AivisSpeech Engine** — runs Style-Bert-VITS2-family models in AIVMX (ONNX) format with CPU inference. Officially supports macOS 13+ (Apple Silicon recommended), LGPL-3.0 only. SBV2-family Japanese quality (accent/intonation) is current state of the art. Per-line emotional control via style IDs + `intonationScale` (0.0–2.0)
- Direct Style-Bert-VITS2 use was rejected (macOS unsupported officially + AGPL-3.0). Kokoro-family (MIT/Apache 2.0, fast) was rejected for having only 2 Japanese voices, failing the multi-speaker casting requirement. VOICEVOX remains a future candidate for expanding character voices

## 4. Development Plan

### Phase 1: Core

- config.toml loading; engine lifecycle management (spawn / health check / reap)
- Workspace management (create / list / delete)
- `list_speakers`, `synthesize_line`, `synthesize_script` (synchronous, small-scale)
- Script JSONL schema v1 + casting table definition
- Tests: engine mocked with httptest + dummy MCP client harness

### Phase 2: Features

- Async job execution for `synthesize_script` + `check_job`
- Line-hash cache (differential re-synthesis)
- `register_dictionary`
- `master` (ffmpeg concat / loudness normalization / mp3-m4b) + automatic credit generation
- Speaker license metadata curation

### Phase 3: Release

- E2E verification with a real (short) work
- docs/{en,ja} three layers, README.md / README.ja.md, CHANGELOG.md, AGENTS.md
- make build (darwin-arm64, Developer ID signed + notarized)
- v0.1.0 release, umbrella submodule update, org profile update, check-org.sh

Each phase is independently reviewable.

## 5. Required API Scopes / Permissions

**None.** Fully local operation (engine on localhost; no cloud APIs, OAuth, or credentials).

## 6. Series Placement

Series: **util-series**

Reason: same lineage as data-toolbox-mcp / ask-gemini-mcp / ask-llm-mcp — MCP servers that give agents capabilities. The design is settled and the use case is clear, so util-series rather than lab-series (experimental). Development starts in `_wip/voice-studio-mcp/` and is added to the umbrella as a submodule at integration time.

## 7. External Platform Constraints

- **AivisSpeech Engine constraints**:
  - Non-streaming (returns WAV after full synthesis) — acceptable since this use case is batch generation
  - Some VOICEVOX APIs return `501 Not Implemented` (`/synthesis_morphing`, `/cancellable_synthesis`, etc.)
  - Requires 1.5GB+ RAM; first startup and first model load take time
  - Intel Macs are not actively verified upstream (Apple Silicon assumed)
- **Voice model terms**: each AIVMX model has its own terms of use (separate from the software's LGPL-3.0). Since the output is distributable media, the casting table carries license-check status and `master` auto-generates credits
- **ffmpeg**: must be pre-installed when using `master`

---

## Discussion Log

- **2026-07-04 TTS foundation research**: deep-research workflow (25 sources, 24 claims verified 3-0) compared macOS-local Japanese TTS. Conclusion: AivisSpeech (SBV2 family, LGPL-3.0, official macOS support) for quality-first batch generation; kokoro family for real-time. This project adopts AivisSpeech as it is batch-oriented
- **Architecture pivot**: initially considered a Go CLI pipeline with embedded LLM calls (manuscript → script → voice script → audio); changed to the premise that agents (Claude Code / Cowork) do the script work themselves, and this tool provides only the synthesis part as an MCP server
- **Naming**: compared voice-studio-mcp / audio-drama-mcp / tts-toolbox-mcp / speech-forge-mcp; chose voice-studio-mcp as it conveys the scope beyond synthesis (dictionary, casting info, mastering)
- **v1 engine scope**: AivisSpeech only; the VOICEVOX-compatible API keeps multi-engine extension open
- **Engine lifecycle**: MCP spawns/reaps the engine as a child process (no human preparation in autonomous workflows)
- **master scope**: included in v1 (ffmpeg runtime dependency accepted). BGM/SE mixing explicitly out of scope
- **Skill separation**: this RFP covers the MCP server only. The radio-drama skill (skills-series) with script-conversion procedures is a separate project after the MCP ships. The script JSONL schema is canonically owned by the MCP
- **Platform**: v1 is macOS (darwin-arm64) only, limiting OS branching and verification cost in engine management
- **Workspace adoption**: one work = one workspace, isolating dictionary/casting/cache/jobs (data-toolbox-mcp approach)
- **Series placement**: util-series (consistent with the MCP server lineage)
