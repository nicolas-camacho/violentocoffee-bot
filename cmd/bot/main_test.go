package main

import (
	"database/sql"
	"log"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMain(m *testing.M) {
	broadcasterName = "testbroadcaster"
	setupTestDB()
	os.Exit(m.Run())
}

func setupTestDB() {
	var err error
	db, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		log.Fatalf("[test] open in-memory db: %v", err)
	}
	db.SetMaxOpenConns(1)

	stmts := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA foreign_keys=ON`,
		`CREATE TABLE IF NOT EXISTS users (
			username  TEXT PRIMARY KEY,
			points    INTEGER NOT NULL DEFAULT 0,
			weapon_id INTEGER REFERENCES weapons(id)
		)`,
		`CREATE TABLE IF NOT EXISTS weapons (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			name        TEXT    NOT NULL UNIQUE,
			damage      INTEGER NOT NULL,
			price       INTEGER NOT NULL,
			crit_chance INTEGER NOT NULL DEFAULT 0,
			crit_mult   REAL    NOT NULL DEFAULT 0.0
		)`,
		`CREATE TABLE IF NOT EXISTS inventory (
			username  TEXT    NOT NULL,
			weapon_id INTEGER NOT NULL,
			PRIMARY KEY (username, weapon_id)
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			log.Fatalf("[test] schema: %v", err)
		}
	}
	seedWeapons()
}
