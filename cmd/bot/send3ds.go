package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

func handleSend3DS(parts []string, username string) string {
	if len(parts) < 2 {
		return "Uso: !3ds <mensaje>"
	}

	ip := os.Getenv("DS3_IP")
	port := os.Getenv("DS3_PORT")
	if ip == "" {
		return "[3DS] DS3_IP no configurado en .env"
	}
	if port == "" {
		port = "8888"
	}

	message := fmt.Sprintf("@%s: %s", username, strings.Join(parts[1:], " "))
	if len(message) > 200 {
		message = message[:200]
	}

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%s", ip, port))
	if err != nil {
		return fmt.Sprintf("[3DS] dirección inválida: %v", err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return fmt.Sprintf("[3DS] error de conexión: %v", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(time.Second))

	if _, err = conn.Write([]byte(message)); err != nil {
		return "[3DS] no disponible"
	}

	ack := make([]byte, 4)
	if _, err = conn.Read(ack); err != nil {
		return "[3DS] no disponible"
	}

	return "[3DS] Mensaje enviado"
}
