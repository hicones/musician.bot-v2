package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"musician-bot-v2/internal/config"
	"musician-bot-v2/internal/database"
	"musician-bot-v2/internal/discord"
)

func main() {
	log.Println("🎵 Iniciando Musician Bot v2...")

	cfg := config.Load()
	db, err := database.New(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Erro ao inicializar banco de dados: %v", err)
	}

	bot, err := discord.NewBot(db, cfg)
	if err != nil {
		log.Fatalf("Erro ao inicializar bot do Discord: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	if err := bot.Start(ctx); err != nil {
		cancel()
		log.Fatalf("Erro ao iniciar bot: %v", err)
	}
	cancel()

	log.Println("🚀 Musician Bot v2 está online e ouvindo eventos!")

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sigChan

	log.Println("🛑 Encerrando Musician Bot v2...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	bot.Close(shutdownCtx)
	log.Println("👋 Bot finalizado com sucesso.")
}
