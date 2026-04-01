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

	// Determine model
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
	images, err := scanner.Scan(dir, flagRecursive)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Found %d images.\n", len(images))

	if len(images) == 0 {
		fmt.Fprintln(os.Stderr, "No images found. Nothing to do.")
		return nil
	}

	// Phase 2 — Individual Analysis
	c := anthropic.NewClient()
	client := &c
	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "Analyzing %d images (concurrency=%d, model=%s)...\n",
		len(images), flagConcurrency, model)

	decisions, err := analyzer.AnalyzeBatch(ctx, client, model, images, flagConcurrency,
		func(done, total int) {
			fmt.Fprintf(os.Stderr, "\rAnalyzing %d/%d...", done, total)
		})
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}
	fmt.Fprintf(os.Stderr, "\rAnalyzed %d/%d images.    \n", len(decisions), len(images))

	// Phase 3 — Group and Comparative Analysis
	fmt.Fprintf(os.Stderr, "Grouping images...\n")
	groups := scanner.GroupBySequence(images)
	fmt.Fprintf(os.Stderr, "Found %d groups.\n", len(groups))

	fmt.Fprintf(os.Stderr, "Running comparative pass (model=%s)...\n", compareModel)
	updatedDecisions, summaries, err := comparator.RunComparativePass(ctx, client, compareModel, groups, decisions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: comparative pass error: %v\n", err)
		updatedDecisions = decisions
	}

	// Phase 4 — Assign group IDs and build reviewer groups
	decisionMap := make(map[string]reviewer.PhotoDecision, len(updatedDecisions))
	for _, d := range updatedDecisions {
		decisionMap[d.Filename] = d
	}

	var reviewerGroups []reviewer.Group
	for gIdx, group := range groups {
		if len(group) < 2 {
			continue
		}
		groupID := fmt.Sprintf("group_%03d", gIdx+1)
		filenames := make([]string, len(group))
		for i, img := range group {
			filenames[i] = img.Filename
		}
		rg := reviewer.Group{
			ID:        groupID,
			Filenames: filenames,
			Summary:   summaries[groupID],
		}
		reviewerGroups = append(reviewerGroups, rg)
	}

	// Rebuild decisions in original image order
	finalDecisions := make([]reviewer.PhotoDecision, 0, len(images))
	for _, img := range images {
		if d, ok := decisionMap[img.Filename]; ok {
			finalDecisions = append(finalDecisions, d)
		}
	}

	// Count by category for summary
	counts := make(map[reviewer.Category]int)
	for _, d := range finalDecisions {
		counts[d.Category]++
	}

	rf := reviewer.ReviewFile{
		Version:    "1",
		Directory:  dir,
		AnalyzedAt: time.Now().UTC(),
		Model:      model,
		Photos:     finalDecisions,
		Groups:     reviewerGroups,
	}

	if flagDryRun {
		fmt.Fprintf(os.Stderr, "\n[dry-run] Would write review file to %s\n", reviewFilePath)
	} else {
		fmt.Fprintf(os.Stderr, "Writing review file to %s...\n", reviewFilePath)
		if err := reviewer.WriteReviewFile(reviewFilePath, rf); err != nil {
			return err
		}
	}

	// Print summary
	fmt.Printf("\nphotocrit analysis complete\n")
	fmt.Printf("  Keepers: %d\n", counts[reviewer.CategoryKeeper])
	fmt.Printf("  Good:    %d\n", counts[reviewer.CategoryGood])
	fmt.Printf("  Failed:  %d\n", counts[reviewer.CategoryFailed])
	fmt.Printf("  Total:   %d\n\n", len(finalDecisions))
	if !flagDryRun {
		fmt.Printf("Review file: %s\n", reviewFilePath)
		fmt.Printf("Inspect and optionally edit the review file, then run:\n")
		fmt.Printf("  photocrit apply %s\n", dir)
	}

	return nil
}
