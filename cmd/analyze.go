package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/spf13/cobra"

	"github.com/photocrit/photocrit/internal/analyzer"
	"github.com/photocrit/photocrit/internal/comparator"
	"github.com/photocrit/photocrit/internal/reviewer"
	"github.com/photocrit/photocrit/internal/scanner"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze <directory>",
	Short: "Analyze images in a directory and write a review file",
	Args:  cobra.ExactArgs(1),
	RunE:  runAnalyze,
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	dir, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("resolve directory: %w", err)
	}

	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("%s is not a valid directory", dir)
	}

	// Resolve review file path
	reviewFilePath := flagReviewFile
	if reviewFilePath == "" {
		reviewFilePath = filepath.Join(dir, "photocrit-review.json")
	}

	// Determine models
	model := flagModel
	if flagHaiku {
		model = "claude-haiku-4-5-20251001"
	}
	compareModel := model
	if flagOpus {
		compareModel = "claude-opus-4-6"
	}

	// Phase 1 — Scan
	fmt.Fprintf(os.Stderr, "Scanning %s...\n", dir)
	limit := scanner.MaxImages
	if flagBatch > 0 {
		limit = 0 // no cap when batching
	}
	images, err := scanner.Scan(dir, flagRecursive, limit)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Found %d images.\n", len(images))

	if len(images) == 0 {
		fmt.Fprintln(os.Stderr, "No images found. Nothing to do.")
		return nil
	}

	// Resume: load existing decisions and skip already-analyzed images
	var existingDecisions []reviewer.PhotoDecision
	var existingGroups []reviewer.Group
	if flagResume {
		if existing, err := reviewer.ReadReviewFile(reviewFilePath); err == nil {
			existingDecisions = existing.Photos
			existingGroups = existing.Groups
			analyzed := make(map[string]bool, len(existingDecisions))
			for _, d := range existingDecisions {
				analyzed[d.Filename] = true
			}
			remaining := images[:0]
			for _, img := range images {
				if !analyzed[img.Filename] {
					remaining = append(remaining, img)
				}
			}
			fmt.Fprintf(os.Stderr, "Resuming: %d already analyzed, %d remaining.\n",
				len(existingDecisions), len(remaining))
			images = remaining
		} else {
			fmt.Fprintf(os.Stderr, "No existing review file found, starting fresh.\n")
		}
	}

	if len(images) == 0 {
		fmt.Fprintln(os.Stderr, "All images already analyzed. Nothing to do.")
		return nil
	}

	// Split into batches
	batches := chunkImages(images, flagBatch)

	c := anthropic.NewClient()
	client := &c
	ctx := context.Background()

	var allDecisions []reviewer.PhotoDecision
	var allGroups []reviewer.Group
	groupOffset := 0
	totalAnalyzed := 0

	for bIdx, batch := range batches {
		if len(batches) > 1 {
			fmt.Fprintf(os.Stderr, "\n--- Batch %d/%d (%d images) ---\n", bIdx+1, len(batches), len(batch))
		}

		// Individual analysis
		fmt.Fprintf(os.Stderr, "Analyzing %d images (concurrency=%d, model=%s)...\n",
			len(batch), flagConcurrency, model)

		decisions, err := analyzer.AnalyzeBatch(ctx, client, model, batch, flagConcurrency,
			func(done, total int) {
				fmt.Fprintf(os.Stderr, "\rAnalyzing %d/%d...", done, total)
			})
		if err != nil {
			return fmt.Errorf("analysis failed (batch %d): %w", bIdx+1, err)
		}
		fmt.Fprintf(os.Stderr, "\rAnalyzed %d/%d images.    \n", len(decisions), len(batch))
		totalAnalyzed += len(decisions)

		// Comparative pass
		groups := scanner.GroupBySequence(batch)
		fmt.Fprintf(os.Stderr, "Running comparative pass (model=%s)...\n", compareModel)
		updatedDecisions, summaries, err := comparator.RunComparativePass(ctx, client, compareModel, groups, decisions)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: comparative pass error: %v\n", err)
			updatedDecisions = decisions
		}

		// Build decision map for this batch
		decisionMap := make(map[string]reviewer.PhotoDecision, len(updatedDecisions))
		for _, d := range updatedDecisions {
			decisionMap[d.Filename] = d
		}

		// Assign group IDs (offset to avoid collisions across batches)
		for gIdx, group := range groups {
			if len(group) < 2 {
				continue
			}
			groupID := fmt.Sprintf("group_%03d", groupOffset+gIdx+1)
			filenames := make([]string, len(group))
			for i, img := range group {
				filenames[i] = img.Filename
			}
			allGroups = append(allGroups, reviewer.Group{
				ID:        groupID,
				Filenames: filenames,
				Summary:   summaries[groupID],
			})
		}
		groupOffset += len(groups)

		// Preserve image order for this batch
		for _, img := range batch {
			if d, ok := decisionMap[img.Filename]; ok {
				allDecisions = append(allDecisions, d)
			}
		}
	}

	// Merge with existing decisions from a previous partial run
	allDecisions = append(existingDecisions, allDecisions...)
	allGroups = append(existingGroups, allGroups...)

	// Count by category
	counts := make(map[reviewer.Category]int)
	for _, d := range allDecisions {
		counts[d.Category]++
	}

	rf := reviewer.ReviewFile{
		Version:    "1",
		Directory:  dir,
		AnalyzedAt: time.Now().UTC(),
		Model:      model,
		Photos:     allDecisions,
		Groups:     allGroups,
	}

	if flagDryRun {
		fmt.Fprintf(os.Stderr, "\n[dry-run] Would write review file to %s\n", reviewFilePath)
	} else {
		fmt.Fprintf(os.Stderr, "Writing review file to %s...\n", reviewFilePath)
		if err := reviewer.WriteReviewFile(reviewFilePath, rf); err != nil {
			return err
		}
	}

	fmt.Printf("\nphotocrit analysis complete\n")
	fmt.Printf("  Keepers: %d\n", counts[reviewer.CategoryKeeper])
	fmt.Printf("  Good:    %d\n", counts[reviewer.CategoryGood])
	fmt.Printf("  Failed:  %d\n", counts[reviewer.CategoryFailed])
	fmt.Printf("  Total:   %d\n\n", totalAnalyzed)
	if !flagDryRun {
		fmt.Printf("Review file: %s\n", reviewFilePath)
		fmt.Printf("Inspect and optionally edit the review file, then run:\n")
		fmt.Printf("  photocrit apply %s\n", dir)
	}

	return nil
}

// chunkImages splits images into chunks of size n. If n <= 0, returns a single chunk.
func chunkImages(images []scanner.ImageFile, n int) [][]scanner.ImageFile {
	if n <= 0 {
		return [][]scanner.ImageFile{images}
	}
	var chunks [][]scanner.ImageFile
	for i := 0; i < len(images); i += n {
		end := i + n
		if end > len(images) {
			end = len(images)
		}
		chunks = append(chunks, images[i:end])
	}
	return chunks
}
