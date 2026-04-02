package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/spf13/cobra"

	"github.com/photocrit/photocrit/internal/analyzer"
	"github.com/photocrit/photocrit/internal/reviewer"
	"github.com/photocrit/photocrit/internal/scanner"
)

var scoreCmd = &cobra.Command{
	Use:   "score <directory>",
	Short: "Add scores to keeper photos that are missing them",
	Long: `Reads the existing review file, finds keeper photos without a score,
re-analyzes only those images to assign a score, and writes the updated review file.`,
	Args: cobra.ExactArgs(1),
	RunE: runScore,
}

func init() {
	rootCmd.AddCommand(scoreCmd)
}

func runScore(cmd *cobra.Command, args []string) error {
	dir, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("resolve directory: %w", err)
	}

	reviewFilePath := flagReviewFile
	if reviewFilePath == "" {
		reviewFilePath = filepath.Join(dir, "photocrit-review.json")
	}

	// Load existing review file
	rf, err := reviewer.ReadReviewFile(reviewFilePath)
	if err != nil {
		return err
	}

	// Find keepers without a score
	var toScore []string
	for _, p := range rf.Photos {
		if reviewer.EffectiveCategory(p) == reviewer.CategoryKeeper && p.Score == nil {
			toScore = append(toScore, p.Filename)
		}
	}

	if len(toScore) == 0 {
		fmt.Println("All keepers already have scores. Nothing to do.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Found %d keepers without scores.\n", len(toScore))

	// Build a set for quick lookup and match against scanned images
	needsScore := make(map[string]bool, len(toScore))
	for _, f := range toScore {
		needsScore[f] = true
	}

	// Scan directory to get full paths for target files
	allImages, err := scanner.Scan(dir, flagRecursive, 0)
	if err != nil {
		return err
	}
	var images []scanner.ImageFile
	for _, img := range allImages {
		if needsScore[img.Filename] {
			images = append(images, img)
		}
	}

	if len(images) == 0 {
		return fmt.Errorf("could not find image files for unscored keepers in %s", dir)
	}

	// Determine model
	model := flagModel
	if flagHaiku {
		model = "claude-haiku-4-5-20251001"
	}

	c := anthropic.NewClient()
	client := &c
	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "Scoring %d images (concurrency=%d, model=%s)...\n",
		len(images), flagConcurrency, model)

	// Process in batches if --batch is set
	batches := chunkImages(images, flagBatch)
	var newDecisions []reviewer.PhotoDecision

	for bIdx, batch := range batches {
		if len(batches) > 1 {
			fmt.Fprintf(os.Stderr, "\n--- Batch %d/%d (%d images) ---\n", bIdx+1, len(batches), len(batch))
		}
		decisions, err := analyzer.AnalyzeBatch(ctx, client, model, batch, flagConcurrency,
			func(done, total int) {
				fmt.Fprintf(os.Stderr, "\rScoring %d/%d...", done, total)
			})
		if err != nil {
			return fmt.Errorf("scoring failed (batch %d): %w", bIdx+1, err)
		}
		fmt.Fprintf(os.Stderr, "\rScored %d/%d images.    \n", len(decisions), len(batch))
		newDecisions = append(newDecisions, decisions...)
	}

	// Build map of new decisions by filename
	scored := make(map[string]reviewer.PhotoDecision, len(newDecisions))
	for _, d := range newDecisions {
		scored[d.Filename] = d
	}

	// Merge scores back into existing decisions, preserving all other fields
	updated := 0
	for i, p := range rf.Photos {
		if nd, ok := scored[p.Filename]; ok && nd.Score != nil {
			rf.Photos[i].Score = nd.Score
			updated++
		}
	}

	fmt.Fprintf(os.Stderr, "Updated scores for %d/%d keepers.\n", updated, len(toScore))

	if flagDryRun {
		fmt.Fprintf(os.Stderr, "[dry-run] Would write updated review file to %s\n", reviewFilePath)
		return nil
	}

	if err := reviewer.WriteReviewFile(reviewFilePath, rf); err != nil {
		return err
	}
	fmt.Printf("Review file updated: %s\n", reviewFilePath)
	fmt.Printf("Run `photocrit apply %s` to move files and generate the report.\n", dir)
	return nil
}
