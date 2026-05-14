package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	twitch "github.com/gempir/go-twitch-irc/v4"
)

const wordleStateFile = "wordle.json"

type wordleState struct {
	CurrentWord    string    `json:"current_word"`
	Date           string    `json:"date"`
	Winner         string    `json:"winner"`
	WinnerAt       time.Time `json:"winner_at"`
	WinnerWord     string    `json:"winner_word"`
	LastWinner     string    `json:"last_winner"`
	LastWinnerAt   time.Time `json:"last_winner_at"`
	LastWinnerWord string    `json:"last_winner_word"`
}

func today() string {
	return time.Now().Format("2006-01-02")
}

func loadWordleState() wordleState {
	data, err := os.ReadFile(wordleStateFile)
	if err != nil {
		return wordleState{}
	}
	var s wordleState
	json.Unmarshal(data, &s)
	return s
}

func saveWordleState(s wordleState) {
	data, _ := json.MarshalIndent(s, "", "  ")
	os.WriteFile(wordleStateFile, data, 0644)
}

// resetIfNewWord resets the game when WORDLE_WORD changes, preserving last winner.
func resetIfNewWord(s *wordleState, currentWord string) {
	if normalizeWord(s.CurrentWord) != normalizeWord(currentWord) {
		if s.Winner != "" {
			s.LastWinner = s.Winner
			s.LastWinnerAt = s.WinnerAt
			s.LastWinnerWord = s.WinnerWord
		}
		s.CurrentWord = currentWord
		s.Date = today()
		s.Winner = ""
		s.WinnerAt = time.Time{}
		s.WinnerWord = ""
	}
}

func normalizeWord(w string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(w) {
		switch r {
		case 'Á':
			b.WriteRune('A')
		case 'É':
			b.WriteRune('E')
		case 'Í':
			b.WriteRune('I')
		case 'Ó':
			b.WriteRune('O')
		case 'Ú', 'Ü':
			b.WriteRune('U')
		default:
			if unicode.IsLetter(r) {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func evaluateWordle(word, guess string) string {
	wordRunes := []rune(word)
	guessRunes := []rune(guess)
	result := make([]string, len(guessRunes))
	wordUsed := make([]bool, len(wordRunes))
	guessUsed := make([]bool, len(guessRunes))

	// First pass: correct position (green)
	for i, g := range guessRunes {
		if i < len(wordRunes) && g == wordRunes[i] {
			result[i] = "🟩"
			wordUsed[i] = true
			guessUsed[i] = true
		}
	}

	// Second pass: wrong position (yellow) or not found (black)
	for i, g := range guessRunes {
		if guessUsed[i] {
			continue
		}
		found := false
		for j, w := range wordRunes {
			if !wordUsed[j] && g == w {
				result[i] = "🟨"
				wordUsed[j] = true
				found = true
				break
			}
		}
		if !found {
			result[i] = "⬛"
		}
	}

	return strings.Join(result, "")
}

func handleWordle(client *twitch.Client, channel string, msg twitch.PrivateMessage) {
	word := os.Getenv("WORDLE_WORD")
	if word == "" {
		client.Say(channel, "⚠️ No hay palabra del día configurada.")
		return
	}
	normalWord := normalizeWord(word)
	wordLen := len([]rune(normalWord))

	state := loadWordleState()
	resetIfNewWord(&state, word)
	saveWordleState(state)

	parts := strings.Fields(msg.Message)

	// !wordle sin argumento → info
	if len(parts) < 2 {
		if state.Winner != "" {
			client.Say(channel, fmt.Sprintf(
				"🎉 ¡Ya hay ganador hoy! @%s adivinó la palabra (%s) a las %s",
				state.Winner,
				state.WinnerWord,
				state.WinnerAt.Local().Format("15:04"),
			))
			return
		}

		info := fmt.Sprintf("📝 La palabra del día tiene %d letras.", wordLen)
		if state.LastWinner != "" {
			info += fmt.Sprintf(" Último ganador: @%s con \"%s\" el %s",
				state.LastWinner,
				state.LastWinnerWord,
				state.LastWinnerAt.Local().Format("02/01 15:04"),
			)
		} else {
			info += " Aún no hay ganadores anteriores."
		}
		client.Say(channel, info)
		return
	}

	// !wordle <intento>
	if state.Winner != "" {
		client.Say(channel, fmt.Sprintf(
			"🎉 ¡Ya hay ganador hoy! @%s adivinó la palabra. Era: %s",
			state.Winner,
			state.WinnerWord,
		))
		return
	}

	guess := normalizeWord(parts[1])
	guessLen := len([]rune(guess))

	if guessLen != wordLen {
		client.Say(channel, fmt.Sprintf(
			"@%s La palabra tiene %d letras, tu intento tiene %d. ¡Intenta de nuevo!",
			msg.User.Name, wordLen, guessLen,
		))
		return
	}

	squares := evaluateWordle(normalWord, guess)

	if guess == normalWord {
		state.Winner = msg.User.Name
		state.WinnerAt = time.Now()
		state.WinnerWord = word
		saveWordleState(state)
		addPoints(msg.User.Name, wordleWinPoints)
		client.Say(channel, fmt.Sprintf(
			"%s 🎉 ¡@%s adivinó la palabra! Era: %s (+%d lattepoints ☕)",
			squares, msg.User.Name, word, wordleWinPoints,
		))
		return
	}

	client.Say(channel, fmt.Sprintf("@%s %s", msg.User.Name, squares))
}
