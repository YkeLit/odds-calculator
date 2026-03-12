package mahjong

import (
	"fmt"
	"sort"
	"strings"

	"odds-calculator/backend/internal/models"
)

type meldInfo struct {
	Tiles     []int
	IsTriplet bool
}

func parseMelds(raw [][]string) ([]meldInfo, error) {
	result := make([]meldInfo, 0, len(raw))
	for _, m := range raw {
		if len(m) != 3 && len(m) != 4 {
			return nil, fmt.Errorf("meld length must be 3 or 4")
		}
		tiles := make([]int, len(m))
		for i, code := range m {
			idx, err := parseTile(code)
			if err != nil {
				return nil, err
			}
			tiles[i] = idx
		}
		sorted := make([]int, len(tiles))
		copy(sorted, tiles)
		sort.Ints(sorted)
		isTriplet := true
		for i := 1; i < len(sorted); i++ {
			if sorted[i] != sorted[0] {
				isTriplet = false
				break
			}
		}
		result = append(result, meldInfo{Tiles: sorted, IsTriplet: isTriplet})
	}
	return result, nil
}

func canHu(counts [tileKinds]int, openMeldCount int, missingSuit int) bool {
	if openMeldCount < 0 || openMeldCount > 4 {
		return false
	}
	if missingSuit >= 0 {
		for i := 0; i < tileKinds; i++ {
			if suitOf(i) == missingSuit && counts[i] > 0 {
				return false
			}
		}
	}
	expectedTiles := 14 - openMeldCount*3
	if sumCounts(counts) != expectedTiles {
		return false
	}
	if openMeldCount == 0 && isSevenPairs(counts) {
		return true
	}
	requiredMelds := 4 - openMeldCount
	for i := 0; i < tileKinds; i++ {
		if counts[i] < 2 {
			continue
		}
		next := counts
		next[i] -= 2
		memo := map[string]bool{}
		if canMakeMelds(next, requiredMelds, memo) {
			return true
		}
	}
	return false
}

func canMakeMelds(counts [tileKinds]int, required int, memo map[string]bool) bool {
	if required == 0 {
		for _, c := range counts {
			if c != 0 {
				return false
			}
		}
		return true
	}
	key := meldMemoKey(counts, required)
	if cached, ok := memo[key]; ok {
		return cached
	}
	idx := -1
	for i := 0; i < tileKinds; i++ {
		if counts[i] > 0 {
			idx = i
			break
		}
	}
	if idx == -1 {
		memo[key] = required == 0
		return required == 0
	}
	if counts[idx] >= 3 {
		next := counts
		next[idx] -= 3
		if canMakeMelds(next, required-1, memo) {
			memo[key] = true
			return true
		}
	}
	rank := idx % 9
	if rank <= 6 && suitOf(idx) == suitOf(idx+1) && suitOf(idx+1) == suitOf(idx+2) &&
		counts[idx+1] > 0 && counts[idx+2] > 0 {
		next := counts
		next[idx]--
		next[idx+1]--
		next[idx+2]--
		if canMakeMelds(next, required-1, memo) {
			memo[key] = true
			return true
		}
	}
	memo[key] = false
	return false
}

func meldMemoKey(counts [tileKinds]int, required int) string {
	var b strings.Builder
	b.Grow(tileKinds + 2)
	for _, c := range counts {
		b.WriteByte(byte(c + '0'))
	}
	b.WriteByte('|')
	b.WriteByte(byte(required + '0'))
	return b.String()
}

func isSevenPairs(counts [tileKinds]int) bool {
	if sumCounts(counts) != 14 {
		return false
	}
	pairCount := 0
	for _, c := range counts {
		if c == 0 {
			continue
		}
		if c == 2 {
			pairCount++
			continue
		}
		if c == 4 {
			pairCount += 2
			continue
		}
		return false
	}
	return pairCount == 7
}

func hasQuad(counts [tileKinds]int) bool {
	for _, c := range counts {
		if c == 4 {
			return true
		}
	}
	return false
}

func isQingYiSe(counts [tileKinds]int, melds []meldInfo) bool {
	suit := -1
	for i, c := range counts {
		if c == 0 {
			continue
		}
		if suit == -1 {
			suit = suitOf(i)
			continue
		}
		if suit != suitOf(i) {
			return false
		}
	}
	for _, m := range melds {
		for _, tile := range m.Tiles {
			if suit == -1 {
				suit = suitOf(tile)
				continue
			}
			if suit != suitOf(tile) {
				return false
			}
		}
	}
	return suit != -1
}

func isDuiDuiHu(counts [tileKinds]int, melds []meldInfo) bool {
	for _, m := range melds {
		if !m.IsTriplet {
			return false
		}
	}
	pairFound := false
	for _, c := range counts {
		if c == 0 {
			continue
		}
		mod := c % 3
		if mod == 0 {
			continue
		}
		if mod == 2 && !pairFound {
			pairFound = true
			continue
		}
		return false
	}
	return pairFound
}

func calculateFan(counts [tileKinds]int, melds []meldInfo, missingSuit int) models.FanResult {
	if !canHu(counts, len(melds), missingSuit) {
		return models.FanResult{}
	}
	if len(melds) == 0 && isSevenPairs(counts) {
		total := 4
		fans := []models.FanItem{{Name: "QiDui", Fan: 4}}
		if hasQuad(counts) {
			total = 5
			fans = []models.FanItem{{Name: "LongQiDui", Fan: 5}}
		}
		if isQingYiSe(counts, melds) {
			total += 2
			fans = append(fans, models.FanItem{Name: "QingYiSe", Fan: 2})
		}
		return models.FanResult{TotalFan: total, Fans: fans}
	}

	total := 1
	fans := []models.FanItem{{Name: "PingHu", Fan: 1}}
	if isDuiDuiHu(counts, melds) {
		total += 1
		fans = append(fans, models.FanItem{Name: "DuiDuiHu", Fan: 1})
	}
	if isQingYiSe(counts, melds) {
		total += 2
		fans = append(fans, models.FanItem{Name: "QingYiSe", Fan: 2})
	}
	return models.FanResult{TotalFan: total, Fans: fans}
}

func tingTiles(counts [tileKinds]int, remaining [tileKinds]int, meldCount int, missingSuit int) []int {
	outs := make([]int, 0)
	for i := 0; i < tileKinds; i++ {
		if remaining[i] <= 0 {
			continue
		}
		next := counts
		next[i]++
		if canHu(next, meldCount, missingSuit) {
			outs = append(outs, i)
		}
	}
	return outs
}
