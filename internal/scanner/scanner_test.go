package scanner

import (
	"testing"
)

func TestGroupBySequence_Sequential(t *testing.T) {
	images := []ImageFile{
		{Filename: "IMG_0041.jpg"},
		{Filename: "IMG_0042.jpg"},
		{Filename: "IMG_0043.jpg"},
		{Filename: "IMG_0047.jpg"}, // gap of 4 — new group
		{Filename: "IMG_0048.jpg"},
	}

	groups := GroupBySequence(images)

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if len(groups[0]) != 3 {
		t.Errorf("expected group 0 to have 3 images, got %d", len(groups[0]))
	}
	if len(groups[1]) != 2 {
		t.Errorf("expected group 1 to have 2 images, got %d", len(groups[1]))
	}
}

func TestGroupBySequence_NonSequential(t *testing.T) {
	images := []ImageFile{
		{Filename: "portrait_001.jpg"},
		{Filename: "landscape_042.jpg"},
		{Filename: "street_100.jpg"},
	}

	groups := GroupBySequence(images)

	if len(groups) != 3 {
		t.Fatalf("expected 3 singleton groups, got %d", len(groups))
	}
	for i, g := range groups {
		if len(g) != 1 {
			t.Errorf("group %d: expected 1 image, got %d", i, len(g))
		}
	}
}

func TestGroupBySequence_GapOf3(t *testing.T) {
	// Gap of exactly 3 should still be in the same group
	images := []ImageFile{
		{Filename: "IMG_0010.jpg"},
		{Filename: "IMG_0013.jpg"},
	}

	groups := GroupBySequence(images)

	if len(groups) != 1 {
		t.Fatalf("expected 1 group (gap=3), got %d", len(groups))
	}
}

func TestGroupBySequence_GapOf4(t *testing.T) {
	// Gap of 4 should split into separate groups
	images := []ImageFile{
		{Filename: "IMG_0010.jpg"},
		{Filename: "IMG_0014.jpg"},
	}

	groups := GroupBySequence(images)

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups (gap=4), got %d", len(groups))
	}
}

func TestGroupBySequence_NoNumbers(t *testing.T) {
	images := []ImageFile{
		{Filename: "photo.jpg"},
		{Filename: "image.png"},
	}

	groups := GroupBySequence(images)

	// No numbers means each is its own group
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
}

func TestGroupBySequence_Empty(t *testing.T) {
	groups := GroupBySequence(nil)
	if groups != nil {
		t.Errorf("expected nil for empty input, got %v", groups)
	}
}
