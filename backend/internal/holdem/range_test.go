package holdem

import (
	"testing"
)

func TestAllCombos(t *testing.T) {
	combos := AllCombos()
	if len(combos) != 1326 {
		t.Fatalf("expected 1326 combos, got %d", len(combos))
	}
}

func TestParseRangeOverride_BasicPairs(t *testing.T) {
	rng, err := ParseRangeOverride("AA")
	if err != nil {
		t.Fatalf("unexpected error parsing AA: %v", err)
	}

	count := 0
	for _, w := range rng {
		if w > 0 {
			count++
		}
	}
	if count != 6 {
		t.Errorf("expected 6 combinations for AA, got %d", count)
	}
}

func TestParseRangeOverride_SuitedAndOffsuit(t *testing.T) {
	rng, err := ParseRangeOverride("AKs, AJo:0.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	aksCount := 0
	ajoCount := 0
	for i, w := range rng {
		if w > 0 {
			combo := GlobalCombos[i]
			isSuited := combo.Card1.Suit == combo.Card2.Suit
			if combo.Card1.Rank == 14 && combo.Card2.Rank == 13 && isSuited {
				aksCount++
				if w != 1.0 {
					t.Errorf("expected weight 1.0 for AKs, got %f", w)
				}
			} else if combo.Card1.Rank == 14 && combo.Card2.Rank == 11 && !isSuited {
				ajoCount++
				if w != 0.5 {
					t.Errorf("expected weight 0.5 for AJo, got %f", w)
				}
			}
		}
	}

	if aksCount != 4 {
		t.Errorf("expected 4 suited AKs combos, got %d", aksCount)
	}
	if ajoCount != 12 {
		t.Errorf("expected 12 offsuit AJo combos, got %d", ajoCount)
	}
}

func TestRange_Normalize(t *testing.T) {
	rng, _ := ParseRangeOverride("AA, KK")
	// 6 + 6 = 12 combos with weight 1
	err := rng.Normalize()
	if err != nil {
		t.Fatalf("unexpected normalize err: %v", err)
	}

	sum := 0.0
	for _, w := range rng {
		sum += w
	}
	if sum < 0.999 || sum > 1.001 { // Check precision
		t.Errorf("expected sum around 1.0, got %f", sum)
	}
}
