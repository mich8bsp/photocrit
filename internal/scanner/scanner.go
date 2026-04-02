// Package scanner finds image files in a directory and groups them by sequence.
package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// MaxImages is the hard cap on images per run.
const MaxImages = 200

// SupportedExtensions lists the file extensions photocrit will process.
var SupportedExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".heic": true,
	".tiff": true,
	".tif":  true,
	".webp": true,
	".cr2":  true,
	".nef":  true,
	".arw":  true,
	".dng":  true,
	".orf":  true,
	".rw2":  true,
}

// CategoryFolders are subdirectories created by the apply command; skip them during scan.
var CategoryFolders = map[string]bool{
	"_failed":   true,
	"_good":     true,
	"_keepers":  true,
}

// ImageFile represents a discovered image file.
type ImageFile struct {
	Path      string
	Filename  string
	Extension string
	SizeBytes int64
}

// Scan walks dir (recursively if recursive=true) and returns all image files found.
// Files inside category subdirectories are skipped.
// If limit > 0, returns an error if more than limit images are found.
func Scan(dir string, recursive bool, limit int) ([]ImageFile, error) {
	var images []ImageFile

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Skip category folders regardless of depth
			if CategoryFolders[info.Name()] {
				return filepath.SkipDir
			}
			// If not recursive, skip subdirectories (but not the root dir itself)
			if !recursive && path != dir {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(info.Name()))
		if !SupportedExtensions[ext] {
			return nil
		}

		images = append(images, ImageFile{
			Path:      path,
			Filename:  info.Name(),
			Extension: ext,
			SizeBytes: info.Size(),
		})

		if limit > 0 && len(images) > limit {
			return fmt.Errorf("exceeded %d image limit", limit)
		}
		return nil
	}

	if err := filepath.Walk(dir, walkFn); err != nil {
		if limit > 0 && strings.Contains(err.Error(), fmt.Sprintf("exceeded %d image limit", limit)) {
			return nil, fmt.Errorf(
				"found more than %d images in %s — use --batch to process in chunks",
				limit, dir,
			)
		}
		return nil, fmt.Errorf("scan directory %s: %w", dir, err)
	}

	return images, nil
}

// numericSuffixRe matches a trailing numeric sequence in a filename stem.
var numericSuffixRe = regexp.MustCompile(`(\d+)$`)

// extractNumber extracts the trailing integer from a filename stem (no extension).
// Returns -1 if no number is found.
func extractNumber(filename string) int {
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	m := numericSuffixRe.FindString(stem)
	if m == "" {
		return -1
	}
	n, err := strconv.Atoi(m)
	if err != nil {
		return -1
	}
	return n
}

// GroupBySequence groups images with consecutive numeric suffixes (gap ≤ 3)
// into candidate burst groups. Singletons are placed in their own single-element group.
func GroupBySequence(images []ImageFile) [][]ImageFile {
	if len(images) == 0 {
		return nil
	}

	// Sort images by extracted number (stable sort to preserve original order for equal numbers)
	sorted := make([]ImageFile, len(images))
	copy(sorted, images)
	sort.SliceStable(sorted, func(i, j int) bool {
		ni := extractNumber(sorted[i].Filename)
		nj := extractNumber(sorted[j].Filename)
		if ni != nj {
			return ni < nj
		}
		return sorted[i].Filename < sorted[j].Filename
	})

	var groups [][]ImageFile
	current := []ImageFile{sorted[0]}

	for i := 1; i < len(sorted); i++ {
		prev := extractNumber(sorted[i-1].Filename)
		curr := extractNumber(sorted[i].Filename)

		// Group if both have numbers and the gap is ≤ 3
		if prev >= 0 && curr >= 0 && (curr-prev) <= 3 {
			current = append(current, sorted[i])
		} else {
			groups = append(groups, current)
			current = []ImageFile{sorted[i]}
		}
	}
	groups = append(groups, current)

	return groups
}
