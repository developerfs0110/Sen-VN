---
name: voicestudio
description: Generate local speech with saved or designed voices, transcribe audio, and automate dubbing or narration workflows through VoiceStudio's REST API or MCP server. Use when the user wants to work with VoiceStudio audio or connect it to an agent or automation.
---

# VoiceStudio — Local AI Voice, Narration & Dubbing (`voicestudio`)

Use the user's running [VoiceStudio](https://github.com/debpalash/VoiceStudio) backend for local TTS narration, transcription, and dubbing while keeping raw source media untouched.

## 1. Connect & Discover (`http://localhost:3900` Default)
1. Check `GET /health` on the configured VoiceStudio backend (`http://localhost:3900` unless overridden by the user).
2. Discover active endpoints with `GET /openapi.json` and available voices/profiles via `GET /v1/audio/voices`. Never invent voice profile IDs.
3. Prefer offline local models; never switch to paid cloud providers, download large models, or disable authentication on `401/403` without explicit user approval. Never print bearer tokens or API keys in logs.

## 2. Speech Generation & Narration Export
Save the request payload cleanly and write generated audio to a dedicated output directory (e.g. `audio/narration.wav` — never inside `raw/` or `master/`):
```json
{
  "model": "tts-1",
  "voice": "<discovered-voice-id>",
  "input": "Xin chào, đây là bản thuyết minh tự động.",
  "response_format": "wav"
}
```
```bash
curl -fsS http://localhost:3900/v1/audio/speech \
  -H "Content-Type: application/json" \
  -d @speech-request.json \
  --output audio/narration.wav
```

## 3. Transcription, Subtitle Sync & Dubbing Readiness
- Before dubbing export, check for missing/failed segments and timing overflow; preserve background music stems when requested and export dubbed audio/video as a separate artifact.
- Verify every generated `.wav`/`.mp3`/`.mp4` output via `sen verify`:
  ```bash
  sen verify <task_id> -- ffprobe -v error -print_format json -show_format -show_streams audio/narration.wav
  ```

