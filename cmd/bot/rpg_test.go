package main

import (
	"testing"
	"time"
)

func TestBuildAttackerList(t *testing.T) {
	attackers := map[string]int{
		"alice": 300,
		"bob":   500,
		"carol": 100,
		"dave":  450,
		"eve":   200,
		"frank": 350,
	}

	list := buildAttackerList(attackers)

	if len(list) != 5 {
		t.Fatalf("buildAttackerList len = %d, want 5 (capped)", len(list))
	}

	for i := 1; i < len(list); i++ {
		if list[i].Damage > list[i-1].Damage {
			t.Errorf("list not descending at [%d]: %d > %d", i, list[i].Damage, list[i-1].Damage)
		}
	}

	if list[0].Username != "bob" || list[0].Damage != 500 {
		t.Errorf("top attacker = {%s %d}, want {bob 500}", list[0].Username, list[0].Damage)
	}

	for _, a := range list {
		if a.Username == "carol" {
			t.Error("carol (lowest damage) should be excluded from top-5 list")
		}
	}
}

func TestBuildAttackerListFewerThanFive(t *testing.T) {
	attackers := map[string]int{
		"alpha": 200,
		"beta":  100,
	}
	list := buildAttackerList(attackers)
	if len(list) != 2 {
		t.Fatalf("expected 2 entries for 2-attacker map, got %d", len(list))
	}
	if list[0].Username != "alpha" || list[0].Damage != 200 {
		t.Errorf("top attacker = {%s %d}, want {alpha 200}", list[0].Username, list[0].Damage)
	}
}

func TestBuildAttackerListEmpty(t *testing.T) {
	list := buildAttackerList(map[string]int{})
	if len(list) != 0 {
		t.Errorf("empty map should return empty list, got %d entries", len(list))
	}
}

func TestEffectiveCooldown(t *testing.T) {
	t.Run("no effects returns base attackCooldown", func(t *testing.T) {
		activeEffects = nil
		got := effectiveCooldown()
		if got != attackCooldown {
			t.Errorf("effectiveCooldown() = %v, want %v", got, attackCooldown)
		}
	})

	t.Run("active increase_cooldown multiplies cooldown", func(t *testing.T) {
		activeEffects = []activeEffect{
			{kind: "increase_cooldown", value: 2.0, expires: time.Now().Add(10 * time.Minute)},
		}
		got := effectiveCooldown()
		want := time.Duration(float64(attackCooldown) * 2.0)
		if got != want {
			t.Errorf("effectiveCooldown() with 2× = %v, want %v", got, want)
		}
		activeEffects = nil
	})

	t.Run("expired effect is ignored", func(t *testing.T) {
		activeEffects = []activeEffect{
			{kind: "increase_cooldown", value: 3.0, expires: time.Now().Add(-1 * time.Second)},
		}
		got := effectiveCooldown()
		if got != attackCooldown {
			t.Errorf("expired effect should not change cooldown; got %v, want %v", got, attackCooldown)
		}
		activeEffects = nil
	})

	t.Run("non-cooldown effect does not change cooldown", func(t *testing.T) {
		activeEffects = []activeEffect{
			{kind: "reduce_crit", value: 15, expires: time.Now().Add(10 * time.Minute)},
		}
		got := effectiveCooldown()
		if got != attackCooldown {
			t.Errorf("non-cooldown effect changed cooldown; got %v, want %v", got, attackCooldown)
		}
		activeEffects = nil
	})
}

func TestDungeonPartyList(t *testing.T) {
	t.Run("nil party returns empty slice", func(t *testing.T) {
		dungeonParty = nil
		list := dungeonPartyList()
		if len(list) != 0 {
			t.Errorf("nil dungeonParty returned %d entries, want 0", len(list))
		}
	})

	t.Run("empty party returns empty slice", func(t *testing.T) {
		dungeonParty = map[string]bool{}
		list := dungeonPartyList()
		if len(list) != 0 {
			t.Errorf("empty dungeonParty returned %d entries, want 0", len(list))
		}
	})

	t.Run("returns all members", func(t *testing.T) {
		dungeonParty = map[string]bool{"alice": true, "bob": true, "carol": true}
		list := dungeonPartyList()
		if len(list) != 3 {
			t.Fatalf("dungeonPartyList len = %d, want 3", len(list))
		}

		members := map[string]bool{"alice": true, "bob": true, "carol": true}
		for _, u := range list {
			if !members[u] {
				t.Errorf("unexpected member in party list: %q", u)
			}
			delete(members, u)
		}
		if len(members) != 0 {
			t.Errorf("missing members from party list: %v", members)
		}
		dungeonParty = nil
	})
}
