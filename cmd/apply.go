package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/photocrit/photocrit/internal/mover"
	"github.com/photocrit/photocrit/internal/report"
	"github.com/photocrit/photocrit/internal/reviewer"
)

var applyCmd = &cobra.Command{
	Use:   "apply <directory>",
	Short: "Apply review file: move files and write report",
	Args:  cobra.ExactArgs(1),
	RunE:  runApply,
}

func init() {
	rootCmd.AddCommand(applyCmd)
}

func runApply(cmd *cobra.Command, args []string) error {
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

	// Step 1 — Read review file
	fmt.Fprintf(os.Stderr, "Reading review file %s...\n", reviewFilePath)
	rf, err := reviewer.ReadReviewFile(reviewFilePath)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Found %d photo decisions.\n", len(rf.Photos))

	// Step 2 — Move files
	fmt.Fprintf(os.Stderr, "Moving files...\n")
	result, err := mover.Move(dir, rf.Photos, flagDryRun)
	if err != nil {
		return fmt.Errorf("move files: %w", err)
	}

	// Step 3 — Write report
	reportContent := report.Generate(rf)
	reportPath := filepath.Join(dir, "photocrit-report.md")

	if flagDryRun {
		fmt.Fprintf(os.Stderr, "\n[dry-run] Would write report to %s\n", reportPath)
	} else {
		fmt.Fprintf(os.Stderr, "Writing report to %s...\n", reportPath)
		if err := report.WriteReport(dir, reportContent); err != nil {
			return err
		}
	}

	// Print summary
	fmt.Printf("\nphotocrit apply complete\n")
	if flagDryRun {
		fmt.Printf("  [dry-run] No files were moved.\n")
	} else {
		fmt.Printf("  Files moved:   %d\n", result.Moved)
		fmt.Printf("  Files skipped: %d\n", result.Skipped)
		fmt.Printf("    → _keepers/: %d\n", result.ByCategory[reviewer.CategoryKeeper])
		fmt.Printf("    → _good/:    %d\n", result.ByCategory[reviewer.CategoryGood])
		fmt.Printf("    → _failed/:  %d\n", result.ByCategory[reviewer.CategoryFailed])
		fmt.Printf("  Report:        %s\n", reportPath)
	}

	return nil
}
