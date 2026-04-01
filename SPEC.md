# photocrit — Specification

## Overview

`photocrit` is a CLI tool that analyzes a directory of photographs using Claude's vision API and categorizes them into three tiers: failed shots, good shots, and keepers. It produces a human-reviewable decision file before touching any files, and generates a markdown report explaining each decision.

---

## CLI Interface

```
photocrit analyze <directory>   # analyze images, write review file, do not move files
photocrit apply <directory>     # read review file, move files, write final report
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--model` | `claude-sonnet-4-6` | Claude model for individual analysis pass |
| `--opus` | false | Use `claude-opus-4-6` for the comparative pass (higher quality judgement) |
| `--concurrency` | `5` | Max parallel Claude API calls |
| `--review-file` | `<dir>/photocrit-review.json` | Path to review file |
| `--dry-run` | false | Print what would happen without writing files |

### Typical workflow

```bash
photocrit analyze ./photos          # run analysis, produces photocrit-review.json
# user inspects and optionally edits photocrit-review.json
photocrit apply ./photos            # moves files, writes photocrit-report.md
```

---

## Categories

| Category | Folder | Description |
|---|---|---|
| `failed` | `_failed/` | Blurry, over/under exposed, out of focus, technically unusable |
| `good` | `_good/` | Technically sound but not particularly memorable or impressive |
| `keeper` | `_keepers/` | Worth saving, sharing, or posting — standout composition, light, or moment |

Folders are created inside the target directory, prefixed with `_` so they sort to the top and are visually distinct from the original image files.

---

## Analysis Pipeline

### Phase 1 — Scan

- Recursively find all image files in the target directory (non-recursive by default, `--recursive` flag to opt in)
- Supported extensions: `.jpg`, `.jpeg`, `.png`, `.heic`, `.tiff`, `.tif`, `.webp`, `.cr2`, `.nef`, `.arw`, `.dng`, `.orf`, `.rw2`
- Skip files inside `_failed/`, `_good/`, `_keepers/` to avoid re-processing
- Enforce a hard cap of 200 images per run; fail with a clear error if exceeded
- Images larger than 5MB are downscaled to fit before encoding (preserve aspect ratio, target longest edge ≤ 2048px) — RAW files in particular may need this

### Phase 2 — Individual Analysis

For each image, make a Claude vision API call with:
- The image as a base64-encoded data URL
- A structured prompt requesting: category, technical assessment, narrative reasoning, notable strengths and weaknesses

The prompt should orient Claude toward the genres the tool targets: **wildlife, macro, street, travel, and landscape** photography. Evaluation criteria include:

- **Technical**: sharpness/focus, exposure (highlights, shadows, dynamic range), noise, motion blur
- **Composition**: rule of thirds, leading lines, framing, subject isolation, background clutter
- **Light**: quality, direction, golden/blue hour, harsh midday, artificial
- **Moment/Impact**: decisive moment, emotion, behaviour (wildlife), drama, story
- **Post-processing potential**: headroom in the image vs. already ruined

Process images concurrently up to `--concurrency` limit. Implement exponential backoff for rate limit errors (429) with up to 5 retries.

### Phase 3 — Comparative Analysis

After individual analysis, group images that are likely from the same scene or burst sequence. Use Claude to compare images within each group and identify which, if any, should be elevated or downgraded relative to the individual analysis.

Grouping heuristic (applied before sending to Claude):
- Images with sequential filenames (e.g., `IMG_0041` through `IMG_0047`) taken within the same session are candidate groups
- Claude is then shown the group and asked to rank them and confirm/revise categories

This pass uses `--opus` model if the flag is set, otherwise falls back to the same model as Phase 2.

### Phase 4 — Review File

Write `photocrit-review.json` (or path from `--review-file`) to the target directory. The user can inspect and edit this file before running `apply`.

```json
{
  "version": "1",
  "directory": "/absolute/path/to/photos",
  "analyzed_at": "2026-04-01T14:00:00Z",
  "model": "claude-sonnet-4-6",
  "photos": [
    {
      "filename": "IMG_0041.jpg",
      "category": "keeper",
      "override": null,
      "reasoning": "Sharp image of a kingfisher mid-dive with excellent subject isolation...",
      "technical": "Well exposed, fast shutter frozen motion, slight chromatic aberration on wing edges",
      "strengths": ["decisive moment", "clean background", "excellent sharpness"],
      "weaknesses": ["minor fringing on feathers"],
      "group_id": "group_003",
      "group_rank": 1
    }
  ],
  "groups": [
    {
      "id": "group_003",
      "filenames": ["IMG_0041.jpg", "IMG_0042.jpg", "IMG_0043.jpg"],
      "summary": "Three shots of the same kingfisher dive. IMG_0041 captured the peak moment."
    }
  ]
}
```

The `override` field is `null` by default. The user can set it to `"failed"`, `"good"`, or `"keeper"` to override the model's decision. The `apply` command uses `override` if set, otherwise uses `category`.

### Phase 5 — Apply

On `photocrit apply <directory>`:
1. Read `photocrit-review.json`
2. Create `_failed/`, `_good/`, `_keepers/` subdirectories as needed
3. Move each file to the appropriate subdirectory (using effective category: `override ?? category`)
4. If a file is missing, log a warning and continue
5. Write `photocrit-report.md`

---

## Report Format

Written to `<directory>/photocrit-report.md` after `apply`.

```markdown
# photocrit Report
**Directory:** /path/to/photos
**Analyzed:** 2026-04-01 14:00
**Model:** claude-sonnet-4-6

## Summary

| Category | Count |
|---|---|
| Keepers | 12 |
| Good | 47 |
| Failed | 8 |
| **Total** | **67** |

---

## Keepers (12)

### IMG_0041.jpg
**Category:** keeper
**Technical:** Well exposed, fast shutter frozen motion, slight chromatic aberration on wing edges

Sharp image of a kingfisher mid-dive with excellent subject isolation...

**Strengths:** decisive moment · clean background · excellent sharpness
**Weaknesses:** minor fringing on feathers

---

## Good Shots (47)

...

## Failed Shots (8)

...

## Groups

### Group 3 — 3 images
IMG_0041 · IMG_0042 · IMG_0043

Three shots of the same kingfisher dive. IMG_0041 captured the peak moment.
```

---

## Error Handling

| Scenario | Behaviour |
|---|---|
| Unreadable / corrupt image | Skip, log warning, continue |
| API rate limit (429) | Exponential backoff, up to 5 retries |
| API hard error (5xx) | Retry once, then fail with clear message |
| >200 images in directory | Fail immediately with count and instructions to split |
| Review file missing on `apply` | Fail with clear message pointing to `analyze` |
| File already in a category folder | Skip on `apply` |
| Target file already exists in destination | Rename with `_1`, `_2` suffix |

---

## Project Structure

```
photocrit/
├── main.go
├── cmd/
│   ├── root.go          # cobra root, shared flags
│   ├── analyze.go       # analyze subcommand
│   └── apply.go         # apply subcommand
├── internal/
│   ├── scanner/
│   │   └── scanner.go   # directory walk, image discovery, grouping heuristic
│   ├── analyzer/
│   │   └── analyzer.go  # individual analysis pass, Claude API calls, concurrency
│   ├── comparator/
│   │   └── comparator.go # comparative pass, group ranking
│   ├── reviewer/
│   │   └── reviewer.go  # review file read/write, schema types
│   ├── mover/
│   │   └── mover.go     # file move operations
│   └── report/
│       └── report.go    # markdown report generation
├── go.mod
└── go.sum
```

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/anthropics/anthropic-sdk-go` | Official Anthropic Go SDK |
| `golang.org/x/image` | Image decoding for resize pre-processing |
| `github.com/nfnt/resize` | Image downscaling before encoding |

Standard library handles JSON, file I/O, base64 encoding, concurrency primitives.

---

## Constraints & Assumptions

- Images are not copied — they are **moved**. The source directory is modified on `apply`.
- The tool does not recurse into subdirectories unless `--recursive` is passed.
- RAW files (cr2, nef, arw, dng, orf, rw2) are included in the scan but Claude can only process renderable formats. RAW files should be decoded to a preview JPEG in memory before sending to the API. If decoding fails, the file is skipped with a warning.
- The tool is stateless between runs. Running `analyze` again overwrites the review file.
- No database, no persistent state beyond the review file and report.
