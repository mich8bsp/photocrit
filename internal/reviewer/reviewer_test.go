package reviewer

import (
	"testing"
)

func TestEffectiveCategory_NoOverride(t *testing.T) {
	pd := PhotoDecision{
		Filename: "test.jpg",
		Category: CategoryKeeper,
		Override: nil,
	}
	if got := EffectiveCategory(pd); got != CategoryKeeper {
		t.Errorf("expected keeper, got %s", got)
	}
}

func TestEffectiveCategory_WithOverride(t *testing.T) {
	override := CategoryFailed
	pd := PhotoDecision{
		Filename: "test.jpg",
		Category: CategoryKeeper,
		Override: &override,
	}
	if got := EffectiveCategory(pd); got != CategoryFailed {
		t.Errorf("expected failed (override), got %s", got)
	}
}

func TestEffectiveCategory_OverrideGood(t *testing.T) {
	override := CategoryGood
	pd := PhotoDecision{
		Filename: "test.jpg",
		Category: CategoryFailed,
		Override: &override,
	}
	if got := EffectiveCategory(pd); got != CategoryGood {
		t.Errorf("expected good (override), got %s", got)
	}
}
