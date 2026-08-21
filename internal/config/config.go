package config

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DiscordToken     string
	AdminUserIDs     []string
	LavalinkHost     string
	LavalinkPassword string
	LavalinkSecure   bool
	DatabasePath     string
	BannerPath       string
	PlaceholderPath  string
}

func Load() *Config {
	// Try loading .env if it exists
	_ = godotenv.Load()

	token := strings.TrimSpace(os.Getenv("DISCORD_TOKEN"))
	if token == "" {
		log.Fatal("ERRO FATAL: DISCORD_TOKEN não encontrado nas variáveis de ambiente.")
	}

	rawAdmins := os.Getenv("ADMIN_USER_IDS")
	var adminIDs []string
	if rawAdmins != "" {
		for _, id := range strings.Split(rawAdmins, ",") {
			trimmed := strings.TrimSpace(id)
			if trimmed != "" {
				adminIDs = append(adminIDs, trimmed)
			}
		}
	} else {
		log.Println("AVISO: ADMIN_USER_IDS não configurado. Nenhum usuário terá permissões administrativas.")
	}

	lavalinkHost := getEnvDefault("LAVALINK_HOST", "127.0.0.1:2333")
	lavalinkPassword := getEnvDefault("LAVALINK_PASSWORD", "youshallnotpass")
	lavalinkSecure := os.Getenv("LAVALINK_SECURE") == "true"
	databasePath := getEnvDefault("DATABASE_PATH", "./data/database.sqlite")
	bannerPath := getEnvDefault("BANNER_PATH", "./assets/banner.png")
	placeholderPath := getEnvDefault("PLACEHOLDER_PATH", "./assets/placeholder.png")

	return &Config{
		DiscordToken:     token,
		AdminUserIDs:     adminIDs,
		LavalinkHost:     lavalinkHost,
		LavalinkPassword: lavalinkPassword,
		LavalinkSecure:   lavalinkSecure,
		DatabasePath:     databasePath,
		BannerPath:       bannerPath,
		PlaceholderPath:  placeholderPath,
	}
}

func (c *Config) IsAdmin(userID string) bool {
	for _, id := range c.AdminUserIDs {
		if id == userID {
			return true
		}
	}
	return false
}

func getEnvDefault(key, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return fallback
}
