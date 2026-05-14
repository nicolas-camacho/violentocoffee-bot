package main

import (
	"encoding/json"
	"log"
	"os"
)

// maps Twitch reward ID -> audio file path
var audioRewards map[string]string

func loadAudioRewards() {
	audioRewards = make(map[string]string)

	data, err := os.ReadFile("rewards.json")
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("error loading rewards.json: %v", err)
		}
		return
	}

	if err := json.Unmarshal(data, &audioRewards); err != nil {
		log.Printf("error parsing rewards.json: %v", err)
		return
	}

	log.Printf("loaded %d audio rewards from rewards.json", len(audioRewards))
}
