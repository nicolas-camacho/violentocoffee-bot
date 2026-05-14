package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	redirectURI = "http://127.0.0.1:8888/callback"
	scope       = "user-read-currently-playing"
)

func main() {
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		log.Fatal("missing SPOTIFY_CLIENT_ID or SPOTIFY_CLIENT_SECRET")
	}

	authURL := fmt.Sprintf(
		"https://accounts.spotify.com/authorize?client_id=%s&response_type=code&redirect_uri=%s&scope=%s",
		clientID,
		url.QueryEscape(redirectURI),
		url.QueryEscape(scope),
	)

	fmt.Println("Abre este URL en tu browser:")
	fmt.Println()
	fmt.Println(authURL)
	fmt.Println()
	fmt.Println("Esperando callback en http://127.0.0.1:8888/callback ...")

	codeCh := make(chan string, 1)

	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "no code in callback", http.StatusBadRequest)
			return
		}
		fmt.Fprintln(w, "Autorizado. Puedes cerrar esta pestaña.")
		codeCh <- code
	})

	go func() {
		if err := http.ListenAndServe("127.0.0.1:8888", nil); err != nil {
			log.Fatal(err)
		}
	}()

	code := <-codeCh

	// exchange code for tokens
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		log.Fatal(err)
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		log.Fatal(err)
	}

	refreshToken, ok := result["refresh_token"].(string)
	if !ok {
		log.Fatalf("no refresh_token in response: %s", string(body))
	}

	fmt.Println()
	fmt.Println("✅ SPOTIFY_REFRESH_TOKEN obtenido:")
	fmt.Println()
	fmt.Println(refreshToken)
	fmt.Println()
	fmt.Println("Agrégalo a tus variables de entorno.")
}
