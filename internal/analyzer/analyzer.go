// Package analyzer handles image preprocessing and Claude vision API calls.
package analyzer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/nfnt/resize"
	"github.com/photocrit/photocrit/internal/reviewer"
	"github.com/photocrit/photocrit/internal/scanner"
	"golang.org/x/image/tiff"
	"golang.org/x/image/webp"
)

// ErrSkip indicates a file should be skipped (e.g. unsupported format, decode failure).
var ErrSkip = errors.New("skip")

// maxLongestEdge is the longest edge in pixels images are downscaled to before sending to the API.
// maxAPIBytes is the Anthropic API's hard limit for base64-decoded image size.
const maxAPIBytes = 5 * 1024 * 1024
const maxLongestEdge = 2048

// rawExtensions lists RAW file extensions.
var rawExtensions = map[string]bool{
	".cr2": true,
	".nef": true,
	".arw": true,
	".dng": true,
	".orf": true,
	".rw2": true,
}

// LoadAndEncode reads an image file, optionally downscales it, and returns a
// base64-encoded JPEG data URL suitable for the Claude vision API.
func LoadAndEncode(path string) (mediaType string, encodedData string, err error) {
	ext := strings.ToLower(path[strings.LastIndex(path, ".")+1:])
	ext = "." + ext

	// HEIC: not supported natively in Go
	if ext == ".heic" {
		// TODO: add libheif CGo binding support for HEIC files in a future version
		fmt.Fprintf(os.Stderr, "warning: skipping %s — HEIC format not supported (requires libheif)\n", path)
		return "", "", ErrSkip
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read %s: %w", path, err)
	}

	// Always decode and downscale to maxLongestEdge before sending to the API.
	img, decErr := decodeImage(ext, data)
	if decErr != nil {
		if rawExtensions[ext] {
			fmt.Fprintf(os.Stderr, "warning: skipping %s — RAW decode failed: %v\n", path, decErr)
			return "", "", ErrSkip
		}
		return "", "", fmt.Errorf("decode %s: %w", path, decErr)
	}

	img = downscale(img, maxLongestEdge)

	// Re-encode as JPEG, reducing quality until the result fits within the API's 5MB limit.
	var buf bytes.Buffer
	for quality := 85; quality >= 50; quality -= 15 {
		buf.Reset()
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return "", "", fmt.Errorf("re-encode %s as JPEG: %w", path, err)
		}
		if buf.Len() <= maxAPIBytes {
			break
		}
	}
	if buf.Len() > maxAPIBytes {
		return "", "", fmt.Errorf("could not reduce %s below 5MB even at minimum quality", path)
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return "image/jpeg", encoded, nil
}

// decodeImage decodes image bytes for the given extension.
func decodeImage(ext string, data []byte) (image.Image, error) {
	r := bytes.NewReader(data)
	switch ext {
	case ".jpg", ".jpeg":
		return jpeg.Decode(r)
	case ".png":
		return png.Decode(r)
	case ".tiff", ".tif":
		return tiff.Decode(r)
	case ".webp":
		return webp.Decode(r)
	default:
		// Attempt generic decode for RAW and other formats
		img, _, err := image.Decode(r)
		return img, err
	}
}

// downscale resizes img so its longest edge is at most maxEdge, preserving aspect ratio.
func downscale(img image.Image, maxEdge uint) image.Image {
	b := img.Bounds()
	w := uint(b.Dx())
	h := uint(b.Dy())
	if w <= maxEdge && h <= maxEdge {
		return img
	}
	if w >= h {
		return resize.Resize(maxEdge, 0, img, resize.Lanczos3)
	}
	return resize.Resize(0, maxEdge, img, resize.Lanczos3)
}

// analysisSchema is the JSON schema we ask Claude to follow in its response.
const analysisSchema = `{
  "category": "failed|good|keeper",
  "reasoning": "narrative explanation",
  "technical": "technical assessment",
  "strengths": ["array", "of", "strengths"],
  "weaknesses": ["array", "of", "weaknesses"]
}`

// analysisSystemPrompt guides Claude in evaluating photographs.
const analysisSystemPrompt = `You are an expert photography critic specializing in wildlife, macro, street, travel, and landscape photography. Your role is to evaluate photographs and categorize them into one of three tiers:

- "failed": Technically unusable — blurry, severely over/under-exposed, badly out of focus, or so technically flawed the image cannot be saved in post-processing
- "good": Technically sound but not particularly memorable; competent execution without a standout moment, composition, or light
- "keeper": Worth saving, sharing, or posting — has a standout quality such as a decisive moment, exceptional light, strong composition, remarkable subject behaviour, or significant emotional impact

Evaluate each image across these criteria:
1. Technical quality: sharpness/focus, exposure (highlights, shadows, dynamic range), noise, motion blur
2. Composition: rule of thirds, leading lines, framing, subject isolation, background clutter
3. Light: quality, direction, golden/blue hour, harsh midday, artificial
4. Moment/Impact: decisive moment, emotion, wildlife behaviour, drama, story
5. Post-processing potential: recoverable headroom vs. already ruined

Respond ONLY with valid JSON matching this schema:
` + analysisSchema

// claudeResponse is the expected JSON structure from Claude.
type claudeResponse struct {
	Category   string   `json:"category"`
	Reasoning  string   `json:"reasoning"`
	Technical  string   `json:"technical"`
	Strengths  []string `json:"strengths"`
	Weaknesses []string `json:"weaknesses"`
}

// AnalyzeImage calls the Claude vision API for a single image and returns a PhotoDecision.
// It implements exponential backoff on 429, single retry on 5xx, max 5 attempts.
func AnalyzeImage(ctx context.Context, client *anthropic.Client, model string, filename string, mediaType string, encodedData string) (reviewer.PhotoDecision, error) {
	const maxAttempts = 5

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			// Compute backoff: 1s, 2s, 4s, 8s
			wait := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return reviewer.PhotoDecision{}, ctx.Err()
			case <-time.After(wait):
			}
		}

		msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     model,
			MaxTokens: 1024,
			System: []anthropic.TextBlockParam{
				{Text: analysisSystemPrompt},
			},
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(
					anthropic.NewImageBlockBase64(mediaType, encodedData),
					anthropic.NewTextBlock(fmt.Sprintf("Please analyze this photograph (%s) and respond with JSON only.", filename)),
				),
			},
		})
		if err != nil {
			var apiErr *anthropic.Error
			if errors.As(err, &apiErr) {
				status := apiErr.StatusCode
				if status == http.StatusTooManyRequests {
					// Rate limited — backoff and retry
					lastErr = err
					continue
				}
				if status >= 500 {
					// Server error — single retry
					if attempt == 0 {
						lastErr = err
						continue
					}
					return reviewer.PhotoDecision{}, fmt.Errorf("API server error for %s (status %d): %w", filename, status, err)
				}
			}
			return reviewer.PhotoDecision{}, fmt.Errorf("API call for %s: %w", filename, err)
		}

		// Extract text from response
		text := extractText(msg)
		if text == "" {
			return reviewer.PhotoDecision{}, fmt.Errorf("empty response from Claude for %s", filename)
		}

		// Parse JSON — strip markdown code fences if present
		text = stripCodeFences(text)

		var cr claudeResponse
		if err := json.Unmarshal([]byte(text), &cr); err != nil {
			return reviewer.PhotoDecision{}, fmt.Errorf("parse Claude response for %s: %w (raw: %s)", filename, err, truncate(text, 200))
		}

		cat := reviewer.Category(cr.Category)
		switch cat {
		case reviewer.CategoryFailed, reviewer.CategoryGood, reviewer.CategoryKeeper:
		default:
			// Default to good if unknown category
			cat = reviewer.CategoryGood
		}

		return reviewer.PhotoDecision{
			Filename:   filename,
			Category:   cat,
			Reasoning:  cr.Reasoning,
			Technical:  cr.Technical,
			Strengths:  cr.Strengths,
			Weaknesses: cr.Weaknesses,
		}, nil
	}

	return reviewer.PhotoDecision{}, fmt.Errorf("max retries exceeded for %s: %w", filename, lastErr)
}

// extractText pulls the first text block from a Claude message response.
func extractText(msg *anthropic.Message) string {
	for _, block := range msg.Content {
		if block.Type == "text" {
			return block.AsText().Text
		}
	}
	return ""
}

// stripCodeFences removes markdown ```json ... ``` wrappers if present.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.SplitN(s, "\n", 2)
		if len(lines) == 2 {
			s = lines[1]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	}
	return strings.TrimSpace(s)
}

// truncate shortens a string to at most n runes.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// AnalyzeBatch runs individual analysis on a slice of images using a worker pool.
// progress is called after each image completes (done, total).
// Errors for individual images are logged as warnings; the batch continues.
func AnalyzeBatch(
	ctx context.Context,
	client *anthropic.Client,
	model string,
	images []scanner.ImageFile,
	concurrency int,
	progress func(done, total int),
) ([]reviewer.PhotoDecision, error) {
	type result struct {
		idx      int
		decision reviewer.PhotoDecision
		err      error
	}

	total := len(images)
	results := make([]reviewer.PhotoDecision, total)

	jobs := make(chan int, total)
	for i := range images {
		jobs <- i
	}
	close(jobs)

	resultCh := make(chan result, total)
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				img := images[idx]
				mt, encoded, err := LoadAndEncode(img.Path)
				if err != nil {
					if errors.Is(err, ErrSkip) {
						resultCh <- result{idx: idx, err: ErrSkip}
						continue
					}
					fmt.Fprintf(os.Stderr, "warning: skipping %s — %v\n", img.Filename, err)
					resultCh <- result{idx: idx, err: err}
					continue
				}

				dec, err := AnalyzeImage(ctx, client, model, img.Filename, mt, encoded)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: skipping %s — %v\n", img.Filename, err)
					resultCh <- result{idx: idx, err: err}
					continue
				}
				resultCh <- result{idx: idx, decision: dec}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	done := 0
	var decisions []reviewer.PhotoDecision
	for r := range resultCh {
		done++
		if progress != nil {
			progress(done, total)
		}
		if r.err != nil {
			continue
		}
		results[r.idx] = r.decision
		decisions = append(decisions, r.decision)
	}

	// Build ordered slice from results (preserving image order, skipping zero values)
	ordered := make([]reviewer.PhotoDecision, 0, total)
	for i, d := range results {
		if d.Filename == "" {
			// Check if original was skipped
			_ = i
			continue
		}
		ordered = append(ordered, d)
	}

	return ordered, nil
}
