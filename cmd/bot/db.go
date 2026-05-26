package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	_ "modernc.org/sqlite"
)

var db *sql.DB

type Weapon struct {
	ID         int
	Name       string
	Damage     int
	Price      int
	CritChance int     // added to base crit chance (%)
	CritMult   float64 // added to base crit multiplier (e.g. 0.5 → 2.5× total)
}

// defaultWeapons seeds / updates on every boot.
// CritChance: bonus % on top of base 5%.
// CritMult:   bonus multiplier on top of base 2.0×.
var defaultWeapons = []Weapon{
	{Name: "palo",      Damage:  5, Price:   0, CritChance:  0, CritMult: 0.0},
	{Name: "daga",      Damage: 15, Price: 100, CritChance:  8, CritMult: 0.0},
	{Name: "espada",    Damage: 25, Price: 200, CritChance:  0, CritMult: 0.0},
	{Name: "arco",      Damage: 28, Price: 300, CritChance: 12, CritMult: 0.0},
	{Name: "hacha",     Damage: 40, Price: 400, CritChance:  0, CritMult: 0.5},
	{Name: "ballesta",  Damage: 33, Price: 450, CritChance:  8, CritMult: 0.3},
	{Name: "lanza",     Damage: 45, Price: 500, CritChance:  0, CritMult: 0.0},
	{Name: "estilete",  Damage: 18, Price: 350, CritChance: 25, CritMult: 0.0},
	{Name: "baculo",    Damage: 50, Price: 600, CritChance:  0, CritMult: 1.0},
	{Name: "katana",    Damage: 35, Price: 650, CritChance: 15, CritMult: 0.5},
	{Name: "maul",           Damage:  65, Price:     900, CritChance:  0, CritMult: 0.8},
	{Name: "arco_larg",      Damage:  42, Price:     800, CritChance: 18, CritMult: 0.5},
	{Name: "mandoble",       Damage:  75, Price:    1200, CritChance:  0, CritMult: 1.0},
	{Name: "grimorio",       Damage:  60, Price:    1500, CritChance:  5, CritMult: 1.5},
	{Name: "flamberge",      Damage:  80, Price:    2000, CritChance: 10, CritMult: 0.8},
	{Name: "runas",          Damage:  90, Price:    3500, CritChance:  0, CritMult: 2.0},
	{Name: "espada_sagrada", Damage: 100, Price:    5000, CritChance:  8, CritMult: 1.5},
	{Name: "canon",          Damage: 130, Price:   10000, CritChance:  0, CritMult: 0.5},
	{Name: "laser",          Damage: 160, Price:   25000, CritChance: 15, CritMult: 1.0},
	{Name: "evacoffee",      Damage: 250, Price: 1000000, CritChance: 25, CritMult: 3.0},
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite", "violentocoffee.db")
	if err != nil {
		log.Fatalf("[db] open: %v", err)
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
			log.Fatalf("[db] init: %v", err)
		}
	}

	// Add columns to existing DBs (no-op if already present)
	db.Exec(`ALTER TABLE weapons ADD COLUMN crit_chance INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE weapons ADD COLUMN crit_mult REAL NOT NULL DEFAULT 0.0`)

	seedWeapons()
	migrateLattepoints()
}

func seedWeapons() {
	for _, w := range defaultWeapons {
		db.Exec(
			`INSERT INTO weapons (name, damage, price, crit_chance, crit_mult) VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(name) DO UPDATE SET
			   damage=excluded.damage, price=excluded.price,
			   crit_chance=excluded.crit_chance, crit_mult=excluded.crit_mult`,
			w.Name, w.Damage, w.Price, w.CritChance, w.CritMult,
		)
	}
}

func migrateLattepoints() {
	data, err := os.ReadFile(lattepointsFile)
	if err != nil {
		return
	}
	var pts map[string]int
	if json.Unmarshal(data, &pts) != nil {
		return
	}
	tx, _ := db.Begin()
	n := 0
	for username, points := range pts {
		res, _ := tx.Exec(
			`INSERT OR IGNORE INTO users (username, points) VALUES (?, ?)`,
			username, points,
		)
		if rows, _ := res.RowsAffected(); rows > 0 {
			n++
		}
	}
	tx.Commit()
	if n > 0 {
		log.Printf("[db] migrated %d users from lattepoints.json", n)
	}
}

// ─── Points ───────────────────────────────────────────────────────────────────

func loadPoints() map[string]int {
	rows, err := db.Query(`SELECT username, points FROM users`)
	if err != nil {
		return make(map[string]int)
	}
	defer rows.Close()
	pts := make(map[string]int)
	for rows.Next() {
		var u string
		var p int
		rows.Scan(&u, &p)
		pts[u] = p
	}
	return pts
}

func getPoints(username string) int {
	var pts int
	db.QueryRow(`SELECT points FROM users WHERE username = ?`, username).Scan(&pts)
	return pts
}

func addPoints(username string, amount int) {
	db.Exec(
		`INSERT INTO users (username, points) VALUES (?, ?)
		 ON CONFLICT(username) DO UPDATE SET points = points + excluded.points`,
		username, amount,
	)
	refreshLeaderboard()
}

func spendPoints(username string, amount int) bool {
	tx, err := db.Begin()
	if err != nil {
		return false
	}
	var cur int
	tx.QueryRow(`SELECT points FROM users WHERE username = ?`, username).Scan(&cur)
	if cur < amount {
		tx.Rollback()
		return false
	}
	tx.Exec(`UPDATE users SET points = points - ? WHERE username = ?`, amount, username)
	if err := tx.Commit(); err != nil {
		return false
	}
	refreshLeaderboard()
	return true
}

func bulkAddPoints(rewards map[string]int) {
	tx, _ := db.Begin()
	for username, amount := range rewards {
		tx.Exec(
			`INSERT INTO users (username, points) VALUES (?, ?)
			 ON CONFLICT(username) DO UPDATE SET points = points + excluded.points`,
			username, amount,
		)
	}
	tx.Commit()
	refreshLeaderboard()
}

func topPoints(n int) []TopEntry {
	rows, err := db.Query(
		`SELECT username, points FROM users WHERE username != ? ORDER BY points DESC LIMIT ?`,
		broadcasterName, n,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	entries := make([]TopEntry, 0, n)
	for rows.Next() {
		var e TopEntry
		rows.Scan(&e.Username, &e.Points)
		entries = append(entries, e)
	}
	return entries
}

// ─── Weapons ──────────────────────────────────────────────────────────────────

func scanWeapon(s interface{ Scan(...any) error }, w *Weapon) error {
	return s.Scan(&w.ID, &w.Name, &w.Damage, &w.Price, &w.CritChance, &w.CritMult)
}

func getAllWeapons() []Weapon {
	rows, err := db.Query(`SELECT id, name, damage, price, crit_chance, crit_mult FROM weapons ORDER BY price`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var weapons []Weapon
	for rows.Next() {
		var w Weapon
		scanWeapon(rows, &w)
		weapons = append(weapons, w)
	}
	return weapons
}

func getInventory(username string) []Weapon {
	rows, err := db.Query(`
		SELECT w.id, w.name, w.damage, w.price, w.crit_chance, w.crit_mult
		FROM weapons w
		JOIN inventory i ON i.weapon_id = w.id
		WHERE i.username = ?`, username)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var weapons []Weapon
	for rows.Next() {
		var w Weapon
		scanWeapon(rows, &w)
		weapons = append(weapons, w)
	}
	return weapons
}

func getWeapon(username string) *Weapon {
	var w Weapon
	err := db.QueryRow(`
		SELECT w.id, w.name, w.damage, w.price, w.crit_chance, w.crit_mult
		FROM weapons w JOIN users u ON u.weapon_id = w.id
		WHERE u.username = ?`, username).Scan(&w.ID, &w.Name, &w.Damage, &w.Price, &w.CritChance, &w.CritMult)
	if err != nil {
		return nil
	}
	return &w
}

func getWeaponDamage(username string) int {
	w := getWeapon(username)
	if w == nil {
		return 10 // base: no weapon equipped
	}
	return w.Damage
}

func buyWeapon(username, weaponName string) (int, error) {
	var w Weapon
	err := db.QueryRow(
		`SELECT id, name, damage, price FROM weapons WHERE name = ?`, weaponName,
	).Scan(&w.ID, &w.Name, &w.Damage, &w.Price)
	if err != nil {
		return 0, fmt.Errorf("arma \"%s\" no existe. Usa !tienda para ver opciones", weaponName)
	}

	var count int
	db.QueryRow(
		`SELECT COUNT(*) FROM inventory WHERE username = ? AND weapon_id = ?`, username, w.ID,
	).Scan(&count)
	if count > 0 {
		return 0, fmt.Errorf("ya tienes %s en tu inventario", w.Name)
	}

	if !spendPoints(username, w.Price) {
		return 0, fmt.Errorf("puntos insuficientes (necesitas %d, tienes %d)", w.Price, getPoints(username))
	}

	db.Exec(`INSERT OR IGNORE INTO users (username, points) VALUES (?, 0)`, username)
	db.Exec(`INSERT INTO inventory (username, weapon_id) VALUES (?, ?)`, username, w.ID)
	return w.Price, nil
}

func equipWeapon(username, weaponName string) error {
	var weaponID int
	err := db.QueryRow(`
		SELECT w.id FROM weapons w
		JOIN inventory i ON i.weapon_id = w.id
		WHERE i.username = ? AND w.name = ?`, username, weaponName).Scan(&weaponID)
	if err != nil {
		return fmt.Errorf("no tienes \"%s\" en tu inventario. Usa !comprar para adquirirla", weaponName)
	}
	db.Exec(`UPDATE users SET weapon_id = ? WHERE username = ?`, weaponID, username)
	return nil
}
