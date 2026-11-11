---
name: video-subtitles-effects
description: Non-destructive video editing templates, smooth transitions, Vietnamese UTF-8 ASS/SRT subtitle burn-in, -14 LUFS loudness normalization, and ffprobe verification with Sen-VN.
---

# Video Editing, Subtitles & Effects (`video-subtitles-effects`)

Follow these production rules whenever editing video, burning Vietnamese subtitles, applying transitions, or exporting social/cinema renders in a **Sen-VN** workspace.

## 1. Non-Negotiable Raw Footage Immutability
1. **Never Touch Raw Folders:** Files inside `raw/`, `master/`, `footage/`, `originals/`, and `assets/raw/` are **immutable** (`is_locked_test = 1` in `.sen/state.db`).
2. **Separate Output Paths:** Declare ONLY output/script/subtitle paths in `sen begin <task_id> --reqs R1 --files edits/final.mp4,subs/vi.ass`. Never pass the `-i` input path as the `ffmpeg` output path.

## 2. Vietnamese UTF-8 Subtitles (`SRT` / `ASS`)
- Always save `.srt` and `.ass` files in **UTF-8** so Vietnamese diacritics (`ă, â, đ, ê, ô, ơ, ư` + tone marks) never mojibake.
- Recommended readable `1080p` `.ass` style header (`PlayResX: 1920`, `PlayResY: 1080`, safe bottom margin `72px`, outline + soft shadow):
  ```ini
  [Script Info]
  ScriptType: v4.00+
  PlayResX: 1920
  PlayResY: 1080
  WrapStyle: 0

  [V4+ Styles]
  Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
  Style: Default,Arial,54,&H00FFFFFF,&H0000FFFF,&H00141414,&H80000000,-1,0,0,0,100,100,0,0,1,3,1,2,80,80,72,1
  ```
- Burn-in command (escaping colons on Windows paths when needed):
  ```bash
  ffmpeg -y -i raw/clip.mp4 -vf "ass=subs/vi.ass" -c:v libx264 -preset medium -crf 18 -pix_fmt yuv420p -c:a aac -b:a 192k edits/final.mp4
  ```

## 3. Transitions & Broadcast Loudness (`-14 LUFS`)
- **Smooth Crossfade (`xfade` + `acrossfade`):**
  ```bash
  ffmpeg -y -i raw/a.mp4 -i raw/b.mp4 \
    -filter_complex "[0:v][1:v]xfade=transition=fade:duration=0.5:offset=4.5[v];[0:a][1:a]acrossfade=d=0.5[a]" \
    -map "[v]" -map "[a]" -c:v libx264 -pix_fmt yuv420p -c:a aac edits/merged.mp4
  ```
- **Loudness Normalization (`-14 LUFS` YouTube/TikTok/Reels standard):**
  Add `-af "loudnorm=I=-14:TP=-1.5:LRA=11"` on export so spoken narration and music never clip.

## 4. Mandatory `ffprobe` Verification via `sen verify`
Every video/audio render task MUST be verified with `ffprobe` JSON output so Sen-VN validates non-empty streams, valid resolution (`width > 0`, `height > 0`), non-zero frame rate, audio channels, and `duration > 0`:
```bash
sen verify <task_id> -- ffprobe -v error -print_format json -show_format -show_streams edits/final.mp4
```

