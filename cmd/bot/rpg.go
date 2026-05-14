package main

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"
)

// ─── Boss phase system ────────────────────────────────────────────────────────

type BossEffect struct {
	kind     string        // "reduce_crit" | "increase_cooldown" | "reduce_damage" | "regen"
	value    float64       // reduction pts / cooldown mult / damage mult / regen fraction
	duration time.Duration
	startMsg string
	endMsg   string
}

type BossPhase struct {
	hpPct   int // trigger when HP drops to this % (0 = not HP-based)
	timeSec int // trigger after N seconds (0 = not time-based)
	effect  BossEffect
}

type activeBossPhase struct {
	BossPhase
	fired bool
}

type activeEffect struct {
	kind    string
	value   float64
	expires time.Time
}

// ─── State machine ────────────────────────────────────────────────────────────

type RPGState int

const (
	StateIdle RPGState = iota
	StateBoss
	StateDungeon
	StateChest
)

var (
	rpgMu    sync.Mutex
	rpgState = StateIdle
	rpgSay   func(string)

	activeBoss       *BossInfo
	bossAttackers    map[string]int       // username → cumulative damage
	attackCooldowns  map[string]time.Time // username → last attack time
	attackCounts     map[string]int       // username → number of attacks
	activeBossPhases []activeBossPhase
	activeEffects    []activeEffect

	activeDungeon        *DungeonInfo
	currentDungeonResist int
	dungeonParty         map[string]bool
	dungeonWeapons       map[string]int // username → weapon damage at join time

	chestOpeners     map[string]bool
	chestSecondsLeft int

	bossVictoryMsg string
	bossEscapeMsg  string
	dungeonVictory string
	dungeonDefeat  string
)

const (
	attackCooldown = 30 * time.Second
	critChance     = 5   // base %
	critMultiplier = 2.0 // base multiplier

	bossRewardPool   = 500
	dungeonReward    = 150
	dungeonDuration  = 60 * time.Second
	chestDuration    = 30 * time.Second
	chestTreasurePct = 70
	chestTreasureAmt = 200
	chestMimicAmt    = 100
)

var bossCatalog = []struct {
	name    string
	hp      int
	summon  string
	victory string
	escape  string
	phases  []BossPhase
}{
	{
		name: "Aldric el Sin Nombre", hp: 800,
		summon:  "⚔️ Un caballero sin corona emerge de la oscuridad. Sus ojos vacíos buscan almas. !atacar para combatir.",
		victory: "⚔️ Aldric cae. Su armadura se desmorona en cenizas frías, sin nombre y sin gloria.",
		escape:  "⚔️ Aldric desaparece entre las sombras. Nadie tuvo el valor de enfrentarlo.",
	},
	{
		name: "La Viuda de Cristal", hp: 650,
		summon:  "⚔️ Una anciana de porcelana y veneno se arrastra desde el fondo del lago. !atacar antes de que cante.",
		victory: "⚔️ La Viuda se quiebra en mil fragmentos. Su último susurro se pierde en el viento.",
		escape:  "⚔️ La Viuda se hunde de nuevo. Solo los cobardes la dejan marchar.",
	},
	{
		name: "Morgott el Proscrito", hp: 1200,
		summon:  "⚔️ ¡Oh, viajeros sin gracia! Morgott, Rey Proscrito, desciende a juzgar vuestras almas miserables. !atacar.",
		victory: "⚔️ Morgott cae de rodillas. Sus cuernos tocan el suelo por última vez. La gracia os abandona.",
		escape:  "⚔️ Morgott ríe y se esfuma. Os llama proscrit@s y tiene razón.",
		phases: []BossPhase{
			{hpPct: 60, effect: BossEffect{
				kind: "increase_cooldown", value: 2.0, duration: 50 * time.Second,
				startMsg: "👑 ¡FASE 2! Morgott desata su poder real. El cooldown de ataques se duplica a 60s durante 50s.",
				endMsg:   "👑 El poder de Morgott mengua. El cooldown vuelve a 30s.",
			}},
		},
	},
	{
		name: "Graf Vlad de Dorakheim", hp: 1000,
		summon:  "⚔️ Las puertas del castillo se abren. El Conde emerge: inmortal, hambriento y furioso. !atacar.",
		victory: "⚔️ El Conde se desintegra en polvo y murciélagos. Mas no olvidéis: la muerte es solo un descanso para él.",
		escape:  "⚔️ El Conde se retira al castillo. Os llama plaga de mortales sin valor.",
		phases: []BossPhase{
			{timeSec: 60, effect: BossEffect{
				kind: "reduce_crit", value: 15, duration: 35 * time.Second,
				startMsg: "🦇 ¡Vlad invoca la Niebla de Sangre! El chance crítico se reduce -15% durante 35s.",
				endMsg:   "🦇 La Niebla de Sangre se disipa. El chance crítico restaurado.",
			}},
		},
	},
	{
		name: "El Caballero de Hierro Hueco", hp: 700,
		summon:  "⚔️ Un eco metálico resuena. El Caballero Hueco avanza, sin alma, sin propósito, sin piedad. !atacar.",
		victory: "⚔️ El hierro se oxida y colapsa. Solo queda el silencio y el olor a carne quemada.",
		escape:  "⚔️ El Caballero se detiene. Os mira un momento y marcha. Ni siquiera merecéis su furia.",
	},
	{
		name: "Malenia la Podrida", hp: 1500,
		summon:  "⚔️ ¡ALERTA! Malenia, Diosa de la Podredumbre, desciende. Cada golpe la CURA. !atacar sin descanso.",
		victory: "⚔️ Sus pétalos caen. Malenia susurra: 'He conocido la derrota...' La Podredumbre se retira.",
		escape:  "⚔️ Malenia florece. La Podredumbre se extiende. Nadie pudo detenerla.",
		phases: []BossPhase{
			{hpPct: 50, effect: BossEffect{
				kind: "regen", value: 0.12, duration: 999 * time.Hour,
				startMsg: "🌸 ¡FASE 2! Malenia florece y se CURA con cada golpe (12% del daño recibido). ¡Atacad sin piedad!",
				endMsg:   "",
			}},
		},
	},
	{
		name: "Padre Gilles el Cazador", hp: 680,
		summon:  "⚔️ Aullidos en la noche. El cazador se ha convertido en bestia. Solo la sangre aplacará su furia. !atacar.",
		victory: "⚔️ El padre cae. En sus últimos instantes, recupera la cordura. Una lágrima. Luego, nada.",
		escape:  "⚔️ La bestia huye hacia los techos. La caza continuará otra noche.",
	},
	{
		name: "El Señor de la Blasfemia", hp: 1300,
		summon:  "⚔️ La Gran Serpiente extiende su lengua. Rykard os invita a unirse a su festín eterno. !atacar.",
		victory: "⚔️ La Serpiente Blasfema se retuerce y muere. Su carne putrefacta alimentará la tierra por siglos.",
		escape:  "⚔️ Rykard ríe desde sus profundidades. Seréis devorad@s. Solo es cuestión de tiempo.",
		phases: []BossPhase{
			{timeSec: 80, effect: BossEffect{
				kind: "regen", value: 0.08, duration: 999 * time.Hour,
				startMsg: "🐍 ¡La Gran Serpiente se regenera! La blasfemia le otorga curación del 8% del daño recibido.",
				endMsg:   "",
			}},
		},
	},
	{
		name: "El Inquisidor de la Llama Negra", hp: 950,
		summon:  "⚔️ El Nuevo Hombre avanza desde las trincheras. Porta la Llama Negra con fervor herético. !atacar.",
		victory: "⚔️ El Inquisidor arde en su propia llama. Una herejía extinguida. El frente, silenciado.",
		escape:  "⚔️ El Inquisidor alza su antorcha. Los traidores serán purgados. Comenzando por vosotros.",
	},
	{
		name: "La Gran Bestia Antigua", hp: 1100,
		summon:  "⚔️ El cielo se oscurece. La Gran Bestia desciende del cosmos. Sus cadenas ya no la contienen. !atacar.",
		victory: "⚔️ La Bestia colapsa. Las cadenas que la aprisionaban se deshacen. El cosmos respira.",
		escape:  "⚔️ La Bestia asciende de nuevo. Llevará vuestros nombres grabados en su memoria eterna.",
		phases: []BossPhase{
			{timeSec: 45, effect: BossEffect{
				kind: "reduce_damage", value: 0.5, duration: 40 * time.Second,
				startMsg: "⭐ ¡La Gran Bestia activa su Aura Cósmica! El daño infligido se reduce un 50% durante 40s.",
				endMsg:   "⭐ El Aura Cósmica se desvanece. El daño vuelve a la normalidad.",
			}},
		},
	},
	{
		name: "El Lobo Cinereo", hp: 820,
		summon:  "⚔️ Un lobo de pelaje gris como la ceniza bloquea el camino. Sus ojos guardan una tristeza antigua. !atacar.",
		victory: "⚔️ El lobo cae. Mientras yace, agita la cola una última vez. Guardó su tumba hasta el final.",
		escape:  "⚔️ El lobo os observa y se retira. Sabe que volveréis. Siempre volvéis.",
	},
	{
		name: "El Arzobispo Herético", hp: 1000,
		summon:  "⚔️ El Arzobispo entona una oración invertida. Su fe alimenta algo antiguo y hambriento. !atacar.",
		victory: "⚔️ El Arzobispo cae en silencio. Sus escrituras se queman solas. La herejía, purificada.",
		escape:  "⚔️ El Arzobispo completa su rito. Algo fue invocado. Algo peor viene en camino.",
		phases: []BossPhase{
			{hpPct: 55, effect: BossEffect{
				kind: "reduce_crit", value: 20, duration: 50 * time.Second,
				startMsg: "⛪ ¡El Arzobispo alza el Escudo Herético! El chance crítico se reduce -20% durante 50s.",
				endMsg:   "⛪ El Escudo Herético se quiebra. El chance crítico restaurado.",
			}},
		},
	},
	{
		name: "Sísifo el Inmortal", hp: 880,
		summon:  "⚔️ El guerrero maldito desciende de su montaña. Ha muerto mil veces. Mil veces ha vuelto. !atacar.",
		victory: "⚔️ Sísifo cae por última vez. Su piedra rueda sin nadie que la empuje. Descansa, al fin.",
		escape:  "⚔️ Sísifo asciende de nuevo. La maldición continúa. Y vosotros sois parte de ella.",
	},
	{
		name: "Dama del Lago Astral", hp: 920,
		summon:  "⚔️ La Dama desciende desde el lago suspendido en el cielo. Su aguja baila con gracia mortal. !atacar.",
		victory: "⚔️ La aguja cae. La Dama susurra una disculpa antes de disolverse en luz de estrellas.",
		escape:  "⚔️ La Dama se eleva de vuelta al lago. Dejó flores donde pisó. Y cadáveres donde miró.",
	},
	{
		name: "Radahn Devorador de Estrellas", hp: 1400,
		summon:  "⚔️ ¡El festival de Radahn ha comenzado! El Cometa Rojo desciende. Gloriad al General. !atacar.",
		victory: "⚔️ Radahn cae. Las estrellas pueden moverse de nuevo. El cosmos recuerda su nombre con respeto.",
		escape:  "⚔️ Radahn os mira desde su caballo y sonríe. Honra vuestra cobardía. Por ahora.",
		phases: []BossPhase{
			{hpPct: 50, effect: BossEffect{
				kind: "increase_cooldown", value: 1.8, duration: 55 * time.Second,
				startMsg: "🌠 ¡FESTIVAL DE RADAHN! ¡Lluvia de meteoros! El cooldown de ataques sube a 54s durante 55s.",
				endMsg:   "🌠 Los meteoros cesan. El cooldown vuelve a 30s.",
			}},
		},
	},
	{
		name: "El Cruzado Caído", hp: 760,
		summon:  "⚔️ Un soldado del Nuevo Hombre deserta hacia la oscuridad. Porta armas benditas y corazón corrompido. !atacar.",
		victory: "⚔️ El Cruzado Caído se desmorona. Sus medallas de honor se funden en el barro de la trinchera.",
		escape:  "⚔️ El Cruzado retrocede. Dios os abandonó hoy. O quizás nunca estuvo aquí.",
	},
	{
		name: "El Devorador de Mundos", hp: 1600,
		summon:  "⚔️ ¡PELIGRO EXTREMO! El Devorador emerge del vacío entre dimensiones. Todo termina aquí. !atacar.",
		victory: "⚔️ El Devorador se colapsa en sí mismo. Un mundo salvado. Por ahora. Por poco.",
		escape:  "⚔️ El Devorador se retira satisfecho. Ya probó vuestro sabor. Volverá hambriento.",
		phases: []BossPhase{
			{hpPct: 50, effect: BossEffect{
				kind: "reduce_damage", value: 0.6, duration: 45 * time.Second,
				startMsg: "🌑 ¡El Devorador abre su coraza dimensional! El daño se reduce un 40% durante 45s. ¡Resistid!",
				endMsg:   "🌑 La coraza se cierra. El daño vuelve a la normalidad.",
			}},
			{hpPct: 25, effect: BossEffect{
				kind: "reduce_crit", value: 25, duration: 60 * time.Second,
				startMsg: "🌑 ¡FASE FINAL! El Devorador distorsiona la realidad. -25% chance crítico durante 60s. ¡Es ahora o nunca!",
				endMsg:   "🌑 La distorsión colapsa. El chance crítico restaurado.",
			}},
		},
	},
}

var dungeonCatalog = []struct {
	name    string
	resist  int
	summon  string
	victory string
	defeat  string
}{
	{
		name: "Cementerio de los Olvidados", resist: 200,
		summon:  "🏰 Las tumbas sin nombre abren sus puertas. Solo los valientes entran. !dungeon para unirte.",
		victory: "🏰 ¡Victoria! El cementerio descansa. Los olvidados, por fin, duermen en paz.",
		defeat:  "🏰 El cementerio os consume. Vuestros nombres se unen a los olvidados. Sin lápida.",
	},
	{
		name: "Las Catacumbas de Farron", resist: 300,
		summon:  "🏰 Los Guardianes de Farron vigilan las catacumbas. Solo la fuerza colectiva puede pasar. !dungeon.",
		victory: "🏰 ¡Los Guardianes caen! La llama de Farron se extingue. El paso es vuestro.",
		defeat:  "🏰 Los Guardianes os expulsan. La llama persiste. La Alianza Juramentada, invicta.",
	},
	{
		name: "Lago del Astral", resist: 350,
		summon:  "🏰 El lago suspendido entre estrellas os llama. Sus criaturas no pertenecen a este mundo. !dungeon.",
		victory: "🏰 ¡El Lago se serena! Las bestias astrales retroceden. La bruma se disipa.",
		defeat:  "🏰 El Lago os absorbe. Seréis parte de su colección eterna de almas náufragas.",
	},
	{
		name: "Torre del Faro Maldito", resist: 400,
		summon:  "🏰 El faro enciende una luz putrefacta. Algo espera en la cima. !dungeon para ascender.",
		victory: "🏰 ¡La luz del faro se apaga! La maldición se rompe. El mar vuelve a ser navegable.",
		defeat:  "🏰 La torre os expulsa. La luz sigue brillando. Otro barco se perderá esta noche.",
	},
	{
		name: "Refugio del Clan Vileblood", resist: 450,
		summon:  "🏰 El castillo de sangre os abre sus puertas. Los Vilebloods os esperan sedientos. !dungeon.",
		victory: "🏰 ¡El clan es purificado! La sangre corrompida se limpia con acero y voluntad.",
		defeat:  "🏰 Los Vilebloods os drenan. Vuestras venas alimentarán el linaje por generaciones.",
	},
	{
		name: "Las Minas Abisales", resist: 500,
		summon:  "🏰 Algo excava desde abajo. Las minas colapsan hacia el Abismo. !dungeon si no teméis la oscuridad.",
		victory: "🏰 ¡El Abismo retrocede! Las minas son vuestras. El eco de vuestra victoria resuena kilómetros.",
		defeat:  "🏰 El Abismo os traga. Allí abajo no hay luz. No hay regreso. Solo susurros.",
	},
	{
		name: "Catedral del Vacío Profundo", resist: 550,
		summon:  "🏰 La Catedral invierte su gravedad. El culto del Vacío invita. !dungeon para entrar.",
		victory: "🏰 ¡El culto disuelto! La Catedral colapsa sobre sí misma. El Vacío, por hoy, derrotado.",
		defeat:  "🏰 El Vacío os acepta. Sois parte del culto ahora. Sin quererlo. Sin poder evitarlo.",
	},
	{
		name: "Los Túneles de la Gran Bestia", resist: 600,
		summon:  "🏰 La Bestia dejó sus túneles repletos de crías hambrientas. !dungeon para limpiarlos.",
		victory: "🏰 ¡Los túneles, purificados! La Gran Bestia pierde sus herederos. Su linaje, truncado.",
		defeat:  "🏰 Las crías os rodean. La Gran Bestia tiene nuevos juguetes. Permanentes.",
	},
	{
		name: "El Frente de las Trincheras", resist: 650,
		summon:  "🏰 ¡Avance ordenado! El Nuevo Hombre defiende la trinchera con fe y munición. !dungeon.",
		victory: "🏰 ¡La trinchera tomada! La bandera del Nuevo Hombre cae. El frente, vuestro.",
		defeat:  "🏰 El Nuevo Hombre os repele. Dios os abandonó. O nunca existió. Igual da.",
	},
	{
		name: "El Palacio de Cristal Roto", resist: 700,
		summon:  "🏰 El palacio de la reina de hielo se fractura. Sus guardianes aún obedecen. !dungeon.",
		victory: "🏰 ¡El palacio colapsa! Los cristales caen en silencio. La reina no da la orden de retirada.",
		defeat:  "🏰 Los guardianes de cristal os fragmentan. Sois parte del palacio ahora. Decoración.",
	},
	{
		name: "Cripta de los Reyes Sin Corona", resist: 800,
		summon:  "🏰 ¡DESAFÍO EXTREMO! Los reyes que nunca reinaron reclaman su trono. !dungeon si sois dignos.",
		victory: "🏰 ¡Los reyes derrocados de nuevo! Sus coronas oxidadas se deshacen. Vosotros sois dignos.",
		defeat:  "🏰 Los reyes os juzgan indignos. Sois ejecutados por la ley de quienes nunca gobernaron. Irónico.",
	},
	{
		name: "Fortaleza del Fin de los Tiempos", resist: 900,
		summon:  "🏰 ¡PELIGRO MÁXIMO! La fortaleza donde el tiempo muere. Nada sale igual. !dungeon para intentarlo.",
		victory: "🏰 ¡Fortaleza tomada! El tiempo reanuda su marcha. Lo que fue sacrificado no puede ser devuelto.",
		defeat:  "🏰 La fortaleza os atrapa en su bucle. Moriréis aquí para siempre. En loop.",
	},
}

// ─── Event loop ───────────────────────────────────────────────────────────────

func startRPGEventLoop(say func(string)) {
	rpgSay = say
	go func() {
		for {
			time.Sleep(10 * time.Minute)

			rpgMu.Lock()
			if rpgState == StateIdle {
				rollAndTrigger()
			}
			rpgMu.Unlock()
		}
	}()
}

func rollAndTrigger() {
	// called with rpgMu held
	roll := rand.Intn(100)
	switch {
	case roll < 50:
		triggerBoss()
	case roll < 80:
		triggerDungeon()
	default:
		triggerChest()
	}
}

func handleManualEvent(parts []string) {
	rpgMu.Lock()
	defer rpgMu.Unlock()
	if rpgState != StateIdle {
		return
	}
	kind := ""
	if len(parts) >= 2 {
		kind = strings.ToLower(parts[1])
	}
	switch kind {
	case "boss":
		triggerBoss()
	case "dungeon", "mazmorra":
		triggerDungeon()
	case "chest", "cofre":
		triggerChest()
	default:
		rollAndTrigger()
	}
}

// ─── Triggers (called with rpgMu held) ────────────────────────────────────────

func triggerBoss() {
	entry := bossCatalog[rand.Intn(len(bossCatalog))]
	activeBoss = &BossInfo{
		Name:  entry.name,
		HP:    entry.hp,
		MaxHP: entry.hp,
	}
	bossAttackers = make(map[string]int)
	attackCooldowns = make(map[string]time.Time)
	attackCounts = make(map[string]int)
	activeEffects = nil
	bossVictoryMsg = entry.victory
	bossEscapeMsg = entry.escape

	activeBossPhases = make([]activeBossPhase, len(entry.phases))
	for i, p := range entry.phases {
		activeBossPhases[i] = activeBossPhase{BossPhase: p}
	}
	for i := range activeBossPhases {
		if activeBossPhases[i].timeSec > 0 {
			idx := i
			delay := time.Duration(activeBossPhases[idx].timeSec) * time.Second
			go func() {
				time.Sleep(delay)
				rpgMu.Lock()
				defer rpgMu.Unlock()
				if rpgState == StateBoss && !activeBossPhases[idx].fired {
					activeBossPhases[idx].fired = true
					triggerBossEffect(activeBossPhases[idx].effect)
				}
			}()
		}
	}

	rpgState = StateBoss
	broadcastOverlay(OverlayState{State: "boss", Boss: activeBoss})
	rpgSay(entry.summon)
}

func triggerDungeon() {
	entry := dungeonCatalog[rand.Intn(len(dungeonCatalog))]
	currentDungeonResist = entry.resist + rand.Intn(entry.resist/2+1)
	activeDungeon = &DungeonInfo{
		Name:        entry.name,
		SecondsLeft: int(dungeonDuration.Seconds()),
		Party:       []string{},
	}
	dungeonParty = make(map[string]bool)
	dungeonWeapons = make(map[string]int)
	dungeonVictory = entry.victory
	dungeonDefeat = entry.defeat
	rpgState = StateDungeon
	broadcastOverlay(OverlayState{State: "dungeon", Dungeon: activeDungeon})
	rpgSay(entry.summon)

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			rpgMu.Lock()
			if rpgState != StateDungeon || activeDungeon == nil {
				rpgMu.Unlock()
				return
			}
			activeDungeon.SecondsLeft--
			activeDungeon.Party = dungeonPartyList()
			broadcastOverlay(OverlayState{State: "dungeon", Dungeon: activeDungeon})
			rpgMu.Unlock()
		}
	}()

	time.AfterFunc(dungeonDuration, resolveDungeon)
}

func triggerChest() {
	chestOpeners = make(map[string]bool)
	chestSecondsLeft = int(chestDuration.Seconds())
	rpgState = StateChest
	broadcastOverlay(OverlayState{State: "chest", Chest: &ChestInfo{SecondsLeft: chestSecondsLeft}})
	rpgSay(fmt.Sprintf("📦 ¡COFRE MÁGICO! Tienes %ds para escribir !cofre. ¿Tesoro o mímico?", chestSecondsLeft))

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			rpgMu.Lock()
			if rpgState != StateChest {
				rpgMu.Unlock()
				return
			}
			chestSecondsLeft--
			broadcastOverlay(OverlayState{State: "chest", Chest: &ChestInfo{SecondsLeft: chestSecondsLeft}})
			rpgMu.Unlock()
		}
	}()

	time.AfterFunc(chestDuration, func() {
		rpgMu.Lock()
		defer rpgMu.Unlock()
		if rpgState == StateChest {
			rpgState = StateIdle
			broadcastOverlay(OverlayState{State: "idle"})
			if len(chestOpeners) == 0 {
				rpgSay("📦 El cofre desapareció sin ser abierto.")
			}
		}
	})
}

// ─── Command handlers ─────────────────────────────────────────────────────────

func handleAttack(username string) {
	rpgMu.Lock()
	defer rpgMu.Unlock()
	if rpgState != StateBoss || activeBoss == nil {
		return
	}
	cd := effectiveCooldown()
	if last, ok := attackCooldowns[username]; ok && time.Since(last) < cd {
		return
	}
	attackCooldowns[username] = time.Now()
	attackCounts[username]++
	n := attackCounts[username]

	weapon := getWeapon(username)
	base := 10
	totalCC := critChance
	totalCM := critMultiplier
	if weapon != nil {
		base = weapon.Damage
		totalCC += weapon.CritChance
		totalCM += weapon.CritMult
	}

	// Apply active effects
	now := time.Now()
	dmgMult := 1.0
	for _, e := range activeEffects {
		if now.After(e.expires) {
			continue
		}
		switch e.kind {
		case "reduce_crit":
			totalCC -= int(e.value)
		case "reduce_damage":
			dmgMult *= e.value
		}
	}
	if totalCC < 0 {
		totalCC = 0
	}
	if totalCC > 95 {
		totalCC = 95
	}

	dmg := int(float64(base*n) * dmgMult)
	crit := rand.Intn(100) < totalCC
	if crit {
		dmg = int(float64(dmg) * totalCM)
	}

	activeBoss.HP -= dmg
	if activeBoss.HP < 0 {
		activeBoss.HP = 0
	}

	// Regen effect: boss heals fraction of damage dealt
	for _, e := range activeEffects {
		if e.kind == "regen" && now.Before(e.expires) {
			heal := int(float64(dmg) * e.value)
			activeBoss.HP += heal
			if activeBoss.HP > activeBoss.MaxHP {
				activeBoss.HP = activeBoss.MaxHP
			}
		}
	}

	bossAttackers[username] += dmg
	activeBoss.Attackers = buildAttackerList(bossAttackers)

	// Check HP-based phases
	hpPct := 100 * activeBoss.HP / activeBoss.MaxHP
	for i := range activeBossPhases {
		p := &activeBossPhases[i]
		if !p.fired && p.hpPct > 0 && hpPct <= p.hpPct {
			p.fired = true
			triggerBossEffect(p.effect)
		}
	}

	broadcastOverlay(OverlayState{State: "boss", Boss: activeBoss})

	if crit {
		rpgSay(fmt.Sprintf("🎯 ¡CRÍTICO! @%s (ataque #%d) → -%d HP a %s", username, n, dmg, activeBoss.Name))
	}

	if activeBoss.HP <= 0 {
		resolveBossLocked()
	}
}

func handleJoinDungeon(username string) {
	rpgMu.Lock()
	defer rpgMu.Unlock()
	if rpgState != StateDungeon || dungeonParty[username] {
		return
	}
	dungeonParty[username] = true
	dungeonWeapons[username] = getWeaponDamage(username)
	activeDungeon.Party = dungeonPartyList()
	broadcastOverlay(OverlayState{State: "dungeon", Dungeon: activeDungeon})
}

func handleOpenChest(username string) {
	rpgMu.Lock()
	defer rpgMu.Unlock()
	if rpgState != StateChest || chestOpeners[username] {
		return
	}
	chestOpeners[username] = true

	var msg string
	if rand.Intn(100) < chestTreasurePct {
		addPoints(username, chestTreasureAmt)
		msg = fmt.Sprintf("@%s abrió el cofre: ¡TESORO! +%d lattepoints ☕", username, chestTreasureAmt)
	} else {
		spendPoints(username, chestMimicAmt)
		msg = fmt.Sprintf("@%s abrió el cofre: ¡MÍMICO! -%d lattepoints 💀", username, chestMimicAmt)
	}
	rpgSay(msg)
}

// ─── Resolution ───────────────────────────────────────────────────────────────

func resolveBossLocked() {
	// called with rpgMu held, boss HP == 0
	total := 0
	for _, d := range bossAttackers {
		total += d
	}
	rewards := make(map[string]int)
	if total > 0 {
		for u, d := range bossAttackers {
			if share := (d * bossRewardPool) / total; share > 0 {
				rewards[u] = share
			}
		}
		bulkAddPoints(rewards)
	}
	rpgState = StateIdle
	activeBoss = nil
	bossAttackers = nil
	activeBossPhases = nil
	activeEffects = nil
	broadcastOverlay(OverlayState{State: "idle"})

	if len(rewards) == 0 {
		rpgSay(bossEscapeMsg)
		return
	}
	parts := make([]string, 0, len(rewards))
	for u, r := range rewards {
		parts = append(parts, fmt.Sprintf("%s +%d", u, r))
	}
	rpgSay(fmt.Sprintf("%s | Recompensas: %s", bossVictoryMsg, strings.Join(parts, " | ")))
}

func resolveDungeon() {
	rpgMu.Lock()
	defer rpgMu.Unlock()
	if rpgState != StateDungeon || activeDungeon == nil {
		return
	}
	resist := currentDungeonResist
	totalPower := 0
	for _, d := range dungeonWeapons {
		totalPower += d
	}

	var msg string
	if totalPower >= resist {
		rewards := make(map[string]int, len(dungeonParty))
		for u := range dungeonParty {
			rewards[u] = dungeonReward
		}
		bulkAddPoints(rewards)
		msg = fmt.Sprintf("%s (poder:%d vs resist:%d) — %d aventurer@s reciben +%d ☕",
			dungeonVictory, totalPower, resist, len(rewards), dungeonReward)
	} else {
		msg = fmt.Sprintf("%s (poder:%d vs resist:%d)",
			dungeonDefeat, totalPower, resist)
	}

	rpgState = StateIdle
	activeDungeon = nil
	dungeonParty = nil
	dungeonWeapons = nil
	broadcastOverlay(OverlayState{State: "idle"})
	rpgSay(msg)
}

// ─── Shop ─────────────────────────────────────────────────────────────────────

func handleShop() string {
	weapons := getAllWeapons()
	if len(weapons) == 0 {
		return "⚔️ Tienda vacía."
	}
	parts := make([]string, 0, len(weapons))
	for _, w := range weapons {
		info := fmt.Sprintf("%s dmg:%d", w.Name, w.Damage)
		if w.CritChance > 0 {
			info += fmt.Sprintf(" cc:+%d%%", w.CritChance)
		}
		if w.CritMult > 0 {
			info += fmt.Sprintf(" cx:+%.1f", w.CritMult)
		}
		info += fmt.Sprintf(" [%dpts]", w.Price)
		parts = append(parts, info)
	}
	return "⚔️ Tienda: " + strings.Join(parts, " | ") + " — !comprar <arma>"
}

func handleBuy(parts []string, username string) string {
	if len(parts) < 2 {
		return "Uso: !comprar <arma>"
	}
	weaponName := strings.ToLower(parts[1])
	cost, err := buyWeapon(username, weaponName)
	if err != nil {
		return fmt.Sprintf("@%s %v", username, err)
	}
	return fmt.Sprintf("@%s compró %s por %d lattepoints ⚔️", username, weaponName, cost)
}

func handleEquip(parts []string, username string) string {
	if len(parts) < 2 {
		return "Uso: !equipar <arma>"
	}
	weaponName := strings.ToLower(parts[1])
	if err := equipWeapon(username, weaponName); err != nil {
		return fmt.Sprintf("@%s %v", username, err)
	}
	return fmt.Sprintf("@%s equipó %s ⚔️", username, weaponName)
}

func handleInventory(username string) string {
	weapons := getInventory(username)
	equipped := getWeapon(username)
	if len(weapons) == 0 {
		return fmt.Sprintf("@%s inventario vacío. Usa !tienda para ver armas.", username)
	}
	parts := make([]string, 0, len(weapons))
	for _, w := range weapons {
		mark := ""
		if equipped != nil && w.ID == equipped.ID {
			mark = "★"
		}
		parts = append(parts, fmt.Sprintf("%s%s(dmg:%d)", mark, w.Name, w.Damage))
	}
	return fmt.Sprintf("@%s inventario: %s (★=equipada)", username, strings.Join(parts, " | "))
}

// ─── Helpers ──────────────────────────────────────────────────────────────────


func triggerBossEffect(e BossEffect) {
	// called with rpgMu held
	activeEffects = append(activeEffects, activeEffect{
		kind:    e.kind,
		value:   e.value,
		expires: time.Now().Add(e.duration),
	})
	rpgSay(e.startMsg)
	if e.endMsg != "" {
		time.AfterFunc(e.duration, func() {
			rpgMu.Lock()
			defer rpgMu.Unlock()
			if rpgState == StateBoss {
				rpgSay(e.endMsg)
			}
		})
	}
}

func effectiveCooldown() time.Duration {
	cd := attackCooldown
	now := time.Now()
	for _, e := range activeEffects {
		if e.kind == "increase_cooldown" && now.Before(e.expires) {
			cd = time.Duration(float64(cd) * e.value)
			break
		}
	}
	return cd
}

func buildAttackerList(attackers map[string]int) []Attacker {
	list := make([]Attacker, 0, len(attackers))
	for u, d := range attackers {
		list = append(list, Attacker{Username: u, Damage: d})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Damage > list[j].Damage
	})
	if len(list) > 5 {
		list = list[:5]
	}
	return list
}

func dungeonPartyList() []string {
	party := make([]string, 0, len(dungeonParty))
	for u := range dungeonParty {
		party = append(party, u)
	}
	return party
}
