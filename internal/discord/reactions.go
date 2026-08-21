package discord

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	"musician-bot-v2/internal/audio"
	"musician-bot-v2/internal/config"
	"musician-bot-v2/internal/database"
	"musician-bot-v2/internal/lyrics"
	"musician-bot-v2/internal/ui"
)

type ReactionHandler struct {
	db      *database.DB
	cfg     *config.Config
	manager *audio.Manager
}

func NewReactionHandler(db *database.DB, cfg *config.Config, manager *audio.Manager) *ReactionHandler {
	return &ReactionHandler{
		db:      db,
		cfg:     cfg,
		manager: manager,
	}
}

func (h *ReactionHandler) Handle(event *events.GuildMessageReactionAdd) {
	if event.Member.User.Bot {
		return
	}

	guildID := event.GuildID
	cfg, err := h.db.GetGuildConfig(guildID.String())
	if err != nil || cfg == nil || cfg.MusicRoomID != event.ChannelID.String() {
		return
	}

	if event.Emoji.Name == nil {
		return
	}
	emojiName := *event.Emoji.Name

	// Remove user reaction immediately to keep buttons clean
	_ = event.Client().Rest.RemoveUserReaction(event.ChannelID, event.MessageID, emojiName, event.UserID)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		player := h.manager.GetPlayer(guildID)

		switch emojiName {
		case "⏮️", "⏮":
			_ = player.PlayPrevious(ctx)

		case "▶️", "▶", "⏸️", "⏸":
			_ = player.Pause(ctx, !player.Queue.IsPaused())

		case "⏭️", "⏭":
			_ = player.Skip(ctx)

		case "⏹️", "⏹":
			_ = player.Stop(ctx)

		case "🔀":
			player.Queue.Shuffle()
			h.manager.UpdatePlayerMessage(guildID)

		case "🔁":
			player.Queue.CycleRepeatMode()
			h.manager.UpdatePlayerMessage(guildID)

		case "⭐", "🌟":
			if !h.cfg.IsAdmin(event.UserID.String()) {
				SendTemporaryFeedback(event.Client(), event.ChannelID, event.UserID, "você não tem permissão para favoritar músicas.")
				return
			}
			current := player.Queue.Current()
			if current == nil || current.Track.Info.URI == nil {
				SendTemporaryFeedback(event.Client(), event.ChannelID, event.UserID, "não há música tocando agora para favoritar.")
				return
			}

			dur := ui.FormatDuration(current.Track.Info.Length)
			saved, err := h.db.SaveFavoriteSong(
				guildID.String(),
				event.UserID.String(),
				current.Track.Info.Title,
				*current.Track.Info.URI,
				&dur,
				current.Track.Info.ArtworkURL,
			)
			if err != nil {
				log.Printf("[Favorites Error] Falha ao favoritar música: %v", err)
				SendTemporaryFeedback(event.Client(), event.ChannelID, event.UserID, "houve um erro ao salvar a música nos favoritos.")
				return
			}

			if saved {
				log.Printf("[Favorites] Música favoritada: '%s' por %s", current.Track.Info.Title, event.Member.User.Tag())
				SendTemporaryFeedback(event.Client(), event.ChannelID, event.UserID, fmt.Sprintf("música favoritada: **%s**.", current.Track.Info.Title))
			} else {
				SendTemporaryFeedback(event.Client(), event.ChannelID, event.UserID, fmt.Sprintf("essa música já estava nos favoritos: **%s**.", current.Track.Info.Title))
			}

		case "📝", "📄":
			current := player.Queue.Current()
			if current == nil {
				SendTemporaryFeedback(event.Client(), event.ChannelID, event.UserID, "não há música tocando agora para buscar a letra.")
				return
			}

			// Clear existing lyrics message if present
			h.manager.ClearLyricsMessage(guildID)

			trackTitle := current.Track.Info.Title
			trackAuthor := current.Track.Info.Author

			res, err := lyrics.FetchLyrics(trackTitle, trackAuthor)
			if err != nil || res == nil || strings.TrimSpace(res.PlainLyrics) == "" {
				SendTemporaryFeedback(event.Client(), event.ChannelID, event.UserID, fmt.Sprintf("não encontrei a letra para a música **%s**.", trackTitle))
				return
			}

			plainText := res.PlainLyrics
			if len(plainText) > 4000 {
				plainText = plainText[:3990] + "..."
			}

			embed := discord.NewEmbed().
				WithColor(0x5865F2).
				WithTitle(fmt.Sprintf("📝 Letra: %s", res.TrackName)).
				WithDescription(plainText).
				WithFooterText("Esta mensagem será apagada automaticamente quando a música terminar.")

			if res.ArtistName != "" {
				embed = embed.WithAuthorName(res.ArtistName)
			}

			sentMsg, err := event.Client().Rest.CreateMessage(event.ChannelID, discord.NewMessageCreate().WithEmbeds(embed))
			if err == nil && sentMsg != nil {
				h.manager.SetLyricsMessage(guildID, sentMsg.ID)
			}

		case "🏠", "🚪":
			_ = h.manager.LeaveVoice(ctx, guildID)
		}
	}()
}
