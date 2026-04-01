# photocrit — Implementation TODO

> Load this file at the start of a session to resume implementation.
> Reference SPEC.md for full design details.
> Work top-to-bottom. Each section depends on the previous.

---

## 0. Project Bootstrap ✅

- [x] `go mod init github.com/<user>/photocrit`
- [x] Add dependencies: `go get github.com/spf13/cobra github.com/anthropics/anthropic-sdk-go golang.org/x/image github.com/nfnt/resize`
- [x] Create directory structure: `cmd/`, `internal/scanner/`, `internal/analyzer/`, `internal/comparator/`, `internal/reviewer/`, `internal/mover/`, `internal/report/`
- [x] Write `main.go` that calls `cmd.Execute()`
- [x] Write `cmd/root.go` with cobra root command and shared flags (`--model`, `--concurrency`, `--dry-run`, `--review-file`, `--opus`, `--recursive`)

---

## 1. Scanner (`internal/scanner/`) ✅

- [x] Define `ImageFile` struct: `Path`, `Filename`, `Extension`, `SizeBytes`
- [x] `Scan(dir string, recursive bool) ([]ImageFile, error)` — walk directory, filter by supported extensions, skip `_failed/`, `_good/`, `_keepers/` folders
- [x] Enforce 200-image hard cap; return descriptive error if exceeded
- [x] `GroupBySequence(images []ImageFile) [][]ImageFile` — group images with sequential filenames (e.g., `IMG_0041`–`IMG_0047`) into candidate burst groups; singletons get their own group

---

## 2. Reviewer Types (`internal/reviewer/`) ✅

Define shared types first — other packages import these.

- [x] `Category` type (string enum): `"failed"`, `"good"`, `"keeper"`
- [x] `PhotoDecision` struct: `Filename`, `Category`, `Override *Category`, `Reasoning`, `Technical`, `Strengths []string`, `Weaknesses []string`, `GroupID string`, `GroupRank int`
- [x] `Group` struct: `ID`, `Filenames []string`, `Summary`
- [x] `ReviewFile` struct: `Version`, `Directory`, `AnalyzedAt`, `Model`, `Photos []PhotoDecision`, `Groups []Group`
- [x] `WriteReviewFile(path string, rf ReviewFile) error`
- [x] `ReadReviewFile(path string) (ReviewFile, error)`
- [x] `EffectiveCategory(pd PhotoDecision) Category` — returns `Override` if set, else `Category`

---

## 3. Image Preprocessing (`internal/analyzer/`) ✅

- [x] `LoadAndEncode(path string) (base64DataURL string, err error)`
  - Read file bytes
  - If size > 5MB: decode image, downscale longest edge to 2048px, re-encode as JPEG
  - Base64-encode and return as `data:image/jpeg;base64,...`
- [x] For RAW extensions (`.cr2`, `.nef`, `.arw`, `.dng`, `.orf`, `.rw2`): attempt decode with `golang.org/x/image`; on failure log warning and return `ErrSkip`
- [x] Handle `.heic` — note in code that Go stdlib does not support HEIC natively; skip with a warning and a TODO comment for future `libheif` CGo binding

---

## 4. Individual Analysis Pass (`internal/analyzer/`) ✅

- [x] Define `AnalysisPrompt` — system + user prompt string oriented toward wildlife, macro, street, travel, landscape genres. Request structured JSON response with fields: `category`, `reasoning`, `technical`, `strengths`, `weaknesses`
- [x] `AnalyzeImage(ctx context.Context, client *anthropic.Client, model string, imageDataURL string) (PhotoDecision, error)`
  - Call Claude messages API with vision content block
  - Parse JSON response into `PhotoDecision`
  - Implement retry logic: exponential backoff on 429, single retry on 5xx, max 5 attempts
- [x] `AnalyzeBatch(ctx context.Context, client *anthropic.Client, model string, images []ImageFile, concurrency int, progress func(done, total int)) ([]PhotoDecision, error)`
  - Worker pool with `concurrency` goroutines
  - Call progress callback after each completion
  - Collect errors; skip failed images with warning, do not abort run

---

## 5. Comparative Analysis Pass (`internal/comparator/`) ✅

- [x] `CompareGroup(ctx context.Context, client *anthropic.Client, model string, group []ImageFile, decisions []PhotoDecision) ([]PhotoDecision, error)`
  - Only process groups with 2+ images
  - Send all images in the group in a single multi-image Claude call
  - Prompt asks Claude to rank the group, identify the best, and confirm/revise individual categories
  - Return updated `PhotoDecision` slice for the group with `GroupRank` set and categories potentially revised
- [x] `RunComparativePass(ctx context.Context, client *anthropic.Client, model string, groups [][]ImageFile, decisions []PhotoDecision) ([]PhotoDecision, error)`
  - Iterate over groups, call `CompareGroup` for multi-image groups
  - Merge revised decisions back into the full decisions slice
  - Run groups sequentially (not concurrently) to avoid overloading context

---

## 6. Analyze Command (`cmd/analyze.go`) ✅

- [x] `photocrit analyze <directory>` subcommand
- [x] Print progress to stderr: `Scanning...`, `Analyzing 1/67...`, `Running comparative pass...`, `Writing review file...`
- [x] Steps wired: `scanner.Scan()` → `analyzer.AnalyzeBatch()` → `comparator.RunComparativePass()` → assign group IDs/ranks → `reviewer.WriteReviewFile()`
- [x] Print summary on completion: counts per category, path to review file, next step instructions

---

## 7. Mover (`internal/mover/`) ✅

- [x] `Move(sourceDir string, decisions []reviewer.PhotoDecision, dryRun bool) error`
  - Create `_failed/`, `_good/`, `_keepers/` as needed
  - For each decision, use `EffectiveCategory()` to determine destination
  - Move file; on filename collision append `_1`, `_2`, etc.
  - If `dryRun`: print intended moves, do not execute
  - Log warning and continue if source file missing

---

## 8. Report Generator (`internal/report/`) ✅

- [x] `Generate(rf reviewer.ReviewFile) string` — returns markdown string
- [x] Structure: header, summary table, Keepers section, Good Shots section, Failed Shots section, Groups section
- [x] Each photo entry: filename as heading, category, technical note, reasoning paragraph, strengths/weaknesses inline
- [x] Groups section: list each multi-image group with filenames and summary
- [x] `WriteReport(dir string, content string) error` — writes to `<dir>/photocrit-report.md`

---

## 9. Apply Command (`cmd/apply.go`) ✅

- [x] `photocrit apply <directory>` subcommand
- [x] Steps: `reviewer.ReadReviewFile()` → `mover.Move()` → `report.Generate()` + `report.WriteReport()`
- [x] Print summary: files moved per category, report path

---

## 10. Polish ✅

- [x] `--dry-run` respected in both `analyze` and `apply`
- [x] Consistent error messages
- [x] `photocrit --version` flag
- [ ] README with install instructions, example usage, and description of the review file schema
- [x] Test: `scanner.GroupBySequence` with sequential and non-sequential filenames
- [x] Test: `reviewer.EffectiveCategory` with and without override
- [x] Test: `mover.Move` with dry-run flag
- [ ] Manual end-to-end test with a small sample directory ← **IN PROGRESS**

---

## Notes for the implementing session

- The Anthropic Go SDK is at `github.com/anthropics/anthropic-sdk-go` — check the SDK README for vision/image message format before writing the API calls
- Claude vision expects images as content blocks of type `image` with `source.type = "base64"`, `source.media_type`, and `source.data`
- Ask Claude to respond in JSON by including `"Respond only with valid JSON matching this schema: {...}"` in the prompt — do not use tool use for structured output unless the SDK makes it easy
- RAW file support via pure Go is limited; it is acceptable to skip RAW files with a clear warning in v1 and note it as a known limitation
- Keep the comparative pass prompt focused: provide the images, their individual assessments, and ask only for ranking + category confirmation — avoid open-ended prompts that produce verbose unstructured output
