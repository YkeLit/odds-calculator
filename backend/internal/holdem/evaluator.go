package holdem

import "sort"

type HandRank struct {
	Category int
	Values   [5]int
}

func CompareHandRank(a, b HandRank) int {
	if a.Category != b.Category {
		if a.Category > b.Category {
			return 1
		}
		return -1
	}
	for i := 0; i < len(a.Values); i++ {
		if a.Values[i] > b.Values[i] {
			return 1
		}
		if a.Values[i] < b.Values[i] {
			return -1
		}
	}
	return 0
}

func EvaluateSeven(cards []Card) HandRank {
	best := HandRank{Category: -1}
	n := len(cards)
	for a := 0; a < n-4; a++ {
		for b := a + 1; b < n-3; b++ {
			for c := b + 1; c < n-2; c++ {
				for d := c + 1; d < n-1; d++ {
					for e := d + 1; e < n; e++ {
						hand := []Card{cards[a], cards[b], cards[c], cards[d], cards[e]}
						r := evaluateFive(hand)
						if best.Category == -1 || CompareHandRank(r, best) > 0 {
							best = r
						}
					}
				}
			}
		}
	}
	return best
}

func evaluateFive(cards []Card) HandRank {
	ranks := make([]int, 5)
	suits := make([]byte, 5)
	rankCount := map[int]int{}
	for i, c := range cards {
		ranks[i] = c.Rank
		suits[i] = c.Suit
		rankCount[c.Rank]++
	}
	sort.Slice(ranks, func(i, j int) bool { return ranks[i] > ranks[j] })

	isFlush := true
	for i := 1; i < len(suits); i++ {
		if suits[i] != suits[0] {
			isFlush = false
			break
		}
	}

	unique := uniqueRanksDesc(ranks)
	straightHigh, isStraight := detectStraight(unique)

	if isStraight && isFlush {
		return HandRank{Category: 8, Values: [5]int{straightHigh}}
	}

	quads := 0
	trips := make([]int, 0)
	pairs := make([]int, 0)
	singles := make([]int, 0)
	for rank, count := range rankCount {
		switch count {
		case 4:
			quads = rank
		case 3:
			trips = append(trips, rank)
		case 2:
			pairs = append(pairs, rank)
		default:
			singles = append(singles, rank)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(trips)))
	sort.Sort(sort.Reverse(sort.IntSlice(pairs)))
	sort.Sort(sort.Reverse(sort.IntSlice(singles)))

	if quads > 0 {
		return HandRank{Category: 7, Values: [5]int{quads, singles[0]}}
	}

	if len(trips) > 0 && (len(pairs) > 0 || len(trips) > 1) {
		pairRank := 0
		if len(pairs) > 0 {
			pairRank = pairs[0]
		} else {
			pairRank = trips[1]
		}
		return HandRank{Category: 6, Values: [5]int{trips[0], pairRank}}
	}

	if isFlush {
		return HandRank{Category: 5, Values: [5]int{ranks[0], ranks[1], ranks[2], ranks[3], ranks[4]}}
	}

	if isStraight {
		return HandRank{Category: 4, Values: [5]int{straightHigh}}
	}

	if len(trips) > 0 {
		return HandRank{Category: 3, Values: [5]int{trips[0], singles[0], singles[1]}}
	}

	if len(pairs) >= 2 {
		return HandRank{Category: 2, Values: [5]int{pairs[0], pairs[1], singles[0]}}
	}

	if len(pairs) == 1 {
		return HandRank{Category: 1, Values: [5]int{pairs[0], singles[0], singles[1], singles[2]}}
	}

	return HandRank{Category: 0, Values: [5]int{ranks[0], ranks[1], ranks[2], ranks[3], ranks[4]}}
}

func uniqueRanksDesc(ranks []int) []int {
	seen := map[int]struct{}{}
	result := make([]int, 0, len(ranks))
	for _, r := range ranks {
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		result = append(result, r)
	}
	return result
}

func detectStraight(uniqueDesc []int) (int, bool) {
	if len(uniqueDesc) < 5 {
		return 0, false
	}
	if uniqueDesc[0] == 14 {
		hasFive, hasFour, hasThree, hasTwo := false, false, false, false
		for _, r := range uniqueDesc {
			switch r {
			case 5:
				hasFive = true
			case 4:
				hasFour = true
			case 3:
				hasThree = true
			case 2:
				hasTwo = true
			}
		}
		if hasFive && hasFour && hasThree && hasTwo {
			return 5, true
		}
	}
	for i := 0; i <= len(uniqueDesc)-5; i++ {
		if uniqueDesc[i]-1 == uniqueDesc[i+1] &&
			uniqueDesc[i+1]-1 == uniqueDesc[i+2] &&
			uniqueDesc[i+2]-1 == uniqueDesc[i+3] &&
			uniqueDesc[i+3]-1 == uniqueDesc[i+4] {
			return uniqueDesc[i], true
		}
	}
	return 0, false
}
