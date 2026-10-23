package main

import "math"

// Reyting: Elo, jamoaviy. Har bir o'yinchi o'z jamoasining o'rtacha reytingi
// raqib jamoaning o'rtacha reytingiga qarshi hisoblanadi. Natija faqat
// g'alaba yoki mag'lubiyatdan kelib chiqadi (CS Premier kabi); hisobdagi
// farq o'zgarishni biroz kattalashtiradi (8:0 — 1.5 baravar, 8:7 — deyarli 1).

const (
	startRating      = 1000
	placementMatches = 5  // shuncha o'yindan keyin daraja ochiladi
	kPlacement       = 40 // dastlabki o'yinlarda reyting tezroq o'zgaradi
	kNormal          = 25
)

type rankInfo struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Min         int    `json:"min"`
	Calibrating bool   `json:"calibrating"`
	Played      int    `json:"played"`
	Need        int    `json:"need"`
}

// Darajalar: o'yinning o'z nomlari, reyting chegarasi bo'yicha.
var rankTiers = []rankInfo{
	{Name: "Oddiy askar", Color: "#9aa3ad", Min: 0},
	{Name: "Askar", Color: "#b9c6d2", Min: 800},
	{Name: "Kichik serjant", Color: "#7fbbe9", Min: 950},
	{Name: "Serjant", Color: "#4f93e6", Min: 1100},
	{Name: "Katta serjant", Color: "#7b70f2", Min: 1250},
	{Name: "Leytenant", Color: "#a862ea", Min: 1400},
	{Name: "Kapitan", Color: "#d75fcb", Min: 1550},
	{Name: "Mayor", Color: "#ea5a7d", Min: 1700},
	{Name: "Podpolkovnik", Color: "#ee6d3e", Min: 1850},
	{Name: "Polkovnik", Color: "#f3a43b", Min: 2000},
	{Name: "General", Color: "#f6d343", Min: 2200},
}

func rankOf(rating, matches int) rankInfo {
	if matches < placementMatches {
		return rankInfo{Name: "Kalibrlash", Color: "#8d8778", Calibrating: true, Played: matches, Need: placementMatches}
	}
	r := rankTiers[0]
	for _, t := range rankTiers {
		if rating >= t.Min {
			r = t
		}
	}
	r.Played, r.Need = matches, placementMatches
	return r
}

func expected(own, opp float64) float64 { return 1 / (1 + math.Pow(10, (opp-own)/400)) }

// eloDelta: o'yinchi reytingining o'zgarishi. own/opp — jamoalarning o'rtacha reytingi.
func eloDelta(own, opp float64, won bool, matchesBefore, scoreDiff int) int {
	k := float64(kNormal)
	if matchesBefore < 10 {
		k = kPlacement
	}
	margin := 1 + 0.5*math.Min(1, math.Abs(float64(scoreDiff))/8)
	s := 0.0
	if won {
		s = 1
	}
	d := int(math.Round(k * margin * (s - expected(own, opp))))
	if won && d < 1 {
		d = 1
	}
	if !won && d > -1 {
		d = -1
	}
	return d
}
