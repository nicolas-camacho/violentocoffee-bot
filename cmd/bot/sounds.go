package main

import (
	"log"
	"os"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
)

const speakerSampleRate = beep.SampleRate(44100)

func initSpeaker() {
	if err := speaker.Init(speakerSampleRate, speakerSampleRate.N(time.Second/10)); err != nil {
		log.Printf("[audio] error inicializando speaker: %v", err)
	} else {
		log.Println("[audio] speaker inicializado")
	}
}

func playAudio(path string) {
	go func() {
		f, err := os.Open(path)
		if err != nil {
			log.Printf("[audio] error abriendo %s: %v", path, err)
			return
		}
		defer f.Close()

		stream, format, err := mp3.Decode(f)
		if err != nil {
			log.Printf("[audio] error decodificando %s: %v", path, err)
			return
		}
		defer stream.Close()

		var s beep.Streamer = stream
		if format.SampleRate != speakerSampleRate {
			s = beep.Resample(3, format.SampleRate, speakerSampleRate, stream)
		}

		done := make(chan struct{})
		speaker.Play(beep.Seq(s, beep.Callback(func() {
			close(done)
		})))

		log.Printf("[audio] reproduciendo %s", path)
		<-done
		log.Printf("[audio] terminado %s", path)
	}()
}
