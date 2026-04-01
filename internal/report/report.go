// Package report generates the markdown report after the apply command.
package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/photocrit/photocrit/internal/reviewer"
)

// Generate builds the markdown report string from a ReviewFile.
func Generate(rf reviewer.ReviewFile) string {
	var sb strings.Builder

	// Header
	sb.WriteString("# photocrit Report\n")
	sb.WriteString(fmt.Sprintf("**Directory:** %s\n", rf.Directory))
	sb.WriteString(fmt.Sprintf("**Analyzed:** %s\n", rf.AnalyzedAt.Format("2006-01-02 15:04")))
	sb.WriteString(fmt.Sprintf("**Model:** %s\n\n", rf.Model))

	// Count by effective category
	counts := make(map[reviewer.Category]int)
	for _, p := range rf.Photos {
		counts[reviewer.EffectiveCategory(p)]++
	}
	total := len(rf.Photos)

	// Summary table
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Category | Count |\n")
	sb.WriteString("|---|---|\n")
	sb.WriteString(fmt.Sprintf("| Keepers | %d |\n", counts[reviewer.CategoryKeeper]))
	sb.WriteString(fmt.Sprintf("| Good | %d |\n", counts[reviewer.CategoryGood]))
	sb.WriteString(fmt.Sprintf("| Failed | %d |\n", counts[reviewer.CategoryFailed]))
	sb.WriteString(fmt.Sprintf("| **Total** | **%d** |\n\n", total))
	sb.WriteString("---\n\n")

	// Sections per category
	writeSection(&sb, rf.Photos, reviewer.CategoryKeeper, "Keepers", counts[reviewer.CategoryKeeper])
	writeSection(&sb, rf.Photos, reviewer.CategoryGood, "Good Shots", counts[reviewer.CategoryGood])
	writeSection(&sb, rf.Photos, reviewer.CategoryFailed, "Failed Shots", counts[reviewer.CategoryFailed])

	// Groups section (only multi-image groups)
	var multiGroups []reviewer.Group
	for _, g := range rf.Groups {
		if len(g.Filenames) > 1 {
			multiGroups = append(multiGroups, g)
		}
	}
	if len(multiGroups) > 0 {
		sb.WriteString("## Groups\n\n")
		for _, g := range multiGroups {
			sb.WriteString(fmt.Sprintf("### %s — %d images\n", g.ID, len(g.Filenames)))
			sb.WriteString(strings.Join(namesWithoutExt(g.Filenames), " · "))
			sb.WriteString("\n\n")
			if g.Summary != "" {
				sb.WriteString(g.Summary)
				sb.WriteString("\n\n")
			}
			sb.WriteString("---\n\n")
		}
	}

	return sb.String()
}

// writeSection appends a section for one category to the builder.
func writeSection(sb *strings.Builder, photos []reviewer.PhotoDecision, cat reviewer.Category, heading string, count int) {
	sb.WriteString(fmt.Sprintf("## %s (%d)\n\n", heading, count))

	for _, p := range photos {
		if reviewer.EffectiveCategory(p) != cat {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s\n", p.Filename))
		sb.WriteString(fmt.Sprintf("**Category:** %s\n", p.Category))
		if p.Override != nil {
			sb.WriteString(fmt.Sprintf("**Override:** %s\n", *p.Override))
		}
		if p.Technical != "" {
			sb.WriteString(fmt.Sprintf("**Technical:** %s\n\n", p.Technical))
		}
		if p.Reasoning != "" {
			sb.WriteString(p.Reasoning)
			sb.WriteString("\n\n")
		}
		if len(p.Strengths) > 0 {
			sb.WriteString(fmt.Sprintf("**Strengths:** %s\n", strings.Join(p.Strengths, " · ")))
		}
		if len(p.Weaknesses) > 0 {
			sb.WriteString(fmt.Sprintf("**Weaknesses:** %s\n", strings.Join(p.Weaknesses, " · ")))
		}
		sb.WriteString("\n---\n\n")
	}
}

// namesWithoutExt strips extensions from filenames for display.
func namesWithoutExt(filenames []string) []string {
	out := make([]string, len(filenames))
	for i, f := range filenames {
		out[i] = strings.TrimSuffix(f, filepath.Ext(f))
	}
	return out
}

// WriteReport writes the markdown content to <dir>/photocrit-report.md.
func WriteReport(dir string, content string) error {
	path := filepath.Join(dir, "photocrit-report.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write report %s: %w", path, err)
	}
	return nil
}
