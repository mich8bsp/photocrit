package mover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/photocrit/photocrit/internal/reviewer"
)

func TestMove_DryRun(t *testing.T) {
	// Create a temp directory with a dummy image file
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test.jpg")
	if err := os.WriteFile(imgPath, []byte("fake jpeg"), 0644); err != nil {
		t.Fatal(err)
	}

	decisions := []reviewer.PhotoDecision{
		{Filename: "test.jpg", Category: reviewer.CategoryKeeper},
	}

	result, err := Move(tmpDir, decisions, true /* dryRun */)
	if err != nil {
		t.Fatalf("Move dry-run failed: %v", err)
	}

	// In dry-run: file should still be in original location
	if _, err := os.Stat(imgPath); os.IsNotExist(err) {
		t.Error("dry-run should not move files — but file was moved")
	}

	// Should report 1 "moved" (counted as would-move)
	if result.Moved != 1 {
		t.Errorf("expected 1 move, got %d", result.Moved)
	}
}

func TestMove_ActualMove(t *testing.T) {
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test.jpg")
	if err := os.WriteFile(imgPath, []byte("fake jpeg"), 0644); err != nil {
		t.Fatal(err)
	}

	decisions := []reviewer.PhotoDecision{
		{Filename: "test.jpg", Category: reviewer.CategoryGood},
	}

	result, err := Move(tmpDir, decisions, false)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	// Original should be gone
	if _, err := os.Stat(imgPath); !os.IsNotExist(err) {
		t.Error("expected original file to be moved away")
	}

	// File should be in _good/
	destPath := filepath.Join(tmpDir, "_good", "test.jpg")
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		t.Error("expected file in _good/ directory")
	}

	if result.Moved != 1 {
		t.Errorf("expected 1 moved, got %d", result.Moved)
	}
}

func TestMove_MissingSource(t *testing.T) {
	tmpDir := t.TempDir()

	decisions := []reviewer.PhotoDecision{
		{Filename: "nonexistent.jpg", Category: reviewer.CategoryFailed},
	}

	result, err := Move(tmpDir, decisions, false)
	if err != nil {
		t.Fatalf("Move should not error on missing source: %v", err)
	}
	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", result.Skipped)
	}
}

func TestMove_Collision(t *testing.T) {
	tmpDir := t.TempDir()

	// Create original file
	imgPath := filepath.Join(tmpDir, "test.jpg")
	if err := os.WriteFile(imgPath, []byte("fake jpeg v1"), 0644); err != nil {
		t.Fatal(err)
	}

	// Pre-populate destination to force collision
	if err := os.MkdirAll(filepath.Join(tmpDir, "_good"), 0755); err != nil {
		t.Fatal(err)
	}
	existingDest := filepath.Join(tmpDir, "_good", "test.jpg")
	if err := os.WriteFile(existingDest, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	decisions := []reviewer.PhotoDecision{
		{Filename: "test.jpg", Category: reviewer.CategoryGood},
	}

	result, err := Move(tmpDir, decisions, false)
	if err != nil {
		t.Fatalf("Move with collision failed: %v", err)
	}

	// Should be renamed to test_1.jpg
	renamedPath := filepath.Join(tmpDir, "_good", "test_1.jpg")
	if _, err := os.Stat(renamedPath); os.IsNotExist(err) {
		t.Error("expected renamed file test_1.jpg in _good/")
	}
	if result.Moved != 1 {
		t.Errorf("expected 1 moved, got %d", result.Moved)
	}
}
