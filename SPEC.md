# photocrit — Specification

## Overview

`photocrit` is a CLI tool that analyzes a directory of photographs using Claude's vision API and categorizes them into three tiers: failed shots, good shots, and keepers. It produces a human-reviewable decision file before touching any files, and generates a markdown report explaining each decision. Keeper photos receive a score (0–100) for ranking.

---

## CLI Interface

```
photocrit analyze <directory>   # analyze images, write review file, do not move files
photocrit apply <directory>     # read review file, move files, write final report
photocrit score <directory>     # add/update scores for keeper photos
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--model` | `claude-sonnet-4-6` | Claude model for individual analysis pass |
| `--haiku` | false | Use `claude-haiku-4-5` for individual pass (~15x cheaper) |
| `--opus` | false | Use `claude-opus-4-6` for the comparative pass |
| `--concurrency` | `5` | Max parallel Claude API calls |
| `--batch` | `0` | Process images in batches of N (0 = no batching, enforces 200-image limit) |
| `--resume` | false | Skip images already present in an existing review file |
| `--review-file` | `<dir>/photocrit-review.json` | Path to review file |
| `--recursive` | false | Recurse into subdirectories |
| `--dry-run` | false | Print what would happen without writing files or moving images |

#### `score`-only flags

| Flag | Default | Description |
|---|---|---|
| `--force` | false | Re-score all keepers, including those already scored |

### Typical workflow

```bash
# Standard run
photocrit analyze ./photos          # produces photocrit-review.json
# inspect/edit photocrit-review.json if needed
photocrit apply ./photos            # moves files, writes photocrit-report.md

# Large directories (>200 images)
photocrit analyze ./photos --batch 100

# Resume an interrupted run
photocrit analyze ./photos --batch 100 --resume

# Backfill scores after apply
photocrit score ./photos
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

- Walk the target directory (non-recursive by default, `--recursive` to opt in)
- Supported extensions: `.jpg`, `.jpeg`, `.png`, `.heic`, `.tiff`, `.tif`, `.webp`, `.cr2`, `.nef`, `.arw`, `.dng`, `.orf`, `.rw2`
- Skip files inside `_failed/`, `_good/`, `_keepers/` unless one of those is the root being scanned
- Enforce a hard cap of 200 images when `--batch` is not set; fail with a clear error if exceeded
- When `--batch N` is set, skip the cap and split images into chunks of N processed sequentially

### Phase 2 — Individual Analysis

For each image:
- Decode and downscale to a maximum of **1024px** on the longest edge before encoding (preserves aspect ratio)
- Re-encode as JPEG; if still >5MB, reduce quality iteratively (85 → 70 → 55) until it fits
- Send to Claude as a base64-encoded image with a structured prompt

The prompt orients Claude toward **wildlife, macro, street, travel, and landscape** photography. Evaluation criteria:
- **Technical**: sharpness/focus, exposure, noise, motion blur
- **Composition**: rule of thirds, leading lines, framing, subject isolation
- **Light**: quality, direction, golden/blue hour, harsh vs. soft
- **Moment/Impact**: decisive moment, emotion, wildlife behaviour, drama
- **Post-processing potential**: recoverable headroom vs. already ruined

Response format: JSON with `category`, `score`, `reasoning`, `technical`, `strengths`, `weaknesses`. `max_tokens` is set to 512.

Process images concurrently up to `--concurrency`. Exponential backoff on 429, single retry on 5xx, max 5 attempts.

### Phase 3 — Comparative Analysis

Group images by sequential filename (gap ≤ 3). For each multi-image group, send all images in a single Claude call and ask it to rank them and confirm/revise individual categories. Groups are processed sequentially. Uses `--opus` model if set.

### Phase 4 — Review File

Write `photocrit-review.json` to the target directory. User can inspect and edit before running `apply`.

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
      "score": 87,
      "reasoning": "Sharp image of a kingfisher mid-dive...",
      "technical": "Well exposed, fast shutter, slight chromatic aberration on wing edges",
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

The `override` field is `null` by default. Set it to `"failed"`, `"good"`, or `"keeper"` to override the model's decision. The `apply` command uses `override` if set, otherwise `category`.

The `score` field is set for keepers only (0–100). Non-keepers have no score field.

### Phase 5 — Apply

On `photocrit apply <directory>`:
1. Read `photocrit-review.json`
2. Create `_failed/`, `_good/`, `_keepers/` subdirectories as needed
3. Move each file using effective category (`override ?? category`)
4. Log warning and continue if source file missing
5. Write `photocrit-report.md`

---

## Scoring

Keepers receive a score from 0–100 computed from four independent subscores:

| Subscore | Weight | What it measures |
|---|---|---|
| `technical_score` | 30% | Unintentional flaws only — missed focus, unintended blur, bad exposure, noise. Intentional bokeh, creative motion blur, and dramatic contrast are not penalised. |
| `composition_score` | 30% | Framing, leading lines, negative space, color harmony, color interest |
| `subject_score` | 25% | Wow factor — rarity and impact of the subject or moment: iconic locations, wildlife in action, candid human moments, unusual light/weather |
| `bokeh_score` | 15% | Quality of subject separation and background rendering. Set to 50 (neutral) when no subject separation is present. |

`final_score = round(technical×0.30 + composition×0.30 + subject×0.25 + bokeh×0.15)`

| Range | Meaning |
|---|---|
| 90–100 | Exceptional — publishable, portfolio-worthy |
| 80–89 | Strong keeper — compelling on all dimensions |
| 65–79 | Solid keeper — clear merit but notable weaknesses |
| 50–64 | Marginal keeper — some redeeming qualities, significant issues |

The report sorts keepers by score descending.

The `score` subcommand can be run after `apply` to backfill scores for keepers that were analyzed before scoring was available. It scans both the root directory and `_keepers/` to find files. Use `--force` to re-score already-scored keepers.

---

## Report Format

Written to `<directory>/photocrit-report.md` after `apply`. Keepers are sorted by score descending.

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
**Score:** 87/100
**Technical:** Well exposed, fast shutter frozen motion...

Sharp image of a kingfisher mid-dive...

**Strengths:** decisive moment · clean background · excellent sharpness
**Weaknesses:** minor fringing on feathers
```

---

## Error Handling

| Scenario | Behaviour |
|---|---|
| Unreadable / corrupt image | Skip, log warning, continue |
| API rate limit (429) | Exponential backoff, up to 5 retries |
| API hard error (5xx) | Retry once, then fail with clear message |
| >200 images, no `--batch` | Fail immediately with instructions to use `--batch` |
| Review file missing on `apply` | Fail with clear message pointing to `analyze` |
| Target file already exists in destination | Rename with `_1`, `_2` suffix |
| HEIC files | Skip with warning (no native Go HEIC support) |
| RAW decode failure | Skip with warning |

---

## Project Structure

```
photocrit/
├── main.go
├── cmd/
│   ├── root.go          # cobra root, shared flags
│   ├── analyze.go       # analyze subcommand
│   ├── apply.go         # apply subcommand
│   └── score.go         # score subcommand
├── internal/
│   ├── scanner/
│   │   └── scanner.go   # directory walk, image discovery, grouping heuristic
│   ├── analyzer/
│   │   └── analyzer.go  # preprocessing, individual analysis, Claude API calls
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

- Images are **moved**, not copied. The source directory is modified on `apply`.
- The tool does not recurse into subdirectories unless `--recursive` is passed.
- All images are downscaled to 1024px longest edge before API submission regardless of original size.
- RAW files are decoded to JPEG in memory; if decoding fails, the file is skipped with a warning.
- HEIC is not supported natively in Go (requires libheif CGo binding — future work).
- The tool is stateless between runs. Running `analyze` again overwrites the review file unless `--resume` is used.
- No database, no persistent state beyond the review file and report.
