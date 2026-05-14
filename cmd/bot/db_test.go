package main

import (
	"testing"
)

func TestAddAndGetPoints(t *testing.T) {
	user := "db_test_addget_1"

	if got := getPoints(user); got != 0 {
		t.Errorf("new user points = %d, want 0", got)
	}

	addPoints(user, 100)
	if got := getPoints(user); got != 100 {
		t.Errorf("after addPoints(100) = %d, want 100", got)
	}

	addPoints(user, 50)
	if got := getPoints(user); got != 150 {
		t.Errorf("after second addPoints(50) = %d, want 150", got)
	}
}

func TestSpendPoints(t *testing.T) {
	user := "db_test_spend_1"
	addPoints(user, 200)

	t.Run("success", func(t *testing.T) {
		if ok := spendPoints(user, 80); !ok {
			t.Fatal("spendPoints should return true when balance is sufficient")
		}
		if got := getPoints(user); got != 120 {
			t.Errorf("after spending 80 from 200 = %d, want 120", got)
		}
	})

	t.Run("insufficient balance", func(t *testing.T) {
		if ok := spendPoints(user, 9999); ok {
			t.Fatal("spendPoints should return false when balance is insufficient")
		}
		if got := getPoints(user); got != 120 {
			t.Errorf("balance unchanged after failed spend = %d, want 120", got)
		}
	})
}

func TestBulkAddPoints(t *testing.T) {
	u1, u2, u3 := "db_test_bulk_1", "db_test_bulk_2", "db_test_bulk_3"

	bulkAddPoints(map[string]int{u1: 300, u2: 150, u3: 75})

	if got := getPoints(u1); got != 300 {
		t.Errorf("%s points = %d, want 300", u1, got)
	}
	if got := getPoints(u2); got != 150 {
		t.Errorf("%s points = %d, want 150", u2, got)
	}
	if got := getPoints(u3); got != 75 {
		t.Errorf("%s points = %d, want 75", u3, got)
	}

	bulkAddPoints(map[string]int{u1: 100})
	if got := getPoints(u1); got != 400 {
		t.Errorf("%s after second bulk add = %d, want 400", u1, got)
	}
}

func TestTopPoints(t *testing.T) {
	users := map[string]int{
		"db_test_top_a":  500,
		"db_test_top_b":  300,
		"db_test_top_c":  400,
		"db_test_top_d":  100,
		"testbroadcaster": 999, // must be excluded
	}
	for u, p := range users {
		addPoints(u, p)
	}

	entries := topPoints(3)
	if len(entries) != 3 {
		t.Fatalf("topPoints(3) returned %d entries, want 3", len(entries))
	}

	for i := 1; i < len(entries); i++ {
		if entries[i].Points > entries[i-1].Points {
			t.Errorf("entries not descending at [%d]: %d > %d", i, entries[i].Points, entries[i-1].Points)
		}
	}

	for _, e := range entries {
		if e.Username == "testbroadcaster" {
			t.Errorf("topPoints must exclude broadcasterName, found %q", e.Username)
		}
	}
}

func TestBuyWeapon(t *testing.T) {
	t.Run("success deducts points and adds to inventory", func(t *testing.T) {
		user := "db_test_buy_1"
		addPoints(user, 500)

		cost, err := buyWeapon(user, "daga")
		if err != nil {
			t.Fatalf("buyWeapon unexpected error: %v", err)
		}
		if cost != 100 {
			t.Errorf("returned cost = %d, want 100", cost)
		}
		if got := getPoints(user); got != 400 {
			t.Errorf("points after purchase = %d, want 400", got)
		}

		inv := getInventory(user)
		found := false
		for _, w := range inv {
			if w.Name == "daga" {
				found = true
				break
			}
		}
		if !found {
			t.Error("daga not found in inventory after purchase")
		}
	})

	t.Run("insufficient points", func(t *testing.T) {
		user := "db_test_buy_2"
		addPoints(user, 50)
		if _, err := buyWeapon(user, "espada"); err == nil {
			t.Fatal("buyWeapon should return error when balance is insufficient")
		}
	})

	t.Run("already owned", func(t *testing.T) {
		user := "db_test_buy_3"
		addPoints(user, 1000)
		if _, err := buyWeapon(user, "arco"); err != nil {
			t.Fatalf("first buyWeapon failed: %v", err)
		}
		if _, err := buyWeapon(user, "arco"); err == nil {
			t.Fatal("buyWeapon should return error when weapon already owned")
		}
	})

	t.Run("nonexistent weapon", func(t *testing.T) {
		user := "db_test_buy_4"
		addPoints(user, 1000)
		if _, err := buyWeapon(user, "espada_magica_xyz"); err == nil {
			t.Fatal("buyWeapon should return error for nonexistent weapon")
		}
	})
}

func TestEquipWeapon(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		user := "db_test_equip_1"
		addPoints(user, 500)
		if _, err := buyWeapon(user, "daga"); err != nil {
			t.Fatalf("setup buyWeapon: %v", err)
		}
		if err := equipWeapon(user, "daga"); err != nil {
			t.Errorf("equipWeapon unexpected error: %v", err)
		}
	})

	t.Run("not in inventory", func(t *testing.T) {
		user := "db_test_equip_2"
		if err := equipWeapon(user, "katana"); err == nil {
			t.Fatal("equipWeapon should return error when weapon not in inventory")
		}
	})
}

func TestGetWeapon(t *testing.T) {
	t.Run("returns nil when none equipped", func(t *testing.T) {
		user := "db_test_getwep_1"
		if w := getWeapon(user); w != nil {
			t.Errorf("getWeapon for user with no weapon = %+v, want nil", w)
		}
	})

	t.Run("returns correct weapon after equip", func(t *testing.T) {
		user := "db_test_getwep_2"
		addPoints(user, 500)
		if _, err := buyWeapon(user, "daga"); err != nil {
			t.Fatalf("setup buyWeapon: %v", err)
		}
		if err := equipWeapon(user, "daga"); err != nil {
			t.Fatalf("setup equipWeapon: %v", err)
		}

		w := getWeapon(user)
		if w == nil {
			t.Fatal("getWeapon returned nil after equipping daga")
		}
		if w.Name != "daga" {
			t.Errorf("weapon name = %q, want %q", w.Name, "daga")
		}
		if w.Damage != 15 {
			t.Errorf("daga damage = %d, want 15", w.Damage)
		}
	})
}

func TestGetWeaponDamage(t *testing.T) {
	t.Run("base damage when no weapon equipped", func(t *testing.T) {
		user := "db_test_wdmg_1"
		if got := getWeaponDamage(user); got != 10 {
			t.Errorf("getWeaponDamage with no weapon = %d, want 10", got)
		}
	})

	t.Run("weapon damage when equipped", func(t *testing.T) {
		user := "db_test_wdmg_2"
		addPoints(user, 1000)
		if _, err := buyWeapon(user, "espada"); err != nil {
			t.Fatalf("setup buyWeapon: %v", err)
		}
		if err := equipWeapon(user, "espada"); err != nil {
			t.Fatalf("setup equipWeapon: %v", err)
		}
		if got := getWeaponDamage(user); got != 25 {
			t.Errorf("getWeaponDamage with espada = %d, want 25", got)
		}
	})
}
