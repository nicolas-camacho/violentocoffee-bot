# violentocoffee-bot

Bot de Twitch para el canal [violentocoffee](https://twitch.tv/violentocoffee), escrito en Go.

## Características

- Comandos de chat: música, setup, redes sociales, Wordle, sonidos, 3DS
- Sistema de puntos propio: **lattepoints** ☕ (persistido en SQLite)
- RPG en vivo: peleas de jefes, raids de mazmorra y cofres mágicos
- Tienda de armas con estadísticas de daño y crítico
- Overlay en tiempo real para OBS (servidor WebSocket en puerto 7373)
- Reproducción de audio por redención de Channel Points
- Eventos automáticos de seguidor: +100 lattepoints + mensaje de bienvenida

## Requisitos

- Go 1.21+
- Cuenta de Twitch (bot + broadcaster)
- Spotify Developer App (para `!song`)
- 3DS con Homebrew Launcher (opcional, para `!3ds`)

## Instalación

```bash
git clone https://github.com/nicolasrmq/violentocoffee-bot
cd violentocoffee-bot
cp .env.example .env
# editar .env con tus credenciales
go run ./cmd/bot/
```

El bot crea la base de datos `violentocoffee.db` automáticamente en el primer arranque. Si existe `lattepoints.json`, migra los puntos a SQLite.

## Configuración

Copiar `.env.example` a `.env` y rellenar:

| Variable | Requerida | Descripción |
|---|---|---|
| `TWITCH_BOT_USERNAME` | Sí | Username de la cuenta bot |
| `TWITCH_OAUTH_TOKEN` | Sí | Token OAuth del bot (`oauth:...`) |
| `TWITCH_CHANNEL` | Sí | Canal a monitorear |
| `TWITCH_CLIENT_ID` | EventSub | Client ID de la app Twitch |
| `TWITCH_BROADCASTER_TOKEN` | EventSub | Token del broadcaster (Channel Points + follows) |
| `SPOTIFY_CLIENT_ID` | `!song` | Client ID de Spotify |
| `SPOTIFY_CLIENT_SECRET` | `!song` | Client Secret de Spotify |
| `SPOTIFY_REFRESH_TOKEN` | `!song` | Refresh token de Spotify |
| `WORDLE_WORD` | `!wordle` | Palabra activa del Wordle |
| `DS3_IP` | `!3ds` | IP local de la 3DS |
| `DS3_PORT` | No (default: `8888`) | Puerto UDP de la 3DS |
| `OVERLAY_PORT` | No (default: `7373`) | Puerto HTTP del overlay |

## Comandos de chat

### Generales

| Comando | Descripción |
|---|---|
| `!song` | Canción reproduciéndose en Spotify |
| `!setup` | Especificaciones del PC |
| `!social` | Redes sociales |
| `!help` | Lista de comandos |
| `!points` / `!lattepoints` | Ver tus lattepoints |
| `!3ds <mensaje>` | Enviar mensaje a la Nintendo 3DS |

### Wordle

| Comando | Descripción |
|---|---|
| `!wordle` | Estado actual: longitud de la palabra, último ganador |
| `!wordle <palabra>` | Adivinar la palabra (+500 lattepoints al ganar) |

La palabra cambia cuando el broadcaster actualiza `WORDLE_WORD` en `.env` y reinicia el bot. Al cambiar, el ganador anterior pasa a "último ganador" y el juego se reinicia. Las tildes se normalizan: `SALON` = `SALÓN`.

### Sonidos

| Comando | Descripción |
|---|---|
| `!sound` | Listar sonidos disponibles con su costo |
| `!sound <nombre>` | Reproducir sonido (costo en `sound_prices.json`, default 25 pts) |

### RPG — Tienda de armas

| Comando | Descripción |
|---|---|
| `!tienda` / `!shop` | Ver todas las armas con estadísticas y precio |
| `!comprar <arma>` / `!buy <arma>` | Comprar arma con lattepoints |
| `!equipar <arma>` / `!equip <arma>` | Equipar arma del inventario |
| `!inventario` / `!inv` | Ver tu inventario (arma equipada marcada con ★) |

### RPG — Combate

| Comando | Descripción |
|---|---|
| `!atacar` / `!attack` | Atacar al jefe activo (cooldown 30s por usuario) |
| `!dungeon` / `!mazmorra` | Unirse a la raid de mazmorra activa (ventana de 60s) |
| `!cofre` / `!abrir` | Abrir el cofre activo (una vez por evento) |

### Comandos del broadcaster (solo el dueño del canal)

| Comando | Descripción |
|---|---|
| `!evento [boss\|dungeon\|cofre]` | Forzar evento RPG inmediatamente |
| `!ovtest [boss\|dungeon\|chest\|treasure\|mimic]` | Vista previa del overlay sin afectar el juego |

## Tabla de armas

Sin arma equipada el jugador hace 10 de daño base. Crit base: 5% de probabilidad, 2.0× multiplicador.

| Arma | Daño | Precio | CC bonus | CX bonus |
|---|---|---|---|---|
| palo | 5 | gratis | — | — |
| daga | 15 | 100 pts | +8% | — |
| espada | 25 | 200 pts | — | — |
| arco | 28 | 300 pts | +12% | — |
| hacha | 40 | 400 pts | — | +0.5× |
| ballesta | 33 | 450 pts | +8% | +0.3× |
| lanza | 45 | 500 pts | — | — |
| estilete | 18 | 350 pts | +25% | — |
| báculo | 50 | 600 pts | — | +1.0× |
| katana | 35 | 650 pts | +15% | +0.5× |
| maul | 65 | 900 pts | — | +0.8× |
| arco_larg | 42 | 800 pts | +18% | +0.5× |

CC bonus = porcentaje de crítico adicional al 5% base. CX bonus = multiplicador adicional al 2.0× base.

## Sistema RPG

Cada 10 minutos, cuando no hay evento activo, el bot lanza un evento aleatorio:

- **50%** — Pelea de jefe
- **30%** — Raid de mazmorra
- **20%** — Cofre mágico

El broadcaster puede forzar cualquier evento con `!evento`.

### Peleas de jefes

18 jefes con lore de Dark Souls, Bloodborne, Elden Ring, Castlevania y Trench Crusade.

- Escribe `!atacar` para golpear al jefe (cooldown de 30s por jugador)
- El daño escala con cada ataque del mismo jugador: `daño = arma × número_de_ataques`
- Crítico: `(5 + bonus_arma)%` de probabilidad, multiplica el daño por `(2.0 + bonus_arma)×`
- Pool de recompensa: **500 lattepoints** repartidos proporcionalmente al daño infligido
- Si nadie ataca antes de que el jefe escape, no hay recompensa

**8 jefes tienen fases especiales** que se activan por porcentaje de HP o por tiempo:

| Efecto | Descripción |
|---|---|
| `reduce_crit` | Reduce el chance crítico de todos los atacantes |
| `increase_cooldown` | Aumenta el cooldown de ataque (ej. Morgott lo duplica a 60s) |
| `reduce_damage` | Reduce el daño infligido en porcentaje |
| `regen` | El jefe se cura un porcentaje del daño recibido (ej. Malenia 12%) |

### Raids de mazmorra

12 mazmorras con resistencias de 200 a 900.

- Ventana de **60 segundos** para unirse con `!dungeon`
- Al cerrar la ventana, se suma el daño de todas las armas equipadas
- Si el poder total supera la resistencia: **+150 lattepoints** por participante
- Si no supera: sin recompensa

### Cofres mágicos

- Ventana de **30 segundos**, cada usuario puede abrir una vez con `!cofre`
- **70%** de probabilidad: tesoro (+200 lattepoints)
- **30%** de probabilidad: mímico (−100 lattepoints)
- El resultado se anuncia por usuario en chat

## Overlay para OBS

El bot levanta un servidor HTTP en `http://localhost:7373`. Agregar en OBS como Browser Source con resolución 1920×1080 y fondo transparente.

El overlay tiene 4 paneles que cambian automáticamente según el estado del RPG:

| Panel | Condición | Contenido |
|---|---|---|
| Idle | Sin evento | Top 3 lattepoints (centro superior) |
| Boss | Jefe activo | Barra de HP del jefe + top 5 atacantes por daño |
| Dungeon | Mazmorra activa | Lista del partido + barra de tiempo verde→amarillo→rojo |
| Chest | Cofre activo | Barra de tiempo verde→amarillo→rojo |

La fuente tipográfica es DungeonMode (pixel/retro). El leaderboard se actualiza en tiempo real con cada cambio de puntos.

## Channel Points (audio)

Agregar entradas en `rewards.json` mapeando el ID del reward de Twitch al archivo de audio:

```json
{
  "reward-uuid-aqui": "audios/shizar.mp3"
}
```

Los precios individuales por sonido se configuran en `sound_prices.json`:

```json
{
  "nombre_sonido": 50
}
```

Los sonidos sin entrada en `sound_prices.json` cuestan 25 lattepoints por defecto.

## Persistencia

| Archivo | Descripción |
|---|---|
| `violentocoffee.db` | SQLite: usuarios, puntos, armas, inventario |
| `wordle.json` | Estado del Wordle activo + historial de ganadores |
| `rewards.json` | Mapeo reward ID → ruta de audio |
| `sound_prices.json` | Precios por sonido (opcional) |
| `lattepoints.json` | Migrado a SQLite en el primer arranque (se conserva como backup) |

## App 3DS

Ver `3ds-app/` para el homebrew que recibe los mensajes. Requiere devkitPro con `3ds-dev`.

```bash
# Compilar (desde MSYS2 con devkitARM configurado)
cd 3ds-app
make
# Genera: 3ds-app.3dsx → copiar a /3ds/3ds-app/ en la SD
```

## Comandos útiles

```bash
# Ejecutar el bot
go run ./cmd/bot/

# Compilar binario
go build -o violentocoffee-bot ./cmd/bot/

# Ejecutar tests
go test ./cmd/bot/ -v

# Obtener token OAuth (browser)
go run ./cmd/gettoken/

# Listar reward IDs de Channel Points
go run ./cmd/listrewards/
```

## Estructura del proyecto

```
violentocoffee-bot/
├── cmd/
│   ├── bot/                 # Bot principal (source + tests)
│   │   ├── main.go          # Punto de entrada, router IRC
│   │   ├── db.go            # SQLite: puntos, armas, inventario
│   │   ├── rpg.go           # Sistema RPG: jefes, mazmorras, cofres
│   │   ├── overlay.go       # Servidor overlay WebSocket
│   │   ├── eventsub.go      # EventSub: Channel Points + follows
│   │   ├── wordle.go        # Juego Wordle
│   │   ├── lattepoints.go   # Economía: puntos y sonidos
│   │   ├── sounds.go        # Reproducción de audio MP3
│   │   ├── rewards.go       # Carga rewards.json
│   │   ├── song.go          # Spotify API
│   │   ├── send3ds.go       # Mensajes UDP a la 3DS
│   │   ├── social.go        # !social
│   │   ├── setup.go         # !setup
│   │   ├── help.go          # !help
│   │   └── *_test.go        # Tests unitarios
│   ├── gettoken/            # Herramienta: obtener OAuth token
│   └── listrewards/         # Herramienta: listar reward IDs
├── web/                     # Overlay frontend (OBS Browser Source)
│   └── index.html
├── assets/
│   ├── audios/              # Archivos MP3
│   └── fonts/               # Fuente DungeonMode TTF
├── docs/
│   └── spec.md              # Especificación técnica
├── 3ds-app/                 # Homebrew para Nintendo 3DS
├── .env.example             # Plantilla de variables de entorno
└── README.md
```
