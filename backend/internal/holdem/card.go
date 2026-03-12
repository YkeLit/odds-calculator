package holdem

import (
	"fmt"
	"strconv"
	"strings"
)

type Card struct {
	Rank int
	Suit byte
	Code string
}

var rankMap = map[string]int{
	"2": 2,
	"3": 3,
	"4": 4,
	"5": 5,
	"6": 6,
	"7": 7,
	"8": 8,
	"9": 9,
	"T": 10,
	"J": 11,
	"Q": 12,
	"K": 13,
	"A": 14,
}

var rankToCode = map[int]string{
	2:  "2",
	3:  "3",
	4:  "4",
	5:  "5",
	6:  "6",
	7:  "7",
	8:  "8",
	9:  "9",
	10: "T",
	11: "J",
	12: "Q",
	13: "K",
	14: "A",
}

func ParseCard(code string) (Card, error) {
	trimmed := strings.TrimSpace(code)
	if len(trimmed) < 2 || len(trimmed) > 3 {
		return Card{}, fmt.Errorf("invalid card %q", code)
	}
	upper := strings.ToUpper(trimmed)
	rankPart := upper[:len(upper)-1]
	suitPart := strings.ToLower(upper[len(upper)-1:])
	if rankPart == "10" {
		rankPart = "T"
	}
	rank, ok := rankMap[rankPart]
	if !ok {
		if n, err := strconv.Atoi(rankPart); err == nil {
			rank = n
		}
	}
	if rank < 2 || rank > 14 {
		return Card{}, fmt.Errorf("invalid card rank %q", rankPart)
	}
	suit := suitPart[0]
	if suit != 's' && suit != 'h' && suit != 'd' && suit != 'c' {
		return Card{}, fmt.Errorf("invalid card suit %q", suitPart)
	}
	return Card{Rank: rank, Suit: suit, Code: rankToCode[rank] + string(suit)}, nil
}

func FullDeck() []Card {
	deck := make([]Card, 0, 52)
	suits := []byte{'s', 'h', 'd', 'c'}
	for _, suit := range suits {
		for rank := 2; rank <= 14; rank++ {
			deck = append(deck, Card{Rank: rank, Suit: suit, Code: rankToCode[rank] + string(suit)})
		}
	}
	return deck
}
