package discord

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"musician-bot-v2/internal/audio"
	"musician-bot-v2/internal/config"
	"musician-bot-v2/internal/database"
	"musician-bot-v2/internal/ui"
)

var SlashCommands = []discord.ApplicationCommandCreate{
	discord.SlashCommandCreate{
		Name:        "setup",
		Description: "Configura o bot de música no servidor",
	},
	discord.SlashCommandCreate{
		Name:        "add-current-to-playlist",
		Description: "Salva a faixa atual em uma playlist",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionSubCommand{
				Name:        "existente",
				Description: "Adiciona a faixa atual em uma playlist existente",
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionString{
						Name:         "playlist",
						Description:  "Playlist que receberá a faixa atual",
						Required:     true,
						Autocomplete: true,
					},
				},
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "nova",
				Description: "Cria uma playlist nova com a faixa atual",
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionString{
						Name:        "nome",
						Description: "Nome da nova playlist",
						Required:    true,
						MaxLength:   intPtr(100),
					},
				},
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "delete-playlist",
		Description: "Exclui uma playlist salva",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:         "playlist",
				Description:  "Playlist que será excluída",
				Required:     true,
				Autocomplete: true,
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "delete-playlist-song",
		Description: "Exclui uma faixa de uma playlist salva",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:         "playlist",
				Description:  "Playlist de onde a faixa será removida",
				Required:     true,
				Autocomplete: true,
			},
			discord.ApplicationCommandOptionString{
				Name:         "faixa",
				Description:  "Faixa que será removida",
				Required:     true,
				Autocomplete: true,
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "remove-favorite",
		Description: "Remove uma faixa dos favoritos",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:         "faixa",
				Description:  "Faixa favoritada que será removida",
				Required:     true,
				Autocomplete: true,
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "export-playlist",
		Description: "Exporta uma playlist salva em um arquivo",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:         "playlist",
				Description:  "Playlist que será exportada",
				Required:     true,
				Autocomplete: true,
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "import-playlist",
		Description: "Importa uma playlist exportada",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionAttachment{
				Name:        "arquivo",
				Description: "Arquivo .csv gerado pelo export de playlist",
				Required:    true,
			},
		},
	},
	discord.SlashCommandCreate{
		Name:        "top",
		Description: "Exibe o ranking de músicas e usuários mais ativos no servidor",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionSubCommand{
				Name:        "musicas",
				Description: "Exibe o Top 10 músicas mais tocadas no servidor",
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "usuarios",
				Description: "Exibe o Top 10 membros que mais pediram músicas",
			},
		},
	},
}

func intPtr(i int) *int {
	return &i
}

type CommandHandler struct {
	db      *database.DB
	cfg     *config.Config
	manager *audio.Manager
}

func NewCommandHandler(db *database.DB, cfg *config.Config, manager *audio.Manager) *CommandHandler {
	return &CommandHandler{
		db:      db,
		cfg:     cfg,
		manager: manager,
	}
}

func (h *CommandHandler) HandleSlash(event *events.ApplicationCommandInteractionCreate) {
	data := event.SlashCommandInteractionData()
	name := data.CommandName()

	switch name {
	case "setup":
		h.handleSetup(event)
	case "add-current-to-playlist":
		h.handleAddCurrentToPlaylist(event)
	case "delete-playlist":
		h.handleDeletePlaylist(event)
	case "delete-playlist-song":
		h.handleDeletePlaylistSong(event)
	case "remove-favorite":
		h.handleRemoveFavorite(event)
	case "export-playlist":
		h.handleExportPlaylist(event)
	case "import-playlist":
		h.handleImportPlaylist(event)
	case "top":
		h.handleTop(event)
	}
}

func (h *CommandHandler) HandleAutocomplete(event *events.AutocompleteInteractionCreate) {
	data := event.Data
	guildID := event.GuildID()
	if guildID == nil {
		_ = event.AutocompleteResult(nil)
		return
	}

	userID := event.User().ID.String()

	switch data.CommandName {
	case "add-current-to-playlist":
		h.autocompleteAddCurrentToPlaylist(event, *guildID)
	case "delete-playlist":
		if !h.cfg.IsAdmin(userID) {
			_ = event.AutocompleteResult(nil)
			return
		}
		h.autocompletePlaylists(event, *guildID)
	case "delete-playlist-song":
		if !h.cfg.IsAdmin(userID) {
			_ = event.AutocompleteResult(nil)
			return
		}
		h.autocompletePlaylistSongs(event, *guildID)
	case "remove-favorite":
		if !h.cfg.IsAdmin(userID) {
			_ = event.AutocompleteResult(nil)
			return
		}
		h.autocompleteFavorites(event, *guildID)
	case "export-playlist":
		h.autocompletePlaylists(event, *guildID)
	}
}

func (h *CommandHandler) handleSetup(event *events.ApplicationCommandInteractionCreate) {
	userID := event.User().ID.String()
	if !h.cfg.IsAdmin(userID) {
		_ = event.CreateMessage(discord.NewMessageCreate().
			WithContent("❌ Você não tem permissão para usar este comando. Apenas admins configurados podem executar o setup.").
			WithFlags(discord.MessageFlagEphemeral))
		return
	}

	guildID := event.GuildID()
	if guildID == nil {
		_ = event.CreateMessage(discord.NewMessageCreate().
			WithContent("❌ Este comando só pode ser usado em um servidor.").
			WithFlags(discord.MessageFlagEphemeral))
		return
	}

	// Defer ephemeral reply
	_ = event.DeferCreateMessage(true)

	guild, ok := event.Client().Caches.Guild(*guildID)
	if !ok {
		_, _ = event.Client().Rest.UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
			Content: stringPtr("❌ Servidor não encontrado no cache."),
		})
		return
	}

	// Check if music-room exists
	var musicChannel *discord.GuildTextChannel
	for c := range event.Client().Caches.ChannelsForGuild(*guildID) {
		if c.Type() == discord.ChannelTypeGuildText && c.Name() == "music-room" {
			if tc, ok := c.(discord.GuildTextChannel); ok {
				musicChannel = &tc
				break
			}
		}
	}

	if musicChannel != nil {
		h.promptReconfigure(event, *guildID, musicChannel.ID())
		return
	}

	// Create music-room channel
	h.createAndSetupChannel(event, guild.ID)
}

func (h *CommandHandler) promptReconfigure(event *events.ApplicationCommandInteractionCreate, guildID snowflake.ID, channelID snowflake.ID) {
	cfg, _ := h.db.GetGuildConfig(guildID.String())

	var desc string
	if cfg != nil {
		desc = fmt.Sprintf("Este servidor já possui um canal `#music-room` configurado em **%s** por <@%s>.\n\nDeseja reconfigurá-lo? Isso limpará mensagens antigas do bot e recriará o player.",
			cfg.SetupAt.Format("02/01/2006"), cfg.SetupBy)
	} else {
		desc = "Este servidor já possui um canal `#music-room`. Deseja reconfigurá-lo?"
	}

	embed := discord.NewEmbed().
		WithTitle("⚠️ Canal music-room já existe").
		WithDescription(desc).
		WithColor(0xFFA500)

	row := discord.NewActionRow(
		discord.NewDangerButton("Sim, reconfigurar", fmt.Sprintf("setup_confirm_yes:%s", channelID)).WithEmoji(discord.ComponentEmoji{Name: "✅"}),
		discord.NewSecondaryButton("Cancelar", "setup_confirm_no").WithEmoji(discord.ComponentEmoji{Name: "❌"}),
	)

	_, _ = event.Client().Rest.UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
		Embeds:     &[]discord.Embed{embed},
		Components: &[]discord.LayoutComponent{row},
	})
}

func (h *CommandHandler) createAndSetupChannel(event *events.ApplicationCommandInteractionCreate, guildID snowflake.ID) {
	topic := "Controle de Música do Bot - Envie links ou nomes de músicas"
	ch, err := event.Client().Rest.CreateGuildChannel(guildID, discord.GuildTextChannelCreate{
		Name:  "music-room",
		Topic: topic,
	})
	if err != nil {
		_, _ = event.Client().Rest.UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
			Content: stringPtr(fmt.Sprintf("❌ Erro ao criar canal: %v", err)),
		})
		return
	}

	h.PerformSetupSteps(event.Client(), guildID, event.User().ID, ch.ID(), event.ApplicationID(), event.Token())
}

func (h *CommandHandler) PerformSetupSteps(client *bot.Client, guildID snowflake.ID, userID snowflake.ID, channelID snowflake.ID, appID snowflake.ID, token string) {
	type Step struct {
		Name     string
		Success  bool
		Duration time.Duration
		Error    string
	}

	var steps []Step

	// Step 1: Cleanup old bot messages in channel
	s1Start := time.Now()
	msgs, err := client.Rest.GetMessages(channelID, 0, 0, 0, 100)
	if err == nil {
		var botMsgIDs []snowflake.ID
		selfID := client.ID()
		for _, m := range msgs {
			if m.Author.ID == selfID {
				botMsgIDs = append(botMsgIDs, m.ID)
			}
		}
		if len(botMsgIDs) > 0 {
			_ = client.Rest.BulkDeleteMessages(channelID, botMsgIDs)
		}
		steps = append(steps, Step{Name: "Limpar mensagens antigas", Success: true, Duration: time.Since(s1Start)})
	} else {
		steps = append(steps, Step{Name: "Limpar mensagens antigas", Success: false, Duration: time.Since(s1Start), Error: err.Error()})
	}

	// Step 2: Send banner header
	s2Start := time.Now()
	var bannerFile *os.File
	bannerFile, err = os.Open(h.cfg.BannerPath)
	if err == nil {
		_, err = client.Rest.CreateMessage(channelID, discord.MessageCreate{
			Files: []*discord.File{discord.NewFile("banner.png", "", bannerFile)},
		})
		_ = bannerFile.Close()
	}
	if err == nil {
		steps = append(steps, Step{Name: "Enviar cabeçalho visual", Success: true, Duration: time.Since(s2Start)})
	} else {
		steps = append(steps, Step{Name: "Enviar cabeçalho visual", Success: false, Duration: time.Since(s2Start), Error: err.Error()})
	}

	// Step 3: Send player embed
	s3Start := time.Now()
	embed := ui.CreatePlayerEmbed(nil, nil, nil, false, ui.RepeatModeOff, "", false, 0)
	buttons := ui.GetPlayerButtons()
	var placeholderFile *os.File
	var playerMsg *discord.Message
	placeholderFile, err = os.Open(h.cfg.PlaceholderPath)
	if err == nil {
		playerMsg, err = client.Rest.CreateMessage(channelID, discord.MessageCreate{
			Embeds:     []discord.Embed{embed},
			Components: buttons,
			Files:      []*discord.File{discord.NewFile("placeholder.png", "", placeholderFile)},
		})
		_ = placeholderFile.Close()
	}
	if err == nil && playerMsg != nil {
		steps = append(steps, Step{Name: "Enviar embed do player", Success: true, Duration: time.Since(s3Start)})
	} else {
		errMsg := "desconhecido"
		if err != nil {
			errMsg = err.Error()
		}
		steps = append(steps, Step{Name: "Enviar embed do player", Success: false, Duration: time.Since(s3Start), Error: errMsg})
	}

	// Step 4: Add reactions to player message
	s4Start := time.Now()
	playerEmojis := []string{
		"⏮️", "▶️", "⏭️", "⏹️", "🔀", "🔁", "🌟", "📝", "🚪",
	}
	reactionsAdded := 0
	if playerMsg != nil {
		for _, emoji := range playerEmojis {
			if err := client.Rest.AddReaction(channelID, playerMsg.ID, emoji); err == nil {
				reactionsAdded++
			}
		}
	}
	steps = append(steps, Step{
		Name:     fmt.Sprintf("Adicionar reações (%d/%d)", reactionsAdded, len(playerEmojis)),
		Success:  reactionsAdded == len(playerEmojis),
		Duration: time.Since(s4Start),
	})

	// Step 5: Save config to database
	s5Start := time.Now()
	var playerMsgID *string
	if playerMsg != nil {
		idStr := playerMsg.ID.String()
		playerMsgID = &idStr
	}
	err = h.db.SaveGuildConfig(database.GuildConfig{
		GuildID:         guildID.String(),
		MusicRoomID:     channelID.String(),
		PlayerMessageID: playerMsgID,
		SetupBy:         userID.String(),
		Status:          "active",
	})
	if err == nil {
		steps = append(steps, Step{Name: "Salvar configuração no banco de dados", Success: true, Duration: time.Since(s5Start)})
	} else {
		steps = append(steps, Step{Name: "Salvar configuração no banco de dados", Success: false, Duration: time.Since(s5Start), Error: err.Error()})
	}

	// Build Result Embed
	allSuccess := true
	for _, s := range steps {
		if !s.Success {
			allSuccess = false
			break
		}
	}

	resultTitle := "✅ Setup concluído com sucesso!"
	resultColor := 0x00FF00
	if !allSuccess {
		resultTitle = "⚠️ Setup concluído com avisos"
		resultColor = 0xFFA500
	}

	var stepLines []string
	for _, s := range steps {
		statusIcon := "✅"
		if !s.Success {
			statusIcon = "❌"
		}
		stepLines = append(stepLines, fmt.Sprintf("%s **%s** (%dms)", statusIcon, s.Name, s.Duration.Milliseconds()))
	}

	resEmbed := discord.NewEmbed().
		WithTitle(resultTitle).
		WithDescription(strings.Join(stepLines, "\n")).
		WithColor(resultColor)

	linkRow := discord.NewActionRow(
		discord.NewLinkButton("Ir para o canal", fmt.Sprintf("https://discord.com/channels/%s/%s", guildID, channelID)),
	)

	_, _ = client.Rest.UpdateInteractionResponse(appID, token, discord.MessageUpdate{
		Embeds:     &[]discord.Embed{resEmbed},
		Components: &[]discord.LayoutComponent{linkRow},
	})
}

func (h *CommandHandler) handleAddCurrentToPlaylist(event *events.ApplicationCommandInteractionCreate) {
	data := event.SlashCommandInteractionData()
	guildID := event.GuildID()
	if guildID == nil {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Comando apenas para servidores.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	player := h.manager.GetPlayer(*guildID)
	current := player.Queue.Current()
	if current == nil || current.Track.Info.URI == nil {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Não há uma faixa tocando agora para salvar.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	sub := data.SubCommandName
	if sub == nil {
		return
	}

	songData := database.SongData{
		Title:     current.Track.Info.Title,
		URL:       *current.Track.Info.URI,
		Duration:  stringPtr(ui.FormatDuration(current.Track.Info.Length)),
		Thumbnail: current.Track.Info.ArtworkURL,
	}

	if *sub == "existente" {
		playlistIDStr := data.String("playlist")
		playlistID, err := strconv.ParseInt(playlistIDStr, 10, 64)
		if err != nil {
			_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Playlist inválida.").WithFlags(discord.MessageFlagEphemeral))
			return
		}

		playlist, err := h.db.GetPlaylist(playlistID, guildID.String())
		if err != nil || playlist == nil {
			_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Playlist não encontrada neste servidor.").WithFlags(discord.MessageFlagEphemeral))
			return
		}

		added, err := h.db.AddPlaylistSong(playlistID, guildID.String(), songData)
		if err != nil || !added {
			_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Não foi possível adicionar a faixa na playlist selecionada.").WithFlags(discord.MessageFlagEphemeral))
			return
		}

		_ = event.CreateMessage(discord.NewMessageCreate().WithContent(fmt.Sprintf("Faixa **%s** adicionada em **%s**.", songData.Title, playlist.Name)).WithFlags(discord.MessageFlagEphemeral))
		return
	}

	if *sub == "nova" {
		name := strings.TrimSpace(data.String("nome"))
		if name == "" {
			_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Informe um nome válido para a playlist.").WithFlags(discord.MessageFlagEphemeral))
			return
		}

		_, err := h.db.SavePlaylist(name, event.User().ID.String(), guildID.String(), []database.SongData{songData})
		if err != nil {
			_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Erro ao criar playlist.").WithFlags(discord.MessageFlagEphemeral))
			return
		}

		_ = event.CreateMessage(discord.NewMessageCreate().WithContent(fmt.Sprintf("Playlist **%s** criada com **%s**.", name, songData.Title)).WithFlags(discord.MessageFlagEphemeral))
	}
}

func (h *CommandHandler) handleDeletePlaylist(event *events.ApplicationCommandInteractionCreate) {
	if !h.cfg.IsAdmin(event.User().ID.String()) {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Você não tem permissão para excluir playlists.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	guildID := event.GuildID()
	if guildID == nil {
		return
	}

	data := event.SlashCommandInteractionData()
	playlistIDStr := data.String("playlist")
	playlistID, err := strconv.ParseInt(playlistIDStr, 10, 64)
	if err != nil {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Playlist inválida.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	playlist, err := h.db.GetPlaylist(playlistID, guildID.String())
	if err != nil || playlist == nil {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Playlist não encontrada neste servidor.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	modal := discord.NewModalCreate(
		fmt.Sprintf("delete_playlist_confirm:%d", playlist.ID),
		"Confirmar exclusão",
	).AddLabel("Digite \"EXCLUIR\" para confirmar", discord.NewShortTextInput("delete_playlist_confirmation").
		WithPlaceholder(truncateString(playlist.Name, 100)).
		WithRequired(true),
	)

	_ = event.Modal(modal)
}

func (h *CommandHandler) handleDeletePlaylistSong(event *events.ApplicationCommandInteractionCreate) {
	if !h.cfg.IsAdmin(event.User().ID.String()) {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Você não tem permissão para excluir faixas de playlists.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	guildID := event.GuildID()
	if guildID == nil {
		return
	}

	data := event.SlashCommandInteractionData()
	playlistID, err := strconv.ParseInt(data.String("playlist"), 10, 64)
	songID, err2 := strconv.ParseInt(data.String("faixa"), 10, 64)
	if err != nil || err2 != nil {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Parâmetros inválidos.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	playlist, err := h.db.GetPlaylist(playlistID, guildID.String())
	if err != nil || playlist == nil {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Playlist não encontrada neste servidor.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	songs, _ := h.db.GetPlaylistSongsWithIDs(playlistID)
	var foundSong *database.PlaylistSong
	for _, s := range songs {
		if s.ID == songID {
			foundSong = &s
			break
		}
	}

	if foundSong == nil {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Faixa não encontrada.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	modal := discord.NewModalCreate(
		fmt.Sprintf("delete_playlist_song_confirm:%d:%d", playlist.ID, foundSong.ID),
		"Confirmar exclusão",
	).AddLabel("Digite \"EXCLUIR\" para confirmar", discord.NewShortTextInput("delete_playlist_song_confirmation").
		WithPlaceholder(truncateString(foundSong.Title, 100)).
		WithRequired(true),
	)

	_ = event.Modal(modal)
}

func (h *CommandHandler) handleRemoveFavorite(event *events.ApplicationCommandInteractionCreate) {
	if !h.cfg.IsAdmin(event.User().ID.String()) {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Você não tem permissão para remover favoritos.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	guildID := event.GuildID()
	if guildID == nil {
		return
	}

	data := event.SlashCommandInteractionData()
	favoriteID, err := strconv.ParseInt(data.String("faixa"), 10, 64)
	if err != nil {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Faixa inválida.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	favs, err := h.db.GetFavoriteSongs(guildID.String())
	if err != nil {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Erro ao carregar favoritos.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	var foundFav *database.FavoriteSong
	for _, f := range favs {
		if f.ID == favoriteID {
			foundFav = &f
			break
		}
	}

	if foundFav == nil {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Favorito não encontrado neste servidor.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	deleted, err := h.db.DeleteFavoriteSong(favoriteID, guildID.String())
	if err != nil || !deleted {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Não foi possível remover o favorito selecionado.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	_ = event.CreateMessage(discord.NewMessageCreate().WithContent(fmt.Sprintf("Favorito **%s** removido com sucesso.", foundFav.Title)).WithFlags(discord.MessageFlagEphemeral))
}

func (h *CommandHandler) handleExportPlaylist(event *events.ApplicationCommandInteractionCreate) {
	guildID := event.GuildID()
	if guildID == nil {
		return
	}

	data := event.SlashCommandInteractionData()
	playlistID, err := strconv.ParseInt(data.String("playlist"), 10, 64)
	if err != nil {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Playlist inválida.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	playlist, err := h.db.GetPlaylist(playlistID, guildID.String())
	if err != nil || playlist == nil {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Playlist não encontrada neste servidor.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	songs, _ := h.db.GetPlaylistSongsWithIDs(playlist.ID)

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	_ = writer.Write([]string{"playlist", "title", "url", "duration", "thumbnail"})
	for _, s := range songs {
		dur := ""
		if s.Duration != nil {
			dur = *s.Duration
		}
		thumb := ""
		if s.Thumbnail != nil {
			thumb = *s.Thumbnail
		}
		_ = writer.Write([]string{playlist.Name, s.Title, s.URL, dur, thumb})
	}
	writer.Flush()

	filename := sanitizeFilename(playlist.Name) + ".csv"
	file := discord.NewFile(filename, "", bytes.NewReader(buf.Bytes()))

	_ = event.CreateMessage(discord.NewMessageCreate().
		WithContent(fmt.Sprintf("Export da playlist **%s** gerado com %d faixa(s).", playlist.Name, len(songs))).
		WithFiles(file).
		WithFlags(discord.MessageFlagEphemeral))
}

func (h *CommandHandler) handleImportPlaylist(event *events.ApplicationCommandInteractionCreate) {
	if !h.cfg.IsAdmin(event.User().ID.String()) {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Você não tem permissão para importar playlists.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	guildID := event.GuildID()
	if guildID == nil {
		return
	}

	data := event.SlashCommandInteractionData()
	attachment, ok := data.OptAttachment("arquivo")
	if !ok || !strings.HasSuffix(strings.ToLower(attachment.Filename), ".csv") || attachment.Size > 512*1024 {
		_ = event.CreateMessage(discord.NewMessageCreate().WithContent("Envie um arquivo .csv de até 512 KB gerado pelo export de playlist.").WithFlags(discord.MessageFlagEphemeral))
		return
	}

	_ = event.DeferCreateMessage(true)

	resp, err := http.Get(attachment.URL)
	if err != nil || resp.StatusCode != http.StatusOK {
		_, _ = event.Client().Rest.UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
			Content: stringPtr("Falha ao baixar arquivo anexado."),
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		_, _ = event.Client().Rest.UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
			Content: stringPtr("Erro ao ler conteúdo do arquivo."),
		})
		return
	}

	r := csv.NewReader(bytes.NewReader(body))
	rows, err := r.ReadAll()
	if err != nil || len(rows) < 2 {
		_, _ = event.Client().Rest.UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
			Content: stringPtr("Não encontrei faixas válidas no arquivo enviado."),
		})
		return
	}

	header := rows[0]
	colTitle := -1
	colURL := -1
	colDur := -1
	colThumb := -1
	colPlaylist := -1

	for i, h := range header {
		switch strings.ToLower(strings.TrimSpace(h)) {
		case "playlist":
			colPlaylist = i
		case "title":
			colTitle = i
		case "url":
			colURL = i
		case "duration":
			colDur = i
		case "thumbnail":
			colThumb = i
		}
	}

	if colTitle == -1 || colURL == -1 {
		_, _ = event.Client().Rest.UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
			Content: stringPtr("CSV inválido. Colunas obrigatórias: title, url."),
		})
		return
	}

	playlistName := strings.TrimSuffix(attachment.Filename, ".csv")
	var songs []database.SongData

	for _, row := range rows[1:] {
		if len(row) <= colTitle || len(row) <= colURL {
			continue
		}
		title := strings.TrimSpace(row[colTitle])
		songURL := strings.TrimSpace(row[colURL])
		if title == "" || songURL == "" || !strings.HasPrefix(strings.ToLower(songURL), "http") {
			continue
		}

		if colPlaylist != -1 && len(row) > colPlaylist && strings.TrimSpace(row[colPlaylist]) != "" {
			playlistName = strings.TrimSpace(row[colPlaylist])
		}

		var dur *string
		if colDur != -1 && len(row) > colDur && strings.TrimSpace(row[colDur]) != "" {
			dur = stringPtr(strings.TrimSpace(row[colDur]))
		}
		var thumb *string
		if colThumb != -1 && len(row) > colThumb && strings.TrimSpace(row[colThumb]) != "" {
			thumb = stringPtr(strings.TrimSpace(row[colThumb]))
		}

		songs = append(songs, database.SongData{
			Title:     title,
			URL:       songURL,
			Duration:  dur,
			Thumbnail: thumb,
		})
	}

	if len(songs) == 0 {
		_, _ = event.Client().Rest.UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
			Content: stringPtr("Nenhuma música válida para importar."),
		})
		return
	}

	_, err = h.db.SavePlaylist(playlistName, event.User().ID.String(), guildID.String(), songs)
	if err != nil {
		_, _ = event.Client().Rest.UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
			Content: stringPtr(fmt.Sprintf("Erro ao salvar playlist importada: %v", err)),
		})
		return
	}

	_, _ = event.Client().Rest.UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
		Content: stringPtr(fmt.Sprintf("Playlist **%s** importada com %d faixa(s).", playlistName, len(songs))),
	})
}

// Autocompletes

func (h *CommandHandler) autocompletePlaylists(event *events.AutocompleteInteractionCreate, guildID snowflake.ID) {
	focused := event.Data.Focused()
	input := normalizeSearch(focused.String())

	playlists, err := h.db.GetPlaylists(guildID.String())
	if err != nil {
		_ = event.AutocompleteResult(nil)
		return
	}

	var choices []discord.AutocompleteChoice
	for _, p := range playlists {
		if strings.Contains(normalizeSearch(p.Name), input) {
			choices = append(choices, discord.AutocompleteChoiceString{
				Name:  truncateString(p.Name, 100),
				Value: strconv.FormatInt(p.ID, 10),
			})
			if len(choices) >= 25 {
				break
			}
		}
	}
	_ = event.AutocompleteResult(choices)
}

func (h *CommandHandler) autocompleteAddCurrentToPlaylist(event *events.AutocompleteInteractionCreate, guildID snowflake.ID) {
	h.autocompletePlaylists(event, guildID)
}

func (h *CommandHandler) autocompletePlaylistSongs(event *events.AutocompleteInteractionCreate, guildID snowflake.ID) {
	focused := event.Data.Focused()
	input := normalizeSearch(focused.String())

	if focused.Name == "playlist" {
		h.autocompletePlaylists(event, guildID)
		return
	}

	if focused.Name == "faixa" {
		playlistIDStr := event.Data.String("playlist")
		if playlistIDStr == "" {
			_ = event.AutocompleteResult(nil)
			return
		}
		playlistID, err := strconv.ParseInt(playlistIDStr, 10, 64)
		if err != nil {
			_ = event.AutocompleteResult(nil)
			return
		}

		songs, err := h.db.GetPlaylistSongsWithIDs(playlistID)
		if err != nil {
			_ = event.AutocompleteResult(nil)
			return
		}

		var choices []discord.AutocompleteChoice
		for _, s := range songs {
			if strings.Contains(normalizeSearch(s.Title), input) {
				label := s.Title
				if s.Duration != nil && *s.Duration != "" {
					label = fmt.Sprintf("%s - %s", s.Title, *s.Duration)
				}
				choices = append(choices, discord.AutocompleteChoiceString{
					Name:  truncateString(label, 100),
					Value: strconv.FormatInt(s.ID, 10),
				})
				if len(choices) >= 25 {
					break
				}
			}
		}
		_ = event.AutocompleteResult(choices)
	}
}

func (h *CommandHandler) autocompleteFavorites(event *events.AutocompleteInteractionCreate, guildID snowflake.ID) {
	focused := event.Data.Focused()
	input := normalizeSearch(focused.String())

	favs, err := h.db.GetFavoriteSongs(guildID.String())
	if err != nil {
		_ = event.AutocompleteResult(nil)
		return
	}

	var choices []discord.AutocompleteChoice
	for _, f := range favs {
		if strings.Contains(normalizeSearch(f.Title), input) {
			label := f.Title
			if f.Duration != nil && *f.Duration != "" {
				label = fmt.Sprintf("%s - %s", f.Title, *f.Duration)
			}
			choices = append(choices, discord.AutocompleteChoiceString{
				Name:  truncateString(label, 100),
				Value: strconv.FormatInt(f.ID, 10),
			})
			if len(choices) >= 25 {
				break
			}
		}
	}
	_ = event.AutocompleteResult(choices)
}

func normalizeSearch(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	res, _, _ := transform.String(t, s)
	return strings.ToLower(strings.TrimSpace(res))
}

func sanitizeFilename(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	res, _, _ := transform.String(t, s)
	res = strings.ToLower(res)
	var sb strings.Builder
	for _, r := range res {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-')
		}
	}
	clean := strings.Trim(sb.String(), "-")
	if clean == "" {
		return "playlist"
	}
	return clean
}

func truncateString(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}

func stringPtr(s string) *string {
	return &s
}

func (h *CommandHandler) handleTop(event *events.ApplicationCommandInteractionCreate) {
	guildID := event.GuildID()
	if guildID == nil {
		_ = event.CreateMessage(discord.NewMessageCreate().
			WithContent("Este comando só pode ser usado em servidores!").
			WithFlags(discord.MessageFlagEphemeral))
		return
	}

	data := event.SlashCommandInteractionData()
	subCmd := ""
	if data.SubCommandName != nil {
		subCmd = *data.SubCommandName
	}

	switch subCmd {
	case "musicas":
		stats, err := h.db.GetTopSongs(guildID.String(), 10)
		if err != nil || len(stats) == 0 {
			_ = event.CreateMessage(discord.NewMessageCreate().
				WithContent("📊 Ainda não há histórico de músicas tocadas neste servidor.").
				WithFlags(discord.MessageFlagEphemeral))
			return
		}

		var lines []string
		for i, s := range stats {
			medal := fmt.Sprintf("`#%d`", i+1)
			if i == 0 {
				medal = "🥇"
			} else if i == 1 {
				medal = "🥈"
			} else if i == 2 {
				medal = "🥉"
			}
			trackLink := s.Title
			if s.URL != "" {
				trackLink = fmt.Sprintf("[%s](%s)", s.Title, s.URL)
			}
			lines = append(lines, fmt.Sprintf("%s **%s** — `%d reproduções`", medal, trackLink, s.PlayCount))
		}

		embed := discord.NewEmbed().
			WithColor(0x5865F2).
			WithTitle("🏆 Top 10 Músicas Mais Tocadas").
			WithDescription(strings.Join(lines, "\n\n")).
			WithFooterText("Estatísticas atualizadas automaticamente a cada reprodução")

		_ = event.CreateMessage(discord.NewMessageCreate().WithEmbeds(embed))

	case "usuarios":
		stats, err := h.db.GetTopUsers(guildID.String(), 10)
		if err != nil || len(stats) == 0 {
			_ = event.CreateMessage(discord.NewMessageCreate().
				WithContent("📊 Ainda não há histórico de pedidos de músicas neste servidor.").
				WithFlags(discord.MessageFlagEphemeral))
			return
		}

		var lines []string
		for i, u := range stats {
			medal := fmt.Sprintf("`#%d`", i+1)
			if i == 0 {
				medal = "🥇"
			} else if i == 1 {
				medal = "🥈"
			} else if i == 2 {
				medal = "🥉"
			}
			lines = append(lines, fmt.Sprintf("%s **%s** — `%d músicas pedidas`", medal, u.UserID, u.PlayCount))
		}

		embed := discord.NewEmbed().
			WithColor(0x5865F2).
			WithTitle("🎧 Top 10 Usuários Mais Ativos").
			WithDescription(strings.Join(lines, "\n\n")).
			WithFooterText("Estatísticas baseadas em pedidos de músicas")

		_ = event.CreateMessage(discord.NewMessageCreate().WithEmbeds(embed))

	default:
		_ = event.CreateMessage(discord.NewMessageCreate().
			WithContent("Subcomando inválido. Use `/top musicas` ou `/top usuarios`.").
			WithFlags(discord.MessageFlagEphemeral))
	}
}

