package main

import (
	twitch "github.com/gempir/go-twitch-irc/v4"
)

var helpCommands = []string{
	"!song          → canción que se está escuchando en Spotify",
	"!setup         → componentes del PC",
	"!social        → redes sociales",
	"!wordle        → info de la palabra del día",
	"!wordle <word> → adivina la palabra del día",
	"!3ds <msg>     → envía mensaje a la 3DS",
	"!points        → ver tus lattepoints",
	"!sound <name>  → reproducir audio",
	"!tienda        → ver armas disponibles y precios",
	"!comprar <arm> → comprar un arma con lattepoints",
	"!equipar <arm> → equipar un arma de tu inventario",
	"!inventario    → ver tus armas",
	"!atacar        → atacar al jefe (cuando hay jefe activo)",
	"!dungeon       → unirse a la mazmorra (cuando hay mazmorra activa)",
	"!cofre         → abrir el cofre mágico (cuando aparece uno)",
	"!help          → lista de comandos",
}

func handleHelp(client *twitch.Client, channel string) {
	for _, line := range helpCommands {
		client.Say(channel, line)
	}
}
