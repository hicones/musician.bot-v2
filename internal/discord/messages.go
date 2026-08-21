package discord

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgolink/v3/lavalink"

	"musician-bot-v2/internal/audio"
	"musician-bot-v2/internal/database"
	"musician-bot-v2/internal/ui"
)

type MessageHandler struct {
	db      *database.DB
	manager *audio.Manager
}

func NewMessageHandler(db *database.DB, manager *audio.Manager) *MessageHandler {
	return &MessageHandler{
		db:      db,
		manager: manager,
	}
}

func (h *MessageHandler) Handle(event *events.MessageCreate) {
	if event.Message.Author.Bot {
		return
	}
	guildID := event.Message.GuildID
	if guildID == nil {
		return
	}

	cfg, _ := h.db.GetGuildConfig(guildID.String())
	isMusicRoom := false
	if cfg != nil && cfg.MusicRoomID == event.Message.ChannelID.String() {
		isMusicRoom = true
	} else {
		ch, ok := event.Client().Caches.Channel(event.Message.ChannelID)
		if ok && ch.Name() == "music-room" {
			isMusicRoom = true
		}
	}

	if !isMusicRoom {
		return
	}

	query := strings.TrimSpace(event.Message.Content)
	if query == "" || strings.HasPrefix(query, "!") {
		return
	}

	// Delete user message to keep #music-room clean
	_ = event.Client().Rest.DeleteMessage(event.Message.ChannelID, event.Message.ID)

	// Check if author is in voice channel
	voiceState, ok := event.Client().Caches.VoiceState(*guildID, event.Message.Author.ID)
	if !ok || voiceState.ChannelID == nil {
		return
	}

	voiceChannelID := *voiceState.ChannelID
	requestType := audio.GetMusicRequestType(query)
	log.Printf("[Music] %s detectada: %s (Usuário: %s)", requestType, query, event.Message.Author.Tag())

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		player := h.manager.GetPlayer(*guildID)

		// If radio mode was active, disable it and clear queue
		if player.Queue.IsRadioMode() {
			player.Queue.Clear()
			player.Queue.SetRadioMode(false)
			player.Queue.SetRepeatMode(ui.RepeatModeOff)
		}

		// Connect bot to voice channel if not already connected
		_ = event.Client().UpdateVoiceState(ctx, *guildID, &voiceChannelID, false, false)

		resolvedQuery := h.manager.ResolveQuery(query)
		result, err := h.manager.LoadTracks(ctx, resolvedQuery)
		if err != nil || result == nil {
			log.Printf("[Music Error] Falha ao carregar faixa '%s': %v", query, err)
			return
		}

		switch result.LoadType {
		case lavalink.LoadTypeTrack:
			if track, ok := result.Data.(lavalink.Track); ok {
				player.Queue.Push(ui.TrackItem{
					Track:       track,
					RequestedBy: event.Message.Author.Username,
				})
				log.Printf("[Music] Faixa adicionada à fila: '%s'", track.Info.Title)
			}
		case lavalink.LoadTypeSearch:
			if search, ok := result.Data.(lavalink.Search); ok && len(search) > 0 {
				player.Queue.Push(ui.TrackItem{
					Track:       search[0],
					RequestedBy: event.Message.Author.Username,
				})
				log.Printf("[Music] Busca carregada: '%s'", search[0].Info.Title)
			}
		case lavalink.LoadTypePlaylist:
			if playlist, ok := result.Data.(lavalink.Playlist); ok && len(playlist.Tracks) > 0 {
				var items []ui.TrackItem
				for _, t := range playlist.Tracks {
					items = append(items, ui.TrackItem{
						Track:       t,
						RequestedBy: event.Message.Author.Username,
					})
				}
				player.Queue.Push(items...)
				log.Printf("[Music] Playlist carregada: '%s' (%d faixas)", playlist.Info.Name, len(items))
			}
		case lavalink.LoadTypeEmpty:
			log.Printf("[Music] Nenhum resultado encontrado para: '%s'", query)
			return
		case lavalink.LoadTypeError:
			log.Printf("[Music] Erro retornado pelo Lavalink para: '%s'", query)
			return
		}

		if player.Queue.Current() == nil {
			_ = player.PlayNext(ctx)
		} else {
			h.manager.UpdatePlayerMessage(*guildID)
		}
	}()
}
