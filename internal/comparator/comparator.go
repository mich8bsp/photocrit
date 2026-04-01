// Package comparator runs the comparative analysis pass over image groups.
package comparator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/photocrit/photocrit/internal/analyzer"
	"github.com/photocrit/photocrit/internal/reviewer"
	"github.com/photocrit/photocrit/internal/scanner"
)

// groupSchema is the JSON schema for the comparative pass response.
const groupSchema = `{
  "rankings": [
    {
      "filename": "IMG_0041.jpg",
      "rank": 1,
      "category": "keeper",
      "reasoning": "brief note on any category change"
    }
  ],
  "summary": "Brief summary of the group and which image stands out"
}`

func comparativeSystemPrompt() string {
	return `You are an expert photography critic. You are comparing a group of photographs that were taken in the same burst or scene sequence.

Your task is to:
1. Rank the images from best (rank 1) to worst
2. Confirm or revise each image's category (failed/good/keeper) based on how they compare to each other
3. Identify which image best captures the moment

Respond ONLY with valid JSON matching this schema:
` + groupSchema
}

// groupRanking is the per-image element of the comparative response.
type groupRanking struct {
	Filename  string `json:"filename"`
	Rank      int    `json:"rank"`
	Category  string `json:"category"`
	Reasoning string `json:"reasoning"`
}

// groupResponse is the full comparative response.
type groupResponse struct {
	Rankings []groupRanking `json:"rankings"`
	Summary  string         `json:"summary"`
}

// CompareGroup sends all images in a group to Claude for comparative ranking.
// Only processes groups with 2+ images. Returns updated decisions with GroupRank set.
func CompareGroup(
	ctx context.Context,
	client *anthropic.Client,
	model string,
	group []scanner.ImageFile,
	decisions []reviewer.PhotoDecision,
) ([]reviewer.PhotoDecision, string, error) {
	if len(group) < 2 {
		// Singleton — just return as-is with rank 1
		if len(group) == 1 {
			updated := make([]reviewer.PhotoDecision, len(decisions))
			copy(updated, decisions)
			for i := range updated {
				updated[i].GroupRank = 1
			}
			return updated, "", nil
		}
		return decisions, "", nil
	}

	// Build decision map for quick lookup
	decisionMap := make(map[string]reviewer.PhotoDecision, len(decisions))
	for _, d := range decisions {
		decisionMap[d.Filename] = d
	}

	// Build content blocks: one image + label per file
	var blocks []anthropic.ContentBlockParamUnion

	for _, img := range group {
		mt, encoded, err := analyzer.LoadAndEncode(img.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s in comparative pass — %v\n", img.Filename, err)
			continue
		}
		blocks = append(blocks, anthropic.NewImageBlockBase64(mt, encoded))
		blocks = append(blocks, anthropic.NewTextBlock(fmt.Sprintf("Image: %s", img.Filename)))
	}

	if len(blocks) == 0 {
		return decisions, "", nil
	}

	// Build individual assessment summary
	var sb strings.Builder
	sb.WriteString("Individual assessments:\n")
	for _, img := range group {
		d, ok := decisionMap[img.Filename]
		if !ok {
			continue
		}
		sb.WriteString(fmt.Sprintf("\n%s — category: %s\n  Technical: %s\n  Reasoning: %s\n",
			d.Filename, d.Category, d.Technical, d.Reasoning))
	}
	sb.WriteString("\nPlease rank these images and confirm/revise their categories.")
	blocks = append(blocks, anthropic.NewTextBlock(sb.String()))

	msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 1024,
		System: []anthropic.TextBlockParam{
			{Text: comparativeSystemPrompt()},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(blocks...),
		},
	})
	if err != nil {
		return decisions, "", fmt.Errorf("comparative API call: %w", err)
	}

	// Extract text
	text := ""
	for _, block := range msg.Content {
		if block.Type == "text" {
			text = block.AsText().Text
			break
		}
	}
	if text == "" {
		return decisions, "", fmt.Errorf("empty comparative response")
	}

	// Strip code fences
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		lines := strings.SplitN(text, "\n", 2)
		if len(lines) == 2 {
			text = lines[1]
		}
		if idx := strings.LastIndex(text, "```"); idx >= 0 {
			text = text[:idx]
		}
		text = strings.TrimSpace(text)
	}

	var gr groupResponse
	if err := json.Unmarshal([]byte(text), &gr); err != nil {
		return decisions, "", fmt.Errorf("parse comparative response: %w (raw: %.200s)", err, text)
	}

	// Apply rankings
	updated := make([]reviewer.PhotoDecision, len(decisions))
	copy(updated, decisions)

	rankMap := make(map[string]groupRanking, len(gr.Rankings))
	for _, r := range gr.Rankings {
		rankMap[r.Filename] = r
	}

	for i, d := range updated {
		if r, ok := rankMap[d.Filename]; ok {
			updated[i].GroupRank = r.Rank
			// Update category if Claude revised it
			newCat := reviewer.Category(r.Category)
			switch newCat {
			case reviewer.CategoryFailed, reviewer.CategoryGood, reviewer.CategoryKeeper:
				updated[i].Category = newCat
				if r.Reasoning != "" {
					updated[i].Reasoning = updated[i].Reasoning + " [Comparative: " + r.Reasoning + "]"
				}
			}
		}
	}

	return updated, gr.Summary, nil
}

// RunComparativePass iterates over all groups and runs CompareGroup for multi-image groups.
// Groups are processed sequentially to avoid overwhelming the API.
// Returns the updated full decisions slice and per-group summaries.
func RunComparativePass(
	ctx context.Context,
	client *anthropic.Client,
	model string,
	groups [][]scanner.ImageFile,
	allDecisions []reviewer.PhotoDecision,
) ([]reviewer.PhotoDecision, map[string]string, error) {
	// Build a mutable map from filename → decision for easy merging
	decMap := make(map[string]reviewer.PhotoDecision, len(allDecisions))
	for _, d := range allDecisions {
		decMap[d.Filename] = d
	}

	summaries := make(map[string]string)

	for gIdx, group := range groups {
		if len(group) < 2 {
			// Singleton — set GroupRank = 1 if decision exists
			if len(group) == 1 {
				fn := group[0].Filename
				if d, ok := decMap[fn]; ok {
					d.GroupRank = 1
					decMap[fn] = d
				}
			}
			continue
		}

		groupID := fmt.Sprintf("group_%03d", gIdx+1)

		// Gather decisions for this group
		var groupDecisions []reviewer.PhotoDecision
		for _, img := range group {
			if d, ok := decMap[img.Filename]; ok {
				groupDecisions = append(groupDecisions, d)
			}
		}

		fmt.Fprintf(os.Stderr, "  Comparing group %s (%d images)...\n", groupID, len(group))

		updated, summary, err := CompareGroup(ctx, client, model, group, groupDecisions)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: comparative pass failed for %s: %v\n", groupID, err)
			// Continue with unmodified decisions for this group
			continue
		}

		summaries[groupID] = summary

		// Merge updated decisions back
		for _, d := range updated {
			d.GroupID = groupID
			decMap[d.Filename] = d
		}
	}

	// Reconstruct ordered slice
	result := make([]reviewer.PhotoDecision, 0, len(allDecisions))
	for _, d := range allDecisions {
		if updated, ok := decMap[d.Filename]; ok {
			result = append(result, updated)
		} else {
			result = append(result, d)
		}
	}

	return result, summaries, nil
}
