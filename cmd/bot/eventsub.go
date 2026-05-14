package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const eventsubWSURL = "wss://eventsub.wss.twitch.tv/ws"

type eventsubMsg struct {
	Metadata struct {
		MessageType string `json:"message_type"`
	} `json:"metadata"`
	Payload json.RawMessage `json:"payload"`
}

func fetchBroadcasterID(clientID, token, channel string) (string, error) {
	req, _ := http.NewRequest("GET", "https://api.twitch.tv/helix/users?login="+channel, nil)
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

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Data) == 0 {
		return "", fmt.Errorf("canal '%s' no encontrado", channel)
	}
	return result.Data[0].ID, nil
}

func startEventSub(clientID, token, broadcasterID string, onRedemption func(rewardID, username string), onFollow func(username string)) {
	go func() {
		for {
			if err := runEventSub(clientID, token, broadcasterID, onRedemption, onFollow); err != nil {
				log.Printf("[eventsub] desconectado: %v — reconectando en 5s", err)
				time.Sleep(5 * time.Second)
			}
		}
	}()
}

func runEventSub(clientID, token, broadcasterID string, onRedemption func(rewardID, username string), onFollow func(username string)) error {
	conn, _, err := websocket.DefaultDialer.Dial(eventsubWSURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	log.Println("[eventsub] conectado")

	for {
		var msg eventsubMsg
		if err := conn.ReadJSON(&msg); err != nil {
			return err
		}

		switch msg.Metadata.MessageType {
		case "session_welcome":
			var payload struct {
				Session struct {
					ID string `json:"id"`
				} `json:"session"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				return fmt.Errorf("error parseando session_welcome: %w", err)
			}
			log.Printf("[eventsub] session_id=%s", payload.Session.ID)
			if err := subscribeChannelPoints(clientID, token, broadcasterID, payload.Session.ID); err != nil {
				return fmt.Errorf("error suscribiendo channel points: %w", err)
			}
			if err := subscribeFollow(clientID, token, broadcasterID, payload.Session.ID); err != nil {
				log.Printf("[eventsub] follow subscription falló (token sin scope moderator:read:followers?): %v", err)
			}
			log.Println("[eventsub] suscripción exitosa — escuchando rewards y follows")

		case "session_keepalive":
			// heartbeat, no action needed

		case "session_reconnect":
			return fmt.Errorf("servidor solicitó reconexión")

		case "notification":
			var payload struct {
				Subscription struct {
					Type string `json:"type"`
				} `json:"subscription"`
				Event struct {
					UserName  string `json:"user_name"`
					UserLogin string `json:"user_login"`
					Reward    struct {
						ID    string `json:"id"`
						Title string `json:"title"`
					} `json:"reward"`
				} `json:"event"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				log.Printf("[eventsub] error parseando notificación: %v", err)
				continue
			}
			e := payload.Event
			switch payload.Subscription.Type {
			case "channel.channel_points_custom_reward_redemption.add":
				log.Printf("[eventsub] reward: id=%s title=%q user=%s", e.Reward.ID, e.Reward.Title, e.UserName)
				onRedemption(e.Reward.ID, e.UserName)
			case "channel.follow":
				log.Printf("[eventsub] nuevo seguidor: %s", e.UserLogin)
				if onFollow != nil {
					onFollow(e.UserLogin)
				}
			}

		case "revocation":
			log.Println("[eventsub] suscripción revocada — revisa el token")
		}
	}
}

func subscribeFollow(clientID, token, broadcasterID, sessionID string) error {
	body := map[string]any{
		"type":    "channel.follow",
		"version": "2",
		"condition": map[string]string{
			"broadcaster_user_id": broadcasterID,
			"moderator_user_id":   broadcasterID,
		},
		"transport": map[string]string{
			"method":     "websocket",
			"session_id": sessionID,
		},
	}
	data, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "https://api.twitch.tv/helix/eventsub/subscriptions", bytes.NewReader(data))
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 202 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func subscribeChannelPoints(clientID, token, broadcasterID, sessionID string) error {
	body := map[string]any{
		"type":    "channel.channel_points_custom_reward_redemption.add",
		"version": "1",
		"condition": map[string]string{
			"broadcaster_user_id": broadcasterID,
		},
		"transport": map[string]string{
			"method":     "websocket",
			"session_id": sessionID,
		},
	}
	data, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "https://api.twitch.tv/helix/eventsub/subscriptions", bytes.NewReader(data))
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 202 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
