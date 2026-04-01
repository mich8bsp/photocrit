package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const version = "0.1.0"

var (
	flagModel       string
	flagOpus        bool
	flagHaiku       bool
	flagConcurrency int
	flagReviewFile  string
	flagDryRun      bool
	flagRecursive   bool
)

var rootCmd = &cobra.Command{
	Use:     "photocrit",
	Short:   "Analyze and categorize photographs using Claude's vision API",
	Version: version,
	Long: `photocrit analyzes a directory of photographs using Claude's vision API
and categorizes them into three tiers: failed shots, good shots, and keepers.

Typical workflow:
  photocrit analyze ./photos   # analyze images, produce photocrit-review.json
  photocrit apply ./photos     # move files, write photocrit-report.md`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagModel, "model", "claude-sonnet-4-6", "Claude model for individual analysis pass")
	rootCmd.PersistentFlags().BoolVar(&flagHaiku, "haiku", false, "Use claude-haiku-4-5 for individual pass (cheaper, faster)")
	rootCmd.PersistentFlags().BoolVar(&flagOpus, "opus", false, "Use claude-opus-4-6 for comparative pass")
	rootCmd.PersistentFlags().IntVar(&flagConcurrency, "concurrency", 5, "Max parallel Claude API calls")
	rootCmd.PersistentFlags().StringVar(&flagReviewFile, "review-file", "", "Path to review file (default: <dir>/photocrit-review.json)")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "Print what would happen without writing files")
	rootCmd.PersistentFlags().BoolVar(&flagRecursive, "recursive", false, "Recursively scan subdirectories")
}
