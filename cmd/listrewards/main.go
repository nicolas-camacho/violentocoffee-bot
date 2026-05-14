package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type usersResponse struct {
	Data []struct {
		ID    string `json:"id"`
		Login string `json:"login"`
	} `json:"data"`
}

type rewardsResponse struct {
	Data []struct {
		ID                  string `json:"id"`
		Title               string `json:"title"`
		Cost                int    `json:"cost"`
		IsEnabled           bool   `json:"is_enabled"`
		IsUserInputRequired bool   `json:"is_user_input_required"`
	} `json:"data"`
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, using system env vars")
	}

	clientID := os.Getenv("TWITCH_CLIENT_ID")
	token := strings.TrimPrefix(os.Getenv("TWITCH_OAUTH_TOKEN"), "oauth:")
	channel := os.Getenv("TWITCH_CHANNEL")

	if clientID == "" || token == "" || channel == "" {
		log.Fatal("missing env vars: TWITCH_CLIENT_ID, TWITCH_OAUTH_TOKEN, TWITCH_CHANNEL")
	}

	broadcasterID, err := getBroadcasterID(clientID, token, channel)
	if err != nil {
		log.Fatalf("error obteniendo broadcaster ID: %v", err)
	}

	rewards, err := getCustomRewards(clientID, token, broadcasterID)
	if err != nil {
		log.Fatalf("error obteniendo rewards: %v", err)
	}

	fmt.Printf("Custom rewards de #%s (%d encontrados):\n\n", channel, len(rewards.Data))
	for _, r := range rewards.Data {
		status := "✅ activo"
		if !r.IsEnabled {
			status = "❌ inactivo"
		}
		inputRequired := ""
		if r.IsUserInputRequired {
			inputRequired = " [requiere texto]"
		}
		fmt.Printf("  ID:    %s\n", r.ID)
		fmt.Printf("  Nombre: %s\n", r.Title)
		fmt.Printf("  Costo:  %d puntos\n", r.Cost)
		fmt.Printf("  Estado: %s%s\n", status, inputRequired)
		fmt.Println()
	}
}

func getBroadcasterID(clientID, token, channel string) (string, error) {
	req, err := http.NewRequest("GET", "https://api.twitch.tv/helix/users?login="+channel, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var users usersResponse
	if err := json.Unmarshal(body, &users); err != nil {
		return "", err
	}
	if len(users.Data) == 0 {
		return "", fmt.Errorf("canal '%s' no encontrado", channel)
	}
	return users.Data[0].ID, nil
}

func getCustomRewards(clientID, token, broadcasterID string) (*rewardsResponse, error) {
	req, err := http.NewRequest("GET", "https://api.twitch.tv/helix/channel_points/custom_rewards?broadcaster_id="+broadcasterID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var rewards rewardsResponse
	if err := json.Unmarshal(body, &rewards); err != nil {
		return nil, err
	}
	return &rewards, nil
}
