# Especificación Técnica — violentocoffee-bot

## Índice

1. [Estructura del proyecto](#estructura)
2. [Arquitectura general](#arquitectura)
3. [Subsistemas](#subsistemas)
   - [IRC / Comandos de chat](#irc--comandos-de-chat)
   - [EventSub — Channel Points y Follows](#eventsub--channel-points-y-follows)
   - [Audio](#audio)
   - [Spotify](#spotify)
   - [Wordle](#wordle)
   - [Lattepoints y Sonidos](#lattepoints-y-sonidos)
   - [3DS Messenger](#3ds-messenger)
   - [Base de datos SQLite](#base-de-datos-sqlite)
   - [Sistema RPG](#sistema-rpg)
   - [Overlay WebSocket](#overlay-websocket)
4. [Persistencia](#persistencia)
5. [Variables de entorno](#variables-de-entorno)
6. [App homebrew 3DS](#app-homebrew-3ds)
7. [Dependencias](#dependencias)

---

## Estructura

```
violentocoffee-bot/
├── cmd/
│   ├── bot/                 # Bot principal — package main
│   │   ├── main.go          # Punto de entrada, router IRC
│   │   ├── db.go            # SQLite: schema, migración, API de puntos y armas
│   │   ├── rpg.go           # Sistema RPG completo (jefes, mazmorras, cofres, tienda)
│   │   ├── overlay.go       # Servidor HTTP/WebSocket + tipos de estado
│   │   ├── eventsub.go      # Cliente EventSub WebSocket
│   │   ├── wordle.go        # Lógica del juego Wordle
│   │   ├── lattepoints.go   # Comandos !points y !sound, constantes de puntos
│   │   ├── sounds.go        # Motor de reproducción de audio
│   │   ├── rewards.go       # Carga rewards.json
│   │   ├── song.go          # Integración Spotify API
│   │   ├── send3ds.go       # Comando !3ds (UDP hacia la 3DS)
│   │   ├── social.go        # Comando !social
│   │   ├── setup.go         # Comando !setup
│   │   ├── help.go          # Comando !help
│   │   ├── main_test.go     # TestMain + setup de DB en memoria
│   │   ├── db_test.go       # Tests de puntos y armas
│   │   ├── rpg_test.go      # Tests de lógica RPG
│   │   └── wordle_test.go   # Tests de Wordle
│   ├── gettoken/            # Herramienta: obtener token OAuth via browser
│   │   └── main.go
│   └── listrewards/         # Herramienta: listar reward IDs de Channel Points
│       └── main.go
│
├── web/                     # Overlay frontend (OBS Browser Source)
│   └── index.html
│
├── assets/
│   ├── audios/              # Archivos MP3 para reproducción con !sound
│   ├── fonts/               # Fuente DungeonMode TTF (servida por overlay)
│   └── dungeonmode/         # Kit completo de la fuente (referencia)
│
├── docs/
│   └── spec.md              # Esta especificación técnica
│
├── 3ds-app/                 # App homebrew para Nintendo 3DS
│   ├── Makefile
│   └── source/main.c
│
├── violentocoffee.db        # SQLite (creado en primer arranque)
├── rewards.json             # Mapeo reward ID → ruta de audio
├── sound_prices.json        # Precio por sonido (opcional)
├── wordle.json              # Estado persistente del Wordle
├── lattepoints.json         # Backup de migración a SQLite
├── .env                     # Variables de entorno (no versionar)
├── .env.example             # Plantilla de variables de entorno
└── README.md
```

---

## Arquitectura

El bot corre como un proceso Go único con múltiples goroutines:

```
┌────────────────────────────────────────────────────────────────┐
│                           main.go                              │
│                                                                │
│  initDB() → initSpeaker() → loadAudioRewards()                 │
│  startOverlayServer() → startEventSub() → startRPGEventLoop()  │
│                                                                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │  IRC client  │  │  EventSub    │  │  RPG event loop      │  │
│  │  (blocking)  │  │  goroutine   │  │  goroutine (10 min)  │  │
│  └──────┬───────┘  └──────┬───────┘  └──────────────────────┘  │
│         │                 │                                     │
│  OnPrivateMessage   onRedemption()                              │
│         │           onFollow()                                  │
│    command switch                                               │
│         │                                                       │
│  ┌──────┴───────────────────────────────────────────────────┐   │
│  │                  violentocoffee.db (SQLite)               │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │   Overlay HTTP :7373  (WebSocket /ws + static /overlay)  │   │
│  └──────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────┘
```

- **IRC loop**: bloqueante en `client.Connect()`. Cada mensaje que empieza con `!` se enruta al handler correspondiente.
- **EventSub goroutine**: conecta via WebSocket a `wss://eventsub.wss.twitch.tv/ws`, se reconecta automáticamente cada 5 segundos ante desconexión.
- **RPG event loop**: goroutine que duerme 10 minutos; si el estado es `StateIdle`, lanza un evento aleatorio.
- La reproducción de audio siempre ocurre en su propia goroutine (no bloquea el loop principal).
- El overlay se comunica con el resto del sistema a través de `broadcastOverlay()` — cualquier cambio de estado RPG o de puntos llama a esta función.

---

## Subsistemas

### IRC / Comandos de chat

**Archivo:** `main.go`

El cliente usa `go-twitch-irc`. Cada `PrivateMessage` se filtra:

1. Si no empieza con `!` → ignorar
2. Split por espacios → `parts[0]` es el comando en minúsculas
3. Switch sobre el comando → llama al handler

```
OnPrivateMessage
  └── strings.HasPrefix(text, "!")
        └── switch command
              ├── !song          → handleSong()
              ├── !setup         → handleSetup()
              ├── !social        → handleSocial()
              ├── !wordle        → handleWordle(client, channel, msg)
              ├── !help          → handleHelp(client, channel)
              ├── !3ds           → handleSend3DS(parts, username)
              ├── !points / !lattepoints → handlePoints(username)
              ├── !sound         → handleSound(parts, username)
              ├── !ovtest        → handleOverlayTest(parts)  [solo broadcaster]
              ├── !evento        → handleManualEvent(parts)  [solo broadcaster]
              ├── !atacar / !attack   → handleAttack(username)
              ├── !dungeon / !mazmorra → handleJoinDungeon(username)
              ├── !cofre / !abrir     → handleOpenChest(username)
              ├── !tienda / !shop     → handleShop()
              ├── !comprar / !buy     → handleBuy(parts, username)
              ├── !equipar / !equip   → handleEquip(parts, username)
              └── !inventario / !inv  → handleInventory(username)
```

Todos los handlers simples retornan `string` y el caller hace `client.Say()`. Los handlers que necesitan enviar múltiples mensajes (wordle, help) reciben `client` directamente.

---

### EventSub — Channel Points y Follows

**Archivo:** `eventsub.go`

Flujo de conexión:

```
startEventSub()
  └── goroutine: loop infinito
        └── runEventSub()
              ├── WebSocket dial → wss://eventsub.wss.twitch.tv/ws
              ├── session_welcome → subscribeChannelPoints()
              │     └── POST /helix/eventsub/subscriptions
              │           type: channel.channel_points_custom_reward_redemption.add
              │           version: "1"
              │           condition: { broadcaster_user_id }
              │
              │                subscribeFollow()
              │     └── POST /helix/eventsub/subscriptions
              │           type: channel.follow
              │           version: "2"
              │           condition: { broadcaster_user_id, moderator_user_id }
              │           (requiere scope moderator:read:followers)
              │
              ├── session_keepalive → no-op
              ├── session_reconnect → return error (trigger reconexión)
              ├── notification
              │     ├── channel.channel_points_custom_reward_redemption.add
              │     │     └── onRedemption(rewardID, username) → playAudio()
              │     └── channel.follow
              │           └── onFollow(userLogin) → addPoints(+100) + Say()
              └── revocation → log warning
```

El broadcaster ID se obtiene una sola vez al inicio via `GET /helix/users?login=<channel>`. Si `subscribeFollow` falla (token sin scope `moderator:read:followers`), se registra el error pero el bot continúa sin seguimiento de follows.

---

### Audio

**Archivo:** `sounds.go`

Motor basado en `faiface/beep`:

- `initSpeaker()` — inicializa el speaker a 44100 Hz con buffer de 100ms
- `playAudio(path)` — decodifica MP3 y reproduce en goroutine propia; si el sample rate difiere de 44100 Hz, resamplea con calidad 3

Toda reproducción ocurre en goroutine propia — el caller no bloquea.

**Archivo:** `rewards.go`

Carga `rewards.json` al inicio: `map[rewardID]audioFilePath`. Cuando EventSub notifica un redemption, se busca el `rewardID` en este mapa y se llama `playAudio()`.

---

### Spotify

**Archivo:** `song.go`

Flujo del comando `!song`:

```
handleSong()
  └── getSpotifyAccessToken()
        └── POST https://accounts.spotify.com/api/token
              grant_type=refresh_token
              Basic Auth: client_id:client_secret
  └── GET https://api.spotify.com/v1/me/player/currently-playing
        Authorization: Bearer <access_token>
  └── HTTP 204 → "No hay canción reproduciéndose"
      JSON → "🎵 {nombre} — {artistas}"
```

Timeout del cliente HTTP: 5 segundos. El access token se obtiene en cada llamada (no se cachea), usando el refresh token del `.env`.

---

### Wordle

**Archivo:** `wordle.go`
**Estado:** `wordle.json`

```json
{
  "current_word": "SALON",
  "date": "2026-05-07",
  "winner": "usuario",
  "winner_at": "...",
  "winner_word": "SALON",
  "last_winner": "...",
  "last_winner_at": "...",
  "last_winner_word": "..."
}
```

**Comportamiento:**

- La palabra viene de `WORDLE_WORD` en `.env`
- Al inicio de cada invocación, si `current_word` normalizado difiere de `WORDLE_WORD` normalizado, se resetea el juego preservando el último ganador (`resetIfNewWord`)
- `normalizeWord()` convierte a mayúsculas y reemplaza vocales con tilde por su equivalente sin tilde (á→A, é→E, í→I, ó→O, ú/ü→U)
- `evaluateWordle()` aplica lógica estándar del Wordle: verde (posición exacta) → amarillo (letra existe, posición incorrecta) → negro

**Al ganar:**
- Guarda ganador en `wordle.json`
- Llama `addPoints(username, 500)` → +500 lattepoints
- Responde con emoji feedback + mensaje de victoria

---

### Lattepoints y Sonidos

**Archivo:** `lattepoints.go`

**Constantes:**

| Constante | Valor | Uso |
|---|---|---|
| `wordleWinPoints` | 500 | Puntos por ganar el Wordle |
| `defaultSoundCost` | 25 | Costo de `!sound` sin entrada en sound_prices.json |

**Precios de sonidos:**

`soundPrice(name)` carga `sound_prices.json` en cada llamada. Retorna el precio configurado para ese nombre, o `defaultSoundCost` si no existe entrada.

**Comando `!sound`:**

1. Lee `assets/audios/` con `os.ReadDir`
2. Construye `map[nombre_lowercase]path`
3. Sin argumento → lista sonidos disponibles con costo individual
4. Con argumento → busca por nombre (case-insensitive), verifica saldo, descuenta, reproduce

---

### 3DS Messenger

**Archivo:** `send3ds.go`
**Protocolo:** UDP con ACK

```
Bot                          3DS app
 │                               │
 ├──── UDP "@user: mensaje" ────►│
 │                               ├── recvfrom()
 │                               ├── sendto("OK")
 │◄─────────── "OK" ─────────────┤
 │                               │
 │  (timeout 1s si no responde)  │
```

- Deadline de 1 segundo en el `UDPConn`
- Si Write falla o Read hace timeout → `"[3DS] no disponible"`
- Mensaje formato: `@username: <texto>`, máx 200 bytes
- IP y puerto configurables via `DS3_IP` / `DS3_PORT` en `.env`

---

### Base de datos SQLite

**Archivo:** `db.go`
**Motor:** `modernc.org/sqlite` (CGO-free)

**Schema:**

```sql
CREATE TABLE users (
    username  TEXT PRIMARY KEY,
    points    INTEGER NOT NULL DEFAULT 0,
    weapon_id INTEGER REFERENCES weapons(id)
);

CREATE TABLE weapons (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL UNIQUE,
    damage      INTEGER NOT NULL,
    price       INTEGER NOT NULL,
    crit_chance INTEGER NOT NULL DEFAULT 0,
    crit_mult   REAL    NOT NULL DEFAULT 0.0
);

CREATE TABLE inventory (
    username  TEXT    NOT NULL,
    weapon_id INTEGER NOT NULL,
    PRIMARY KEY (username, weapon_id)
);
```

Pragmas: `journal_mode=WAL`, `foreign_keys=ON`. Conexión única (`SetMaxOpenConns(1)`).

**Inicialización (`initDB`):**

1. Abre `violentocoffee.db`
2. Ejecuta los `CREATE TABLE IF NOT EXISTS`
3. Aplica `ALTER TABLE` para añadir columnas a bases existentes (no-op si ya existen)
4. `seedWeapons()` — upsert de las 12 armas del catálogo
5. `migrateLattepoints()` — si existe `lattepoints.json`, inserta usuarios con `INSERT OR IGNORE`

**API interna de puntos:**

```go
getPoints(username) int
addPoints(username, amount)            // upsert + refreshLeaderboard()
spendPoints(username, amount) bool     // transacción; false si saldo insuficiente
bulkAddPoints(rewards map[string]int)  // transacción batch para recompensas de RPG
topPoints(n int) []TopEntry            // top N, excluye al broadcaster
```

**API interna de armas:**

```go
getAllWeapons() []Weapon               // ordenadas por precio
getInventory(username) []Weapon        // join inventory + weapons
getWeapon(username) *Weapon            // arma actualmente equipada
getWeaponDamage(username) int          // daño del arma equipada (10 si ninguna)
buyWeapon(username, name) (cost, err)  // verifica duplicado en inv, descuenta puntos
equipWeapon(username, name) error      // requiere que esté en el inventario
```

---

### Sistema RPG

**Archivo:** `rpg.go`

#### Máquina de estados

```
                ┌────────────┐
           ┌───►│ StateIdle  │◄───────────────┐
           │    └─────┬──────┘                │
           │          │ timer 10min / !evento  │
           │          ▼                        │
           │    ┌────────────────────────────┐  │
           │    │      rollAndTrigger()       │  │
           │    │  50% boss / 30% dungeon     │  │
           │    │  20% chest                 │  │
           │    └──┬──────────┬──────┬───────┘  │
           │       │          │      │          │
           │       ▼          │      ▼          │
           │  StateBoss        │  StateChest    │
           │  !atacar          │  !cofre        │
           │  HP=0 → victoria  │  timer 30s     │
           │    └──────────────┼────────────────┘
           │                   ▼
           │             StateDungeon
           │             !dungeon (60s)
           │             resolve → victoria/derrota
           └──────────────────────────────────────┘
```

**Variables globales protegidas por `rpgMu sync.Mutex`:**

| Variable | Tipo | Descripción |
|---|---|---|
| `rpgState` | `RPGState` | Estado actual de la máquina |
| `activeBoss` | `*BossInfo` | Jefe activo (nil si no hay) |
| `bossAttackers` | `map[string]int` | username → daño acumulado |
| `attackCooldowns` | `map[string]time.Time` | username → último ataque |
| `attackCounts` | `map[string]int` | username → número de ataques |
| `activeBossPhases` | `[]activeBossPhase` | Fases del jefe pendientes/disparadas |
| `activeEffects` | `[]activeEffect` | Efectos activos con tiempo de expiración |
| `activeDungeon` | `*DungeonInfo` | Dungeon activo |
| `dungeonParty` | `map[string]bool` | Jugadores unidos |
| `dungeonWeapons` | `map[string]int` | Daño del arma al momento de unirse |
| `chestOpeners` | `map[string]bool` | Usuarios que ya abrieron el cofre |

#### Sistema de fases de jefe

```go
type BossPhase struct {
    hpPct   int           // se activa cuando HP cae a este % (0 = no HP-based)
    timeSec int           // se activa después de N segundos (0 = no time-based)
    effect  BossEffect
}

type BossEffect struct {
    kind     string        // "reduce_crit" | "increase_cooldown" | "reduce_damage" | "regen"
    value    float64       // semántica según kind:
                          //   reduce_crit: puntos porcentuales a restar
                          //   increase_cooldown: multiplicador del cooldown base
                          //   reduce_damage: fracción a aplicar al daño (0.5 = 50% del daño)
                          //   regen: fracción del daño recibido que se cura
    duration time.Duration
    startMsg string
    endMsg   string
}
```

Fases basadas en tiempo: se disparan con `time.AfterFunc` al momento de crear el jefe.
Fases basadas en HP: se verifican en cada `handleAttack` tras calcular el nuevo HP%.

Jefes con fases especiales:

| Jefe | Fase | Trigger | Efecto |
|---|---|---|---|
| Morgott | increase_cooldown ×2.0 | HP ≤ 60% | Cooldown sube a 60s durante 50s |
| Graf Vlad | reduce_crit −15% | 60s | Crit chance −15% durante 35s |
| Malenia | regen 12% | HP ≤ 50% | Se cura 12% del daño recibido, permanente |
| Señor de la Blasfemia | regen 8% | 80s | Se cura 8% del daño recibido, permanente |
| Gran Bestia Antigua | reduce_damage 50% | 45s | Daño infligido reducido al 50% durante 40s |
| Arzobispo Herético | reduce_crit −20% | HP ≤ 55% | Crit chance −20% durante 50s |
| Radahn | increase_cooldown ×1.8 | HP ≤ 50% | Cooldown sube a 54s durante 55s |
| Devorador de Mundos | reduce_damage 40% + reduce_crit −25% | HP ≤ 50% y HP ≤ 25% | Dos fases progresivas |

#### Cálculo de daño en ataque

```
n = attackCounts[username]  (número de ataque, empieza en 1)
base = weapon.Damage  (o 10 si sin arma)
totalCC = 5 + weapon.CritChance  (clamped a [0, 95])
totalCM = 2.0 + weapon.CritMult

Aplicar efectos activos:
  reduce_crit → totalCC -= value
  reduce_damage → dmgMult *= value

dmg = int(float64(base * n) * dmgMult)

Si rand(100) < totalCC:
  dmg = int(float64(dmg) * totalCM)  // crítico

Aplicar regen del jefe:
  boss.HP += int(float64(dmg) * regenValue)
```

#### Resolución de jefe

Cuando `boss.HP <= 0`, `resolveBossLocked()` (llamada con `rpgMu` retenido):

1. Suma el daño total de todos los atacantes
2. Para cada atacante: `share = (daño_atacante * 500) / total`
3. `bulkAddPoints(rewards)` — una sola transacción
4. Announce en chat + broadcast overlay a `StateIdle`

#### Resolución de dungeon

`resolveDungeon()` se llama con `time.AfterFunc(60s)`:

1. `totalPower = sum(dungeonWeapons)`
2. `currentDungeonResist = resist + rand(resist/2+1)` (resist base del catálogo)
3. Si `totalPower >= resist`: `bulkAddPoints` con 150 pts por participante
4. Announce en chat + broadcast overlay a `StateIdle`

#### Resolución de cofre

Cada `!cofre` se resuelve inmediatamente en `handleOpenChest`:
- 70%: `addPoints(+200)`, mensaje en chat
- 30%: `spendPoints(100)`, mensaje en chat

Al expirar el timer: si `rpgState == StateChest`, vuelve a `StateIdle`.

---

### Overlay WebSocket

**Archivos:** `cmd/bot/overlay.go`, `web/index.html`

#### Servidor

```
startOverlayServer()
  └── http.ListenAndServe(:7373)
        ├── GET /ws        → overlayWSHandler (WebSocket upgrade)
        ├── GET /font/*    → FileServer("assets/fonts/")
        └── GET /*         → FileServer("web/")
```

#### Protocolo WebSocket

Mensajes unidireccionales del servidor al cliente (JSON). El cliente envía mensajes vacíos solo para keepalive de la conexión.

**Tipo `OverlayState`:**

```go
type OverlayState struct {
    State   string       `json:"state"`   // "idle" | "boss" | "dungeon" | "chest"
    Boss    *BossInfo    `json:"boss,omitempty"`
    Dungeon *DungeonInfo `json:"dungeon,omitempty"`
    Chest   *ChestInfo   `json:"chest,omitempty"`
    Top     []TopEntry   `json:"top,omitempty"`  // solo en state=idle
}

type BossInfo struct {
    Name      string     `json:"name"`
    HP        int        `json:"hp"`
    MaxHP     int        `json:"maxHp"`
    Attackers []Attacker `json:"attackers"`  // top 5 por daño
}

type DungeonInfo struct {
    Name        string   `json:"name"`
    SecondsLeft int      `json:"secondsLeft"`
    Party       []string `json:"party"`
}

type ChestInfo struct {
    SecondsLeft int    `json:"secondsLeft"`
    Username    string `json:"username"`
    Result      string `json:"result"`   // "" | "treasure" | "mimic"
    Amount      int    `json:"amount"`
}

type TopEntry struct {
    Username string `json:"username"`
    Points   int    `json:"points"`
}
```

#### Flujo de broadcast

```
broadcastOverlay(state)
  ├── Si state == "idle": state.Top = topPoints(3)
  ├── overlayCurrentState = state
  └── Para cada conn en overlayClients:
        conn.WriteMessage(TextMessage, json.Marshal(state))
        si error → close + delete
```

Al conectar un nuevo cliente, se le envía el `overlayCurrentState` actual inmediatamente.

`refreshLeaderboard()` — llamada desde `addPoints` y `spendPoints` — dispara `broadcastOverlay(idle)` solo si el estado actual es idle, para mantener el leaderboard actualizado sin interrumpir eventos RPG activos.

#### Frontend (`web/index.html`)

- Auto-reconexión WebSocket cada 2 segundos ante desconexión
- Fondo transparente 1920×1080
- 4 paneles controlados por CSS visibility + el campo `state` del mensaje
- Timer bars para dungeon (60s) y chest (30s): animación CSS `scaleX` con color progresivo verde→amarillo→rojo
- HP bar del jefe: color dependiente del porcentaje (`hp / maxHp`)
- Top 3 leaderboard: se actualiza en cada mensaje con `state=idle`

---

## Persistencia

| Archivo | Formato | Contenido |
|---|---|---|
| `violentocoffee.db` | SQLite | Tablas: `users`, `weapons`, `inventory` |
| `wordle.json` | JSON | Estado del wordle activo + último ganador |
| `rewards.json` | JSON | Mapa `rewardID → ruta audio` |
| `sound_prices.json` | JSON | Mapa `nombre_sonido → precio` (opcional) |
| `lattepoints.json` | JSON | Origen de migración a SQLite (solo lectura tras migrar) |

---

## Variables de entorno

| Variable | Requerida | Default | Descripción |
|---|---|---|---|
| `TWITCH_BOT_USERNAME` | ✅ | — | Username del bot |
| `TWITCH_OAUTH_TOKEN` | ✅ | — | Token IRC del bot (`oauth:...`) |
| `TWITCH_CHANNEL` | ✅ | — | Canal a monitorear (también = broadcaster) |
| `TWITCH_CLIENT_ID` | ⚠️ | — | Para EventSub (Channel Points + follows) |
| `TWITCH_BROADCASTER_TOKEN` | ⚠️ | fallback a `TWITCH_OAUTH_TOKEN` sin `oauth:` | Token del broadcaster |
| `SPOTIFY_CLIENT_ID` | ⚠️ | — | Para `!song` |
| `SPOTIFY_CLIENT_SECRET` | ⚠️ | — | Para `!song` |
| `SPOTIFY_REFRESH_TOKEN` | ⚠️ | — | Para `!song` |
| `WORDLE_WORD` | ⚠️ | — | Palabra activa del Wordle |
| `DS3_IP` | ⚠️ | — | IP local de la 3DS |
| `DS3_PORT` | ❌ | `8888` | Puerto UDP de la 3DS |
| `OVERLAY_PORT` | ❌ | `7373` | Puerto HTTP del overlay |

⚠️ Opcional pero necesaria para el subsistema correspondiente.

---

## App homebrew 3DS

**Directorio:** `3ds-app/`
**Lenguaje:** C (libctru)
**Compilador:** devkitARM (devkitPro)

### Funcionamiento

```
main()
  ├── gfxInitDefault()
  ├── consoleInit(GFX_TOP)    ← mensajes
  ├── consoleInit(GFX_BOTTOM) ← IP + estado
  ├── socInit()               ← servicio de red de la 3DS
  ├── socket(AF_INET, SOCK_DGRAM)
  ├── fcntl(O_NONBLOCK)       ← no bloquea el loop
  ├── bind(INADDR_ANY, 8888)
  ├── gethostid() → mostrar IP en pantalla inferior
  └── loop:
        ├── hidScanInput() → START = salir
        ├── recvfrom() (no bloqueante)
        │     └── mensaje recibido:
        │           ├── sendto("OK") ← ACK al bot
        │           ├── push_message(buf)
        │           └── redraw pantalla superior
        └── gspWaitForVBlank()
```

### Pantallas

- **Superior (GFX_TOP):** historial de mensajes (máx 18, scroll automático eliminando el más antiguo)
- **Inferior (GFX_BOTTOM):** IP de la 3DS, puerto, estado

### Buffer SOC

La 3DS requiere un buffer alineado a 0x1000 de tamaño mínimo 0x100000 (1 MB) para el servicio de red (SOC). Se aloca con `memalign` y se pasa a `socInit`.

### Compilar (Windows + MSYS2)

```bash
# En MSYS2 shell con devkitPro instalado
export DEVKITARM=/c/devkitPro/devkitARM
export CTRULIB=/c/devkitPro/libctru

cd /c/Users/nicolas/development/violentocoffee-bot/3ds-app
make

# Salida: 3ds-app.3dsx
# Copiar a SD: /3ds/3ds-app/3ds-app.3dsx
```

---

## Dependencias

```
github.com/gempir/go-twitch-irc/v4  — cliente IRC de Twitch
github.com/gorilla/websocket         — WebSocket para EventSub y overlay
github.com/faiface/beep              — motor de audio
github.com/hajimehoshi/go-mp3        — decodificador MP3
github.com/hajimehoshi/oto           — backend de audio del SO
github.com/joho/godotenv             — carga de .env
modernc.org/sqlite                   — SQLite puro Go (sin CGO)
```
