package mahjong

import (
	"fmt"
	"strings"
)

const tileKinds = 27

func parseTile(code string) (int, error) {
	t := strings.ToLower(strings.TrimSpace(code))
	if len(t) != 2 {
		return -1, fmt.Errorf("invalid tile %q", code)
	}
	suit := t[0]
	rank := int(t[1] - '0')
	if rank < 1 || rank > 9 {
		return -1, fmt.Errorf("invalid tile rank %q", code)
	}
	suitIdx := -1
	switch suit {
	case 'm', 'w':
		suitIdx = 0
	case 'p', 't':
		suitIdx = 1
	case 's', 'b':
		suitIdx = 2
	default:
		return -1, fmt.Errorf("invalid tile suit %q", code)
	}
	return suitIdx*9 + (rank - 1), nil
}

func tileString(idx int) string {
	suit := "m"
	switch idx / 9 {
	case 1:
		suit = "p"
	case 2:
		suit = "s"
	}
	rank := (idx % 9) + 1
	return fmt.Sprintf("%s%d", suit, rank)
}

func parseSuit(code string) (int, error) {
	s := strings.ToLower(strings.TrimSpace(code))
	switch s {
	case "", "none":
		return -1, nil
	case "m", "w", "wan":
		return 0, nil
	case "p", "t", "tong":
		return 1, nil
	case "s", "b", "tiao":
		return 2, nil
	default:
		return -1, fmt.Errorf("invalid missingSuit %q", code)
	}
}

func addTiles(counts *[tileKinds]int, tiles []string) error {
	for _, t := range tiles {
		idx, err := parseTile(t)
		if err != nil {
			return err
		}
		counts[idx]++
	}
	return nil
}

func sumCounts(counts [tileKinds]int) int {
	total := 0
	for _, c := range counts {
		total += c
	}
	return total
}

func suitOf(idx int) int {
	return idx / 9
}
