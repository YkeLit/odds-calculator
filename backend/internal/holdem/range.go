package holdem

import (
	"fmt"
	"strconv"
	"strings"
)

// Combo array represents all 1326 possible hand combinations.
// Stored as an array of probabilities.
type Range [1326]float64

// HandCombo represents a specific hole card combination.
type HandCombo struct {
	Card1 Card
	Card2 Card
}

func (c HandCombo) Code() string {
	return c.Card1.Code + c.Card2.Code
}

// AllCombos returns the 1326 possible combos ordered consistently.
func AllCombos() []HandCombo {
	deck := FullDeck()
	combos := make([]HandCombo, 0, 1326)
	for i := 0; i < len(deck)-1; i++ {
		for j := i + 1; j < len(deck); j++ {
			// keep higher rank first to canonicalize
			c1 := deck[i]
			c2 := deck[j]
			if c2.Rank > c1.Rank || (c2.Rank == c1.Rank && c2.Suit > c1.Suit) {
				c1, c2 = c2, c1
			}
			combos = append(combos, HandCombo{c1, c2})
		}
	}
	return combos
}

// ComboIndexMaps holds mapping for fast O(1) index lookup.
var (
	GlobalCombos []HandCombo
	ComboToIndex map[string]int
)

func init() {
	GlobalCombos = AllCombos()
	ComboToIndex = make(map[string]int, 1326)
	for i, c := range GlobalCombos {
		ComboToIndex[c.Code()] = i
		// canonicalizing reverse string as well for safety
		revCode := c.Card2.Code + c.Card1.Code
		ComboToIndex[revCode] = i
	}
}

// NewEmptyRange returns a range where all combos have 0 weight.
func NewEmptyRange() Range {
	return Range{}
}

// NewFullRange returns a range where all 1326 combos have weight 1.0 (100% distribution).
func NewFullRange() Range {
	r := Range{}
	for i := 0; i < 1326; i++ {
		r[i] = 1.0
	}
	return r
}

// Normalize modifies the Range so sum of weights equals 1.0.
// If all weights are 0, it returns an error.
func (r *Range) Normalize() error {
	sum := 0.0
	for _, w := range r {
		sum += w
	}
	if sum <= 0 {
		return fmt.Errorf("range has zero total weight")
	}
	for i := range r {
		r[i] /= sum
	}
	return nil
}

// ParseRangeOverride parses syntax like "AA,AKs,AQo,JJ:0.7"
// Returns a filled Range. Note this range is NOT normalized yet.
func ParseRangeOverride(override string) (Range, error) {
	rng := NewEmptyRange()
	if override == "" {
		return NewFullRange(), nil
	}

	parts := strings.Split(override, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		weight := 1.0
		if strings.Contains(part, ":") {
			sp := strings.SplitN(part, ":", 2)
			part = sp[0]
			val, err := strconv.ParseFloat(sp[1], 64)
			if err != nil {
				return rng, fmt.Errorf("invalid weight %q in token %q", sp[1], part)
			}
			if val < 0 || val > 1 {
				return rng, fmt.Errorf("weight must be between 0 and 1: %f in token %q", val, part)
			}
			weight = val
		}

		// parse the token logic
		err := parseTokenAndApply(part, weight, &rng)
		if err != nil {
			return rng, err
		}
	}
	return rng, nil
}

// parseTokenAndApply handles standard pocket patterns: "AA", "AK", "AKs", "AKo"
func parseTokenAndApply(token string, weight float64, rng *Range) error {
	token = strings.ToUpper(token)
	if len(token) < 2 || len(token) > 3 {
		return fmt.Errorf("invalid range token length: %s", token)
	}

	rank1Code := string(token[0])
	rank2Code := string(token[1])
	
	if rank1Code == "1" && token[1] == '0' { // Handle 10 as T
		rank1Code = "T"
		if len(token) > 2 {
			rank2Code = string(token[2])
			if len(token) == 4 { // e.g. 10 10 -> will fail length validation above anyway, just fallback basic
				return fmt.Errorf("unsupported 10 formatting: %s, use T", token)
			}
		} else {
			return fmt.Errorf("invalid token: %s", token)
		}
	}
	
	r1, ok1 := rankMap[rank1Code]
	r2, ok2 := rankMap[rank2Code]

	if !ok1 || !ok2 {
		return fmt.Errorf("invalid card ranks in token: %s", token)
	}

	// Canonicalize: r1 >= r2
	if r2 > r1 {
		r1, r2 = r2, r1
	}

	isSuited := false
	isOffsuit := false
	if len(token) == 3 {
		suffix := strings.ToLower(string(token[2]))
		if suffix == "s" {
			isSuited = true
		} else if suffix == "o" {
			isOffsuit = true
		} else {
			return fmt.Errorf("invalid suffix %s in token %s", suffix, token)
		}
	}

	if r1 == r2 {
		if isSuited { // e.g. "AAs" doesn't make sense
			return fmt.Errorf("pocket pair cannot be suited: %s", token)
		}
		// Apply pairs (6 combos)
		applyPair(r1, weight, rng)
		return nil
	}

	// Non-pairs
	if isSuited {
		applySuited(r1, r2, weight, rng)
	} else if isOffsuit {
		applyOffsuit(r1, r2, weight, rng)
	} else {
		applySuited(r1, r2, weight, rng)
		applyOffsuit(r1, r2, weight, rng)
	}

	return nil
}

var suits = []byte{'s', 'h', 'd', 'c'}

func applyPair(rank int, weight float64, rng *Range) {
	for i := 0; i < len(suits)-1; i++ {
		for j := i + 1; j < len(suits); j++ {
			c1Code := fmt.Sprintf("%s%c", rankToCode[rank], suits[i])
			c2Code := fmt.Sprintf("%s%c", rankToCode[rank], suits[j])
			if idx, ok := ComboToIndex[c1Code+c2Code]; ok {
				rng[idx] = weight
			}
		}
	}
}

func applySuited(r1, r2 int, weight float64, rng *Range) {
	for _, s := range suits {
		c1Code := fmt.Sprintf("%s%c", rankToCode[r1], s)
		c2Code := fmt.Sprintf("%s%c", rankToCode[r2], s)
		if idx, ok := ComboToIndex[c1Code+c2Code]; ok {
			rng[idx] = weight
		}
	}
}

func applyOffsuit(r1, r2 int, weight float64, rng *Range) {
	for _, s1 := range suits {
		for _, s2 := range suits {
			if s1 == s2 {
				continue
			}
			c1Code := fmt.Sprintf("%s%c", rankToCode[r1], s1)
			c2Code := fmt.Sprintf("%s%c", rankToCode[r2], s2)
			if idx, ok := ComboToIndex[c1Code+c2Code]; ok {
				rng[idx] = weight
			}
		}
	}
}

// Built-in positions defaults. 
// A simplistic default representation for position ranges.
var DefaultPositionRanges = map[string]string{
	"UTG": "77,AQs,AKo,AJs,KQs,KJs", // Simplistic tight
	"MP":  "55,AJo,A10s,KJo,QTs,JTs", // Added some 
	"CO":  "22,A2s,A9o,K9s,Q9s,J9s,T9s", // Very loose example strings
	"BTN": "22,A2s,A2o,K2s,K9o,Q8s,J8s,T8s,98s,87s", 
	"SB":  "22,A2s,A2o,K2s,Q2s,J5s,T7s", 
	"BB":  "22,A2s,A2o,K2s,Q2s,J2s", 
}
