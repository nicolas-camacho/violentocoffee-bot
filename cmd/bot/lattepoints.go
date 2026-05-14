package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	lattepointsFile  = "lattepoints.json"
	soundPricesFile  = "sound_prices.json"
	wordleWinPoints  = 500
	defaultSoundCost = 25
	audiosDir        = "assets/audios"
)

func loadSoundPrices() map[string]int {
	data, err := os.ReadFile(soundPricesFile)
	if err != nil {
		return make(map[string]int)
	}
	var prices map[string]int
	if err := json.Unmarshal(data, &prices); err != nil {
		return make(map[string]int)
	}
	return prices
}

func soundPrice(name string) int {
	prices := loadSoundPrices()
	if cost, ok := prices[name]; ok {
		return cost
	}
	return defaultSoundCost
}

func handlePoints(username string) string {
	pts := getPoints(username)
	return fmt.Sprintf("@%s tiene %d lattepoints ☕", username, pts)
}

func handleSound(parts []string, username string) string {
	entries, err := os.ReadDir(audiosDir)
	if err != nil {
		return "[sound] carpeta de audios no encontrada"
	}

	available := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.TrimSuffix(strings.ToLower(e.Name()), filepath.Ext(e.Name()))
		available[name] = filepath.Join(audiosDir, e.Name())
	}

	if len(parts) < 2 {
		entries2 := make([]string, 0, len(available))
		for n := range available {
			entries2 = append(entries2, fmt.Sprintf("%s (%dpts)", n, soundPrice(n)))
		}
		return "Sonidos: " + strings.Join(entries2, " | ")
	}

	target := strings.ToLower(parts[1])
	path, ok := available[target]
	if !ok {
		return fmt.Sprintf("@%s sonido \"%s\" no existe. Usa !sound para ver disponibles.", username, parts[1])
	}

	cost := soundPrice(target)
	if !spendPoints(username, cost) {
		return fmt.Sprintf("@%s no tienes suficientes lattepoints (necesitas %d, tienes %d) ☕", username, cost, getPoints(username))
	}

	playAudio(path)
	return fmt.Sprintf("@%s gastó %d lattepoints → 🔊 %s", username, cost, parts[1])
}
