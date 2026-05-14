package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	twitch "github.com/gempir/go-twitch-irc/v4"
	"github.com/joho/godotenv"
)

var broadcasterName string

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, using system env vars")
	}

	botUsername := os.Getenv("TWITCH_BOT_USERNAME")
	oauthToken := os.Getenv("TWITCH_OAUTH_TOKEN")
	channel := os.Getenv("TWITCH_CHANNEL")

	if botUsername == "" || oauthToken == "" || channel == "" {
		log.Fatal("missing env vars: TWITCH_BOT_USERNAME, TWITCH_OAUTH_TOKEN, TWITCH_CHANNEL")
	}
	broadcasterName = channel

	initDB()
	initSpeaker()
	loadAudioRewards()
	startOverlayServer()
	broadcastOverlay(OverlayState{State: "idle"})

	client := twitch.NewClient(botUsername, oauthToken)

	clientID := os.Getenv("TWITCH_CLIENT_ID")
	broadcasterToken := os.Getenv("TWITCH_BROADCASTER_TOKEN")
	if broadcasterToken == "" {
		broadcasterToken = strings.TrimPrefix(oauthToken, "oauth:")
	}
	if clientID == "" || broadcasterToken == "" {
		log.Println("[eventsub] TWITCH_CLIENT_ID o TWITCH_BROADCASTER_TOKEN no configurados — rewards desactivados")
	} else {
		broadcasterID, err := fetchBroadcasterID(clientID, broadcasterToken, channel)
		if err != nil {
			log.Printf("[eventsub] no se pudo obtener broadcaster ID: %v", err)
		} else {
			log.Printf("[eventsub] broadcaster ID: %s", broadcasterID)
			startEventSub(clientID, broadcasterToken, broadcasterID,
				func(rewardID, username string) {
					if audioFile, ok := audioRewards[rewardID]; ok {
						log.Printf("[reward] match → reproduciendo %s para %s", audioFile, username)
						playAudio(audioFile)
					} else {
						log.Printf("[reward] id=%s user=%s sin match en rewards.json", rewardID, username)
					}
				},
				func(username string) {
					const followBonus = 100
					addPoints(username, followBonus)
					client.Say(channel, fmt.Sprintf("¡Bienvenid@ @%s! Gracias por el follow ☕ +%d lattepoints de regalo", username, followBonus))
				},
			)
		}
	}

	startRPGEventLoop(func(m string) { client.Say(channel, m) })

	client.OnPrivateMessage(func(msg twitch.PrivateMessage) {
		text := strings.TrimSpace(msg.Message)
		if !strings.HasPrefix(text, "!") {
			return
		}

		parts := strings.Fields(text)
		command := strings.ToLower(parts[0])

		switch command {
		case "!song":
			response := handleSong()
			client.Say(channel, response)
		case "!setup":
			response := handleSetup()
			client.Say(channel, response)
		case "!social":
			response := handleSocial()
			client.Say(channel, response)
		case "!wordle":
			handleWordle(client, channel, msg)
		case "!help":
			handleHelp(client, channel)
		case "!3ds":
			response := handleSend3DS(parts, msg.User.Name)
			client.Say(channel, response)
		case "!points", "!lattepoints":
			client.Say(channel, handlePoints(msg.User.Name))
		case "!sound":
			client.Say(channel, handleSound(parts, msg.User.Name))
		case "!ovtest":
			if msg.User.Name == channel {
				handleOverlayTest(parts)
			}
		case "!evento":
			if msg.User.Name == channel {
				handleManualEvent(parts)
			}
		case "!atacar", "!attack":
			handleAttack(msg.User.Name)
		case "!dungeon", "!mazmorra":
			handleJoinDungeon(msg.User.Name)
		case "!cofre", "!abrir":
			handleOpenChest(msg.User.Name)
		case "!tienda", "!shop":
			client.Say(channel, handleShop())
		case "!comprar", "!buy":
			client.Say(channel, handleBuy(parts, msg.User.Name))
		case "!equipar", "!equip":
			client.Say(channel, handleEquip(parts, msg.User.Name))
		case "!inventario", "!inv":
			client.Say(channel, handleInventory(msg.User.Name))
		}
	})

	client.Join(channel)

	fmt.Printf("Bot connected. Listening on #%s\n", channel)
	if err := client.Connect(); err != nil {
		log.Fatal(err)
	}
}
