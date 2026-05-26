package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

var (
	chatterMu      sync.RWMutex
	activeChatters = make(map[string]bool)
)

func trackChatter(username string) {
	chatterMu.Lock()
	activeChatters[username] = true
	chatterMu.Unlock()
}

func handleGive(parts []string) string {
	if len(parts) < 2 {
		return "Uso: !dar <cantidad> | !dar <usuario> <cantidad>"
	}

	// !dar <amount> → todos los chatters activos
	if amount, err := strconv.Atoi(parts[1]); err == nil {
		if amount <= 0 {
			return "La cantidad debe ser mayor a 0."
		}
		chatterMu.RLock()
		chatters := make([]string, 0, len(activeChatters))
		for u := range activeChatters {
			chatters = append(chatters, u)
		}
		chatterMu.RUnlock()
		if len(chatters) == 0 {
			return "No hay chatters activos registrados aún."
		}
		rewards := make(map[string]int, len(chatters))
		for _, u := range chatters {
			rewards[u] = amount
		}
		bulkAddPoints(rewards)
		return fmt.Sprintf("☕ +%d lattepoints para los %d del chat!", amount, len(chatters))
	}

	// !dar <user> <amount> → usuario individual
	if len(parts) < 3 {
		return "Uso: !dar <cantidad> | !dar <usuario> <cantidad>"
	}
	username := strings.ToLower(strings.TrimPrefix(parts[1], "@"))
	amount, err := strconv.Atoi(parts[2])
	if err != nil || amount <= 0 {
		return "La cantidad debe ser un número mayor a 0."
	}
	addPoints(username, amount)
	return fmt.Sprintf("☕ @%s recibió +%d lattepoints!", username, amount)
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
