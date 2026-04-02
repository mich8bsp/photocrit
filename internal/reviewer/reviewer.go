// Package reviewer defines the review file schema and read/write operations.
package reviewer

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Category represents the tier assigned to a photo.
type Category string

const (
	CategoryFailed Category = "failed"
	CategoryGood   Category = "good"
	CategoryKeeper Category = "keeper"
)

// PhotoDecision holds the analysis result for a single photo.
type PhotoDecision struct {
	Filename   string    `json:"filename"`
	Category   Category  `json:"category"`
	Override   *Category `json:"override"`
	Score      *int      `json:"score,omitempty"` // 0–100, keepers only
	Reasoning  string    `json:"reasoning"`
	Technical  string    `json:"technical"`
	Strengths  []string  `json:"strengths"`
	Weaknesses []string  `json:"weaknesses"`
	GroupID    string    `json:"group_id,omitempty"`
	GroupRank  int       `json:"group_rank,omitempty"`
}

// Group represents a burst or scene group of related photos.
type Group struct {
	ID        string   `json:"id"`
	Filenames []string `json:"filenames"`
	Summary   string   `json:"summary"`
}

// ReviewFile is the top-level structure written to photocrit-review.json.
type ReviewFile struct {
	Version    string          `json:"version"`
	Directory  string          `json:"directory"`
	AnalyzedAt time.Time       `json:"analyzed_at"`
	Model      string          `json:"model"`
	Photos     []PhotoDecision `json:"photos"`
	Groups     []Group         `json:"groups"`
}

// EffectiveCategory returns Override if set, otherwise Category.
func EffectiveCategory(pd PhotoDecision) Category {
	if pd.Override != nil {
		return *pd.Override
	}
	return pd.Category
}

// WriteReviewFile serializes rf to the given path as formatted JSON.
func WriteReviewFile(path string, rf ReviewFile) error {
	data, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal review file: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write review file %s: %w", path, err)
	}
	return nil
}

// ReadReviewFile reads and deserializes a review file from path.
func ReadReviewFile(path string) (ReviewFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ReviewFile{}, fmt.Errorf("review file not found at %s — run `photocrit analyze` first", path)
		}
		return ReviewFile{}, fmt.Errorf("read review file %s: %w", path, err)
	}
	var rf ReviewFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return ReviewFile{}, fmt.Errorf("parse review file %s: %w", path, err)
	}
	return rf, nil
}
