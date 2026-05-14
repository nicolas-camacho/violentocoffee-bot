package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var spotifyHTTPClient = &http.Client{Timeout: 5 * time.Second}

type spotifyTokenResponse struct {
	AccessToken string `json:"access_token"`
}

type spotifyCurrentlyPlaying struct {
	IsPlaying bool `json:"is_playing"`
	Item      *struct {
		Name    string `json:"name"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
	} `json:"item"`
}

func getSpotifyAccessToken() (string, error) {
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	refreshToken := os.Getenv("SPOTIFY_REFRESH_TOKEN")

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := spotifyHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var tokenResp spotifyTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", err
	}
	return tokenResp.AccessToken, nil
}

func handleSong() string {
	accessToken, err := getSpotifyAccessToken()
	if err != nil {
		return "No se pudo conectar a Spotify FeelsBadMan"
	}

	req, err := http.NewRequest("GET", "https://api.spotify.com/v1/me/player/currently-playing", nil)
	if err != nil {
		return "Error al consultar Spotify FeelsBadMan"
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := spotifyHTTPClient.Do(req)
	if err != nil {
		return "Error al consultar Spotify FeelsBadMan"
	}
	defer resp.Body.Close()

	if resp.StatusCode == 204 {
		return "No hay ninguna canción reproduciéndose ahora mismo."
	}

	body, _ := io.ReadAll(resp.Body)
	var playing spotifyCurrentlyPlaying
	if err := json.Unmarshal(body, &playing); err != nil {
		return "Error al leer respuesta de Spotify FeelsBadMan"
	}

	if !playing.IsPlaying || playing.Item == nil {
		return "No hay ninguna canción reproduciéndose ahora mismo."
	}

	artists := make([]string, len(playing.Item.Artists))
	for i, a := range playing.Item.Artists {
		artists[i] = a.Name
	}

	return fmt.Sprintf("🎵 %s — %s", playing.Item.Name, strings.Join(artists, ", "))
}
