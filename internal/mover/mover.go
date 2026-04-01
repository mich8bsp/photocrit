// Package mover handles file move operations for the apply command.
package mover

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/photocrit/photocrit/internal/reviewer"
)

// categoryFolder maps a category to its destination folder name.
func categoryFolder(cat reviewer.Category) string {
	switch cat {
	case reviewer.CategoryFailed:
		return "_failed"
	case reviewer.CategoryGood:
		return "_good"
	case reviewer.CategoryKeeper:
		return "_keepers"
	default:
		return "_good"
	}
}

// MoveResult summarizes the outcome of the move operation.
type MoveResult struct {
	Moved   int
	Skipped int
	ByCategory map[reviewer.Category]int
}

// Move moves each photo to its appropriate subdirectory.
// If dryRun is true, it prints intended moves without executing them.
func Move(sourceDir string, decisions []reviewer.PhotoDecision, dryRun bool) (MoveResult, error) {
	result := MoveResult{
		ByCategory: make(map[reviewer.Category]int),
	}

	// Create destination directories (even in dry-run we check paths)
	if !dryRun {
		for _, folder := range []string{"_failed", "_good", "_keepers"} {
			dest := filepath.Join(sourceDir, folder)
			if err := os.MkdirAll(dest, 0755); err != nil {
				return result, fmt.Errorf("create directory %s: %w", dest, err)
			}
		}
	}

	for _, d := range decisions {
		cat := reviewer.EffectiveCategory(d)
		destFolder := categoryFolder(cat)
		srcPath := filepath.Join(sourceDir, d.Filename)
		destDir := filepath.Join(sourceDir, destFolder)
		destPath := uniquePath(destDir, d.Filename)

		// Check source exists
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: source file not found, skipping: %s\n", srcPath)
			result.Skipped++
			continue
		}

		if dryRun {
			fmt.Printf("would move %s → %s/%s\n", d.Filename, destFolder, filepath.Base(destPath))
			result.Moved++
			result.ByCategory[cat]++
			continue
		}

		if err := os.Rename(srcPath, destPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to move %s: %v\n", d.Filename, err)
			result.Skipped++
			continue
		}

		result.Moved++
		result.ByCategory[cat]++
	}

	return result, nil
}

// uniquePath returns a destination path that does not already exist,
// appending _1, _2, etc. to the stem if needed.
func uniquePath(dir, filename string) string {
	candidate := filepath.Join(dir, filename)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}

	ext := filepath.Ext(filename)
	stem := strings.TrimSuffix(filename, ext)

	for i := 1; ; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s_%d%s", stem, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
