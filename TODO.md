# photocrit — Implementation TODO

> Load this file at the start of a session to resume implementation.
> Reference SPEC.md for full design details.
> Work top-to-bottom. Each section depends on the previous.

---

## 0. Project Bootstrap ✅

- [x] `go mod init github.com/photocrit/photocrit`
- [x] Add dependencies: `go get github.com/spf13/cobra github.com/anthropics/anthropic-sdk-go golang.org/x/image github.com/nfnt/resize`
- [x] Create directory structure: `cmd/`, `internal/scanner/`, `internal/analyzer/`, `internal/comparator/`, `internal/reviewer/`, `internal/mover/`, `internal/report/`
- [x] Write `main.go` that calls `cmd.Execute()`
- [x] Write `cmd/root.go` with cobra root command and shared flags

---

## 1. Scanner (`internal/scanner/`) ✅

- [x] Define `ImageFile` struct: `Path`, `Filename`, `Extension`, `SizeBytes`
- [x] `Scan(dir string, recursive bool, limit int) ([]ImageFile, error)` — walk directory, filter by supported extensions, skip category folders unless they are the root being scanned, enforce limit if > 0
- [x] `GroupBySequence(images []ImageFile) [][]ImageFile` — group images with sequential filenames (gap ≤ 3) into burst groups; singletons get their own group

---

## 2. Reviewer Types (`internal/reviewer/`) ✅

- [x] `Category` type: `"failed"`, `"good"`, `"keeper"`
- [x] `PhotoDecision` struct: `Filename`, `Category`, `Override *Category`, `Score *int` (keepers only, computed), `TechnicalScore *int`, `ArtisticScore *int`, `Reasoning`, `Technical`, `Strengths []string`, `Weaknesses []string`, `GroupID`, `GroupRank`
- [x] `Group` struct: `ID`, `Filenames []string`, `Summary`
- [x] `ReviewFile` struct: `Version`, `Directory`, `AnalyzedAt`, `Model`, `Photos`, `Groups`
- [x] `WriteReviewFile`, `ReadReviewFile`, `EffectiveCategory`

---

## 3. Image Preprocessing (`internal/analyzer/`) ✅

- [x] Always decode and downscale to 1024px longest edge before encoding
- [x] Re-encode as JPEG; if still >5MB iterate quality 85→70→55 until it fits
- [x] RAW files: attempt decode, skip with warning on failure
- [x] HEIC: skip with warning + TODO comment for libheif

---

## 4. Individual Analysis Pass (`internal/analyzer/`) ✅

- [x] Prompt oriented toward wildlife, macro, street, travel, landscape
- [x] JSON response schema: `category`, `technical_score`, `artistic_score`, `reasoning`, `technical`, `strengths`, `weaknesses`
- [x] Two-subscore approach: model returns `technical_score` (60% weight) and `artistic_score` (40% weight); final score computed in Go as `round(ts*0.6 + as*0.4)`
- [x] Score anchors in prompt: 90+ exceptional, 80-89 strong, 65-79 solid, 50-64 marginal; subscores=0 for non-keepers
- [x] `max_tokens` = 512; concise response requested (2-3 sentence reasoning, 3 items max per strength/weakness list)
- [x] Retry: exponential backoff on 429, single retry on 5xx, max 5 attempts
- [x] `AnalyzeBatch` worker pool with progress callback; skips failed images without aborting run

---

## 5. Comparative Analysis Pass (`internal/comparator/`) ✅

- [x] Multi-image Claude call per group; rank + confirm/revise categories
- [x] Groups run sequentially; singletons skipped
- [x] Revised decisions merged back into full slice

---

## 6. Analyze Command (`cmd/analyze.go`) ✅

- [x] `photocrit analyze <directory>` subcommand
- [x] `--batch N`: split images into chunks of N, process sequentially, merge into single review file
- [x] `--resume`: load existing review file, skip already-analyzed images, merge new decisions
- [x] Progress to stderr; summary to stdout on completion

---

## 7. Mover (`internal/mover/`) ✅

- [x] Create `_failed/`, `_good/`, `_keepers/` as needed
- [x] Move files using `EffectiveCategory()`; collision handling with `_1`, `_2` suffix
- [x] `--dry-run` support; missing-source warning

---

## 8. Report Generator (`internal/report/`) ✅

- [x] Keepers sorted by score descending
- [x] Score displayed per keeper entry (`Score: XX/100`)
- [x] Structure: header, summary table, Keepers, Good Shots, Failed Shots, Groups sections
- [x] `WriteReport` writes to `<dir>/photocrit-report.md`

---

## 9. Apply Command (`cmd/apply.go`) ✅

- [x] `photocrit apply <directory>` subcommand
- [x] Read review file → move files → generate + write report
- [x] Summary: files moved per category, report path

---

## 10. Score Command (`cmd/score.go`) ✅

- [x] `photocrit score <directory>` subcommand
- [x] Reads review file, finds keepers without scores (or all keepers with `--force`)
- [x] Scans both root directory and `_keepers/` subdirectory to locate files post-apply
- [x] Re-analyzes only target images; merges scores back into existing decisions
- [x] Merges `technical_score` and `artistic_score` subscores back alongside final score
- [x] Respects `--batch`, `--concurrency`, `--haiku`, `--dry-run`

---

## 11. Polish

- [x] `--dry-run` respected everywhere
- [x] `--haiku` flag for cheaper individual pass
- [x] `photocrit --version` flag
- [x] README with install instructions and usage
- [x] Tests: `scanner.GroupBySequence`, `reviewer.EffectiveCategory`, `mover.Move`
- [x] End-to-end tested on real photo directories (Aizuwakamatsu set, flickr_data 544 images)
- [ ] Update README to document `--batch`, `--resume`, `score` subcommand, and scoring

---

## Known Limitations

- HEIC not supported (requires libheif CGo binding)
- RAW decode support limited to what `golang.org/x/image` can handle
- Scoring is done independently per image; no cross-image score normalisation (Claude may be inconsistent across batches)
