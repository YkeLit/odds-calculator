package mahjong

import (
	"fmt"
	"math"
	"sort"
	"time"

	"odds-calculator/backend/internal/models"
)

func Analyze(req models.MahjongAnalyzeRequest) (models.MahjongAnalyzeResponse, error) {
	started := time.Now()
	if len(req.HandTiles) != 13 && len(req.HandTiles) != 14 {
		return models.MahjongAnalyzeResponse{}, fmt.Errorf("handTiles must be 13 or 14 tiles")
	}
	missingSuit, err := parseSuit(req.MissingSuit)
	if err != nil {
		return models.MahjongAnalyzeResponse{}, err
	}
	melds, err := parseMelds(req.Melds)
	if err != nil {
		return models.MahjongAnalyzeResponse{}, err
	}

	var handCounts [tileKinds]int
	if err := addTiles(&handCounts, req.HandTiles); err != nil {
		return models.MahjongAnalyzeResponse{}, err
	}
	var visibleCounts [tileKinds]int
	if err := addTiles(&visibleCounts, req.VisibleTiles); err != nil {
		return models.MahjongAnalyzeResponse{}, err
	}
	var meldCounts [tileKinds]int
	for _, m := range melds {
		for _, tile := range m.Tiles {
			meldCounts[tile]++
		}
	}

	var remaining [tileKinds]int
	for i := 0; i < tileKinds; i++ {
		used := handCounts[i] + visibleCounts[i] + meldCounts[i]
		left := 4 - used
		if left < 0 {
			return models.MahjongAnalyzeResponse{}, fmt.Errorf("tile %s over-used (%d)", tileString(i), used)
		}
		remaining[i] = left
	}

	resp := models.MahjongAnalyzeResponse{}
	if len(req.HandTiles) == 13 {
		outs := tingTiles(handCounts, remaining, len(melds), missingSuit)
		resp.IsTing = len(outs) > 0
		resp.WinningTiles = stringifyTiles(outs)
		huProb, _, _ := evaluateTwoDraw(handCounts, remaining, melds, missingSuit)
		resp.NextTwoDrawHuProb = round4(huProb)
		resp.ElapsedMs = time.Since(started).Milliseconds()
		return resp, nil
	}

	recommendations := make([]models.DiscardRecommendation, 0)
	seenDiscard := map[int]struct{}{}
	for tile := 0; tile < tileKinds; tile++ {
		if handCounts[tile] == 0 {
			continue
		}
		if _, ok := seenDiscard[tile]; ok {
			continue
		}
		seenDiscard[tile] = struct{}{}

		afterDiscard := handCounts
		afterDiscard[tile]--
		huProb, expectedFan, fanContribution := evaluateTwoDraw(afterDiscard, remaining, melds, missingSuit)
		recommendations = append(recommendations, models.DiscardRecommendation{
			Tile:            tileString(tile),
			ExpectedFan:     round4(expectedFan),
			HuProb:          round4(huProb),
			TopFanBreakdown: topContributions(fanContribution, 5),
		})
	}

	sort.Slice(recommendations, func(i, j int) bool {
		if recommendations[i].ExpectedFan != recommendations[j].ExpectedFan {
			return recommendations[i].ExpectedFan > recommendations[j].ExpectedFan
		}
		if recommendations[i].HuProb != recommendations[j].HuProb {
			return recommendations[i].HuProb > recommendations[j].HuProb
		}
		return recommendations[i].Tile < recommendations[j].Tile
	})

	resp.DiscardRecommendations = recommendations
	if len(recommendations) > 0 {
		resp.NextTwoDrawHuProb = recommendations[0].HuProb
		bestDiscardIdx, _ := parseTile(recommendations[0].Tile)
		afterDiscard := handCounts
		afterDiscard[bestDiscardIdx]--
		outs := tingTiles(afterDiscard, remaining, len(melds), missingSuit)
		resp.WinningTiles = stringifyTiles(outs)
		resp.IsTing = len(outs) > 0
	}
	resp.ElapsedMs = time.Since(started).Milliseconds()
	return resp, nil
}

func evaluateTwoDraw(counts13 [tileKinds]int, remaining [tileKinds]int, melds []meldInfo, missingSuit int) (float64, float64, map[string]float64) {
	totalFirst := sumCounts(remaining)
	if totalFirst <= 0 {
		return 0, 0, map[string]float64{}
	}
	hitProb := 0.0
	expectedFan := 0.0
	fanContribution := map[string]float64{}
	for draw := 0; draw < tileKinds; draw++ {
		if remaining[draw] <= 0 {
			continue
		}
		p1 := float64(remaining[draw]) / float64(totalFirst)
		afterDraw := counts13
		afterDraw[draw]++
		nextRemaining := remaining
		nextRemaining[draw]--

		if canHu(afterDraw, len(melds), missingSuit) {
			fan := calculateFan(afterDraw, melds, missingSuit)
			hitProb += p1
			expectedFan += p1 * float64(fan.TotalFan)
			for _, item := range fan.Fans {
				fanContribution[item.Name] += p1 * float64(item.Fan)
			}
			continue
		}
		secondFan, secondProb, secondContrib := bestSecondDraw(afterDraw, nextRemaining, melds, missingSuit)
		expectedFan += p1 * secondFan
		hitProb += p1 * secondProb
		for k, v := range secondContrib {
			fanContribution[k] += p1 * v
		}
	}
	return hitProb, expectedFan, fanContribution
}

func bestSecondDraw(counts14 [tileKinds]int, remaining [tileKinds]int, melds []meldInfo, missingSuit int) (float64, float64, map[string]float64) {
	totalSecond := sumCounts(remaining)
	if totalSecond <= 0 {
		return 0, 0, map[string]float64{}
	}
	bestFan := -1.0
	bestProb := -1.0
	bestContribution := map[string]float64{}
	seenDiscard := map[int]struct{}{}
	for discard := 0; discard < tileKinds; discard++ {
		if counts14[discard] == 0 {
			continue
		}
		if _, ok := seenDiscard[discard]; ok {
			continue
		}
		seenDiscard[discard] = struct{}{}
		afterDiscard := counts14
		afterDiscard[discard]--

		fanExpected := 0.0
		hitProb := 0.0
		contribution := map[string]float64{}
		for draw := 0; draw < tileKinds; draw++ {
			if remaining[draw] <= 0 {
				continue
			}
			p2 := float64(remaining[draw]) / float64(totalSecond)
			afterSecond := afterDiscard
			afterSecond[draw]++
			if !canHu(afterSecond, len(melds), missingSuit) {
				continue
			}
			fan := calculateFan(afterSecond, melds, missingSuit)
			hitProb += p2
			fanExpected += p2 * float64(fan.TotalFan)
			for _, item := range fan.Fans {
				contribution[item.Name] += p2 * float64(item.Fan)
			}
		}
		if fanExpected > bestFan || (fanExpected == bestFan && hitProb > bestProb) {
			bestFan = fanExpected
			bestProb = hitProb
			bestContribution = contribution
		}
	}
	if bestFan < 0 {
		return 0, 0, map[string]float64{}
	}
	return bestFan, bestProb, bestContribution
}

func stringifyTiles(tiles []int) []string {
	result := make([]string, 0, len(tiles))
	for _, tile := range tiles {
		result = append(result, tileString(tile))
	}
	sort.Strings(result)
	return result
}

func topContributions(contrib map[string]float64, limit int) []models.FanContribution {
	list := make([]models.FanContribution, 0, len(contrib))
	for name, value := range contrib {
		if value <= 0 {
			continue
		}
		list = append(list, models.FanContribution{Fan: name, Contribution: round4(value)})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Contribution != list[j].Contribution {
			return list[i].Contribution > list[j].Contribution
		}
		return list[i].Fan < list[j].Fan
	})
	if len(list) > limit {
		return list[:limit]
	}
	return list
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
