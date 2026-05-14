package main

import (
	"testing"
	"time"
)

func TestNormalizeWord(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"hello", "HELLO"},
		{"Álamo", "ALAMO"},
		{"éxito", "EXITO"},
		{"Ímpar", "IMPAR"},
		{"Óscar", "OSCAR"},
		{"Único", "UNICO"},
		{"güero", "GUERO"},
		{"Ñoño", "ÑOÑO"},
		{"abc123", "ABC"},
		{"", ""},
		{"¡Hola!", "HOLA"},
	}

	for _, tc := range cases {
		got := normalizeWord(tc.input)
		if got != tc.want {
			t.Errorf("normalizeWord(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestEvaluateWordle(t *testing.T) {
	cases := []struct {
		name  string
		word  string
		guess string
		want  string
	}{
		{
			name:  "all green",
			word:  "GATOS",
			guess: "GATOS",
			want:  "🟩🟩🟩🟩🟩",
		},
		{
			name:  "all black",
			word:  "GATOS",
			guess: "FBRIL",
			want:  "⬛⬛⬛⬛⬛",
		},
		{
			name:  "all yellow",
			word:  "ABCDE",
			guess: "EABCD",
			want:  "🟨🟨🟨🟨🟨",
		},
		{
			name:  "mixed green and black",
			word:  "PERRO",
			guess: "PXRXO",
			want:  "🟩⬛🟩⬛🟩",
		},
		{
			name:  "duplicate letter in guess — only first consumed",
			word:  "AABBC",
			guess: "AAABC",
			want:  "🟩🟩⬛🟩🟩",
		},
		{
			name:  "letter in word once guessed twice — second black",
			word:  "BRAVE",
			guess: "AARRR",
			want:  "🟨⬛🟨⬛⬛",
		},
		{
			name:  "yellow then black for same repeated letter",
			word:  "SALSA",
			guess: "SSXXX",
			want:  "🟩🟨⬛⬛⬛",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := evaluateWordle(tc.word, tc.guess)
			if got != tc.want {
				t.Errorf("evaluateWordle(%q, %q) = %q, want %q", tc.word, tc.guess, got, tc.want)
			}
		})
	}
}

func TestResetIfNewWord(t *testing.T) {
	t.Run("new word from empty state", func(t *testing.T) {
		s := wordleState{}
		resetIfNewWord(&s, "GATOS")
		if s.CurrentWord != "GATOS" {
			t.Errorf("CurrentWord = %q, want %q", s.CurrentWord, "GATOS")
		}
		if s.Winner != "" {
			t.Errorf("Winner should be empty after reset, got %q", s.Winner)
		}
		if s.Date == "" {
			t.Error("Date should be set after reset")
		}
	})

	t.Run("new word preserves last winner", func(t *testing.T) {
		prev := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
		s := wordleState{
			CurrentWord: "PERRO",
			Winner:      "alice",
			WinnerAt:    prev,
			WinnerWord:  "PERRO",
		}
		resetIfNewWord(&s, "GATOS")
		if s.LastWinner != "alice" {
			t.Errorf("LastWinner = %q, want %q", s.LastWinner, "alice")
		}
		if !s.LastWinnerAt.Equal(prev) {
			t.Errorf("LastWinnerAt = %v, want %v", s.LastWinnerAt, prev)
		}
		if s.LastWinnerWord != "PERRO" {
			t.Errorf("LastWinnerWord = %q, want %q", s.LastWinnerWord, "PERRO")
		}
		if s.Winner != "" {
			t.Errorf("Winner should be cleared, got %q", s.Winner)
		}
		if s.CurrentWord != "GATOS" {
			t.Errorf("CurrentWord = %q, want %q", s.CurrentWord, "GATOS")
		}
	})

	t.Run("same word no reset", func(t *testing.T) {
		s := wordleState{
			CurrentWord: "GATOS",
			Winner:      "bob",
			Date:        "2025-01-01",
		}
		resetIfNewWord(&s, "GATOS")
		if s.Winner != "bob" {
			t.Errorf("Winner should not change, got %q", s.Winner)
		}
		if s.Date != "2025-01-01" {
			t.Errorf("Date should not change, got %q", s.Date)
		}
	})

	t.Run("accent normalisation — same word different encoding no reset", func(t *testing.T) {
		s := wordleState{CurrentWord: "GATÓS", Winner: "charlie"}
		resetIfNewWord(&s, "GATOS")
		if s.Winner != "charlie" {
			t.Errorf("Winner should not change when normalised words match, got %q", s.Winner)
		}
	})

	t.Run("no winner — LastWinner not overwritten on reset", func(t *testing.T) {
		s := wordleState{
			CurrentWord: "VIEJO",
			Winner:      "",
			LastWinner:  "prior_winner",
		}
		resetIfNewWord(&s, "NUEVO")
		if s.LastWinner != "prior_winner" {
			t.Errorf("LastWinner should not be overwritten when no current winner; got %q", s.LastWinner)
		}
	})
}
