package discord

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"

	"musician-bot-v2/internal/activity"
	"musician-bot-v2/internal/audio"
	"musician-bot-v2/internal/database"
	"musician-bot-v2/internal/ui"
)

type SelectHandler struct {
	db      *database.DB
	manager *audio.Manager
}

func NewSelectHandler(db *database.DB, manager *audio.Manager) *SelectHandler {
	return &SelectHandler{
		db:      db,
		manager: manager,
	}
}

func (h *SelectHandler) Handle(event *events.ComponentInteractionCreate) {
	data := event.StringSelectMenuInteractionData()
	guildID := event.GuildID()
	if guildID == nil || len(data.Values) == 0 {
		return
	}

	if data.CustomID() == "select_effect" {
		h.handleSelectEffect(event, *guildID, data.Values[0])
		return
	}

	if data.CustomID() != "select_playlist" {
		return
	}

	playlistID, err := strconv.ParseInt(data.Values[0], 10, 64)
	if err != nil {
		return
	}

	voiceState, ok := event.Client().Caches.VoiceState(*guildID, event.User().ID)
	if !ok || voiceState.ChannelID == nil {
		_ = event.CreateMessage(discord.NewMessageCreate().
			WithContent("Você precisa estar em um canal de voz!").
			WithFlags(discord.MessageFlagEphemeral))
		return
	}

	songs, err := h.db.GetPlaylistSongs(playlistID)
	if err != nil || len(songs) == 0 {
		_ = event.CreateMessage(discord.NewMessageCreate().
			WithContent("Esta playlist não contém músicas válidas.").
			WithFlags(discord.MessageFlagEphemeral))
		return
	}

	_ = event.DeferCreateMessage(true)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		player := h.manager.GetPlayer(*guildID)
		_ = player.Stop(ctx)

		_ = event.Client().UpdateVoiceState(ctx, *guildID, voiceState.ChannelID, false, false)

		shuffled := activity.ShuffleSlice(songs)
		loaded := 0

		for _, s := range shuffled {
			result, err := h.manager.LoadTracks(ctx, s.URL)
			if err != nil || result == nil {
				continue
			}

			switch result.LoadType {
			case lavalink.LoadTypeTrack:
				if track, ok := result.Data.(lavalink.Track); ok {
					player.Queue.Push(ui.TrackItem{
						Track:       track,
						RequestedBy: event.User().Username,
					})
					loaded++
				}
			case lavalink.LoadTypeSearch:
				if search, ok := result.Data.(lavalink.Search); ok && len(search) > 0 {
					player.Queue.Push(ui.TrackItem{
						Track:       search[0],
						RequestedBy: event.User().Username,
					})
					loaded++
				}
			case lavalink.LoadTypePlaylist:
				if playlist, ok := result.Data.(lavalink.Playlist); ok && len(playlist.Tracks) > 0 {
					player.Queue.Push(ui.TrackItem{
						Track:       playlist.Tracks[0],
						RequestedBy: event.User().Username,
					})
					loaded++
				}
			}
		}

		if loaded > 0 {
			_ = player.PlayNext(ctx)
			h.manager.UpdatePlayerMessage(*guildID)
			_, _ = event.Client().Rest.UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
				Content: stringPtr(fmt.Sprintf("🎵 Tocando playlist selecionada com **%d** músicas!", loaded)),
			})
		} else {
			_, _ = event.Client().Rest.UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
				Content: stringPtr("Não foi possível carregar as faixas da playlist."),
			})
		}
	}()
}

func (h *SelectHandler) handleSelectEffect(event *events.ComponentInteractionCreate, guildID snowflake.ID, rawValue string) {
	effectName := strings.TrimPrefix(rawValue, "effect:")
	if effectName == "clear" {
		effectName = ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	player := h.manager.GetPlayer(guildID)
	err := player.SetFilter(ctx, effectName)
	if err != nil {
		_ = event.UpdateMessage(discord.MessageUpdate{
			Content: stringPtr("❌ Erro ao aplicar efeito de áudio."),
		})
		return
	}

	feedback := "✅ Efeitos de áudio desativados (som padrão)."
	if effectName != "" {
		feedback = fmt.Sprintf("✅ Efeito **%s** aplicado com sucesso!", ui.FormatFilterName(effectName))
	}

	components := ui.GetEffectsSelectMenu(effectName)
	_ = event.UpdateMessage(discord.MessageUpdate{
		Content:    stringPtr(fmt.Sprintf("🎛️ **Painel de Efeitos de Áudio**\n%s", feedback)),
		Components: &components,
	})
}

