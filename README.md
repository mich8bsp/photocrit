# photocrit

A CLI tool that analyzes a directory of photographs using Claude's vision API and categorizes them into three tiers: **failed**, **good**, and **keeper**. Designed for wildlife, macro, street, travel, and landscape photography.

## How it works

1. **`analyze`** — sends each image to Claude for individual assessment, then runs a comparative pass to rank similar/burst shots against each other. Writes a `photocrit-review.json` file with every decision and its reasoning.
2. You inspect (and optionally edit) the review file.
3. **`apply`** — moves files into `_failed/`, `_good/`, and `_keepers/` subdirectories and writes a `photocrit-report.md` with full per-photo narrative analysis.

## Categories

| Category | Folder | Description |
|---|---|---|
| `failed` | `_failed/` | Blurry, over/under-exposed, out of focus, technically unusable |
| `good` | `_good/` | Technically sound but not particularly memorable |
| `keeper` | `_keepers/` | Worth saving, sharing, or posting |

## Installation

Requires Go 1.21+.

```bash
git clone https://github.com/mich8bsp/photocrit
cd photocrit
go build -o photocrit .
```

Or install directly:

```bash
go install github.com/mich8bsp/photocrit@latest
```

## Usage

```bash
export ANTHROPIC_API_KEY=sk-ant-...

# Step 1: analyze a directory
photocrit analyze ./photos

# Step 2: inspect the review file, override any decisions if needed
# edit ./photos/photocrit-review.json  (set "override": "keeper" etc.)

# Step 3: move files and generate report
photocrit apply ./photos
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--model` | `claude-sonnet-4-6` | Claude model for analysis |
| `--opus` | false | Use `claude-opus-4-6` for the comparative pass |
| `--concurrency` | `5` | Parallel API calls during individual analysis |
| `--review-file` | `<dir>/photocrit-review.json` | Custom review file path |
| `--recursive` | false | Recurse into subdirectories |
| `--dry-run` | false | Preview actions without writing files or moving images |

## Review file

After `analyze`, a `photocrit-review.json` is written to the target directory. Each photo entry looks like:

```json
{
  "filename": "IMG_0041.jpg",
  "category": "keeper",
  "override": null,
  "reasoning": "Sharp image of a kingfisher mid-dive...",
  "technical": "Well exposed, fast shutter frozen motion...",
  "strengths": ["decisive moment", "clean background"],
  "weaknesses": ["minor fringing on feathers"],
  "group_id": "group_003",
  "group_rank": 1
}
```

Set `"override"` to `"failed"`, `"good"`, or `"keeper"` to change a decision before applying.

## Requirements

- [Anthropic API key](https://console.anthropic.com) with credits
- Go 1.21+
- Supported image formats: JPEG, PNG, TIFF, WebP — plus RAW formats (CR2, NEF, ARW, DNG, ORF, RW2) with best-effort decoding

> **Note:** HEIC files are not currently supported. RAW file support depends on Go's image decoding capabilities and may vary by format.

## Limits

- Hard cap of 200 images per run
- All images are downscaled to a maximum of 2048px on their longest edge before being sent to the API
