package discord

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"

	"musician-bot-v2/internal/audio"
	"musician-bot-v2/internal/config"
	"musician-bot-v2/internal/database"
	"musician-bot-v2/internal/ui"
)

type ModalHandler struct {
	db      *database.DB
	cfg     *config.Config
	manager *audio.Manager
}

func NewModalHandler(db *database.DB, cfg *config.Config, manager *audio.Manager) *ModalHandler {
	return &ModalHandler{
		db:      db,
		cfg:     cfg,
		manager: manager,
	}
}

func (h *ModalHandler) Handle(event *events.ModalSubmitInteractionCreate) {
	customID := event.Data.CustomID
	guildID := event.GuildID()
	if guildID == nil {
		return
	}

	if customID == "save_playlist_modal" {
		h.handleSavePlaylist(event, *guildID)
		return
	}

	if strings.HasPrefix(customID, "delete_playlist_confirm:") {
		h.handleDeletePlaylistConfirm(event, *guildID, customID)
		return
	}

	if strings.HasPrefix(customID, "delete_playlist_song_confirm:") {
		h.handleDeletePlaylistSongConfirm(event, *guildID, customID)
		return
	}
}

func (h *ModalHandler) handleSavePlaylist(event *events.ModalSubmitInteractionCreate, guildID snowflake.ID) {
	name := strings.TrimSpace(event.Data.Text("playlist_name"))
	if name == "" {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Nome inválido para a playlist.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	player := h.manager.GetPlayer(guildID)
	current := player.Queue.Current()
	tracks := player.Queue.Tracks()

	if current == nil && len(tracks) == 0 {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Erro ao salvar: fila vazia.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	var songList []database.SongData
	if current != nil && current.Track.Info.URI != nil {
		songList = append(songList, database.SongData{
			Title:     current.Track.Info.Title,
			URL:       *current.Track.Info.URI,
			Duration:  stringPtr(ui.FormatDuration(current.Track.Info.Length)),
			Thumbnail: current.Track.Info.ArtworkURL,
		})
	}

	for _, t := range tracks {
		if t.Track.Info.URI != nil {
			songList = append(songList, database.SongData{
				Title:     t.Track.Info.Title,
				URL:       *t.Track.Info.URI,
				Duration:  stringPtr(ui.FormatDuration(t.Track.Info.Length)),
				Thumbnail: t.Track.Info.ArtworkURL,
			})
		}
	}

	_, err := h.db.SavePlaylist(name, event.User().ID.String(), guildID.String(), songList)
	if err != nil {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Erro ao salvar playlist no banco de dados.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	_ = event.CreateMessage(discord.NewMessageCreate().WithContent(fmt.Sprintf("Playlist **%s** salva com sucesso com %d músicas!", name, len(songList))).WithFlags(discord.MessageFlagEphemeral))
}

func (h *ModalHandler) handleDeletePlaylistConfirm(event *events.ModalSubmitInteractionCreate, guildID snowflake.ID, customID string) {
	if !h.cfg.IsAdmin(event.User().ID.String()) {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Você não tem permissão para excluir playlists.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	parts := strings.Split(customID, ":")
	if len(parts) < 2 {
		return
	}

	playlistID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return
	}

	confirmation := strings.ToUpper(strings.TrimSpace(event.Data.Text("delete_playlist_confirmation")))
	if confirmation != "EXCLUIR" {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Exclusão cancelada. A confirmação precisa ser EXCLUIR.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	playlist, err := h.db.GetPlaylist(playlistID, guildID.String())
	if err != nil || playlist == nil {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Playlist não encontrada neste servidor.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	deleted, err := h.db.DeletePlaylist(playlistID, guildID.String())
	if err != nil || !deleted {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Não foi possível excluir a playlist selecionada.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	_ = event.CreateMessage(discord.NewMessageCreate().WithContent(fmt.Sprintf("Playlist **%s** excluída com sucesso.", playlist.Name)).WithFlags(discord.MessageFlagEphemeral))
}

func (h *ModalHandler) handleDeletePlaylistSongConfirm(event *events.ModalSubmitInteractionCreate, guildID snowflake.ID, customID string) {
	if !h.cfg.IsAdmin(event.User().ID.String()) {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Você não tem permissão para excluir faixas de playlists.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	parts := strings.Split(customID, ":")
	if len(parts) < 3 {
		return
	}

	playlistID, err := strconv.ParseInt(parts[1], 10, 64)
	songID, err2 := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || err2 != nil {
		return
	}

	confirmation := strings.ToUpper(strings.TrimSpace(event.Data.Text("delete_playlist_song_confirmation")))
	if confirmation != "EXCLUIR" {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Exclusão cancelada. A confirmação precisa ser EXCLUIR.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	deleted, err := h.db.DeletePlaylistSong(playlistID, songID, guildID.String())
	if err != nil || !deleted {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Não foi possível excluir a faixa selecionada.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Faixa excluída da playlist com sucesso.").WithFlags(discord.MessageFlagEphemeral))
}
