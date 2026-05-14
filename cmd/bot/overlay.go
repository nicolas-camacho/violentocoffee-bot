package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/websocket"
)

type OverlayState struct {
	State   string       `json:"state"` // idle, boss, dungeon, chest
	Boss    *BossInfo    `json:"boss,omitempty"`
	Dungeon *DungeonInfo `json:"dungeon,omitempty"`
	Chest   *ChestInfo   `json:"chest,omitempty"`
	Top     []TopEntry   `json:"top,omitempty"`
}

type BossInfo struct {
	Name      string     `json:"name"`
	HP        int        `json:"hp"`
	MaxHP     int        `json:"maxHp"`
	Attackers []Attacker `json:"attackers"`
}

type Attacker struct {
	Username string `json:"username"`
	Damage   int    `json:"damage"`
}

type DungeonInfo struct {
	Name        string   `json:"name"`
	SecondsLeft int      `json:"secondsLeft"`
	Party       []string `json:"party"`
}

type ChestInfo struct {
	SecondsLeft int    `json:"secondsLeft"`
	Username    string `json:"username"`
	Result      string `json:"result"` // "", "treasure", "mimic"
	Amount      int    `json:"amount"`
}

type TopEntry struct {
	Username string `json:"username"`
	Points   int    `json:"points"`
}

var (
	overlayClients = make(map[*websocket.Conn]bool)
	overlayMu      sync.Mutex
	overlayUpgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	overlayCurrentState = OverlayState{State: "idle"}
)


func refreshLeaderboard() {
	if overlayCurrentState.State == "idle" {
		broadcastOverlay(OverlayState{State: "idle"})
	}
}

func broadcastOverlay(state OverlayState) {
	if state.State == "idle" {
		state.Top = topPoints(3)
	}
	overlayCurrentState = state
	data, _ := json.Marshal(state)
	overlayMu.Lock()
	defer overlayMu.Unlock()
	for conn := range overlayClients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			conn.Close()
			delete(overlayClients, conn)
		}
	}
}

func overlayWSHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := overlayUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[overlay] ws upgrade: %v", err)
		return
	}

	overlayMu.Lock()
	overlayClients[conn] = true
	overlayMu.Unlock()

	// Send current state immediately on connect
	state := overlayCurrentState
	if state.State == "idle" {
		state.Top = topPoints(3)
	}
	if data, err := json.Marshal(state); err == nil {
		conn.WriteMessage(websocket.TextMessage, data)
	}

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			overlayMu.Lock()
			delete(overlayClients, conn)
			overlayMu.Unlock()
			conn.Close()
			return
		}
	}
}

// handleOverlayTest lets the broadcaster preview each overlay state via !ovtest <state>
func handleOverlayTest(parts []string) {
	state := "idle"
	if len(parts) >= 2 {
		state = parts[1]
	}
	switch state {
	case "boss":
		broadcastOverlay(OverlayState{
			State: "boss",
			Boss: &BossInfo{
				Name:  "Goblin King",
				HP:    620,
				MaxHP: 1000,
				Attackers: []Attacker{
					{Username: "nellauw2", Damage: 230},
					{Username: "sailorkitty02", Damage: 150},
					{Username: "violentocoffee", Damage: 380},
				},
			},
		})
	case "dungeon":
		broadcastOverlay(OverlayState{
			State: "dungeon",
			Dungeon: &DungeonInfo{
				Name:        "Goblin Tunnels",
				SecondsLeft: 90,
				Party:       []string{"nellauw2", "sailorkitty02", "violentocoffee"},
			},
		})
	case "chest":
		broadcastOverlay(OverlayState{
			State: "chest",
			Chest: &ChestInfo{},
		})
	case "treasure":
		broadcastOverlay(OverlayState{
			State: "chest",
			Chest: &ChestInfo{Username: "nellauw2", Result: "treasure", Amount: 150},
		})
	case "mimic":
		broadcastOverlay(OverlayState{
			State: "chest",
			Chest: &ChestInfo{Username: "nellauw2", Result: "mimic", Amount: 75},
		})
	default:
		broadcastOverlay(OverlayState{State: "idle"})
	}
}

func startOverlayServer() {
	port := os.Getenv("OVERLAY_PORT")
	if port == "" {
		port = "7373"
	}

	mux := http.NewServeMux()
	mux.Handle("/font/", http.StripPrefix("/font/", http.FileServer(http.Dir("assets/fonts"))))
	mux.HandleFunc("/ws", overlayWSHandler)
	mux.Handle("/", http.FileServer(http.Dir("web")))

	log.Printf("[overlay] http://localhost:%s", port)
	go func() {
		if err := http.ListenAndServe(":"+port, mux); err != nil {
			log.Printf("[overlay] error: %v", err)
		}
	}()
}
