package discord

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"

	"musician-bot-v2/internal/audio"
	"musician-bot-v2/internal/config"
	"musician-bot-v2/internal/database"
	"musician-bot-v2/internal/ui"
)

type ButtonHandler struct {
	db             *database.DB
	cfg            *config.Config
	manager        *audio.Manager
	commandHandler *CommandHandler
}

func NewButtonHandler(db *database.DB, cfg *config.Config, manager *audio.Manager, cmdHandler *CommandHandler) *ButtonHandler {
	return &ButtonHandler{
		db:             db,
		cfg:            cfg,
		manager:        manager,
		commandHandler: cmdHandler,
	}
}

func (h *ButtonHandler) Handle(event *events.ComponentInteractionCreate) {
	customID := event.ButtonInteractionData().CustomID()
	guildID := event.GuildID()
	if guildID == nil {
		return
	}

	if customID == "view_queue" || strings.HasPrefix(customID, "view_queue:") {
		h.handleViewQueue(event, *guildID, customID)
		return
	}

	if customID == "back_to_player" {
		h.handleBackToPlayer(event, *guildID)
		return
	}

	if customID == "save_playlist_btn" {
		h.handleSavePlaylistBtn(event, *guildID)
		return
	}

	if customID == "play_playlist" {
		h.handlePlayPlaylistBtn(event, *guildID)
		return
	}

	if customID == "start_radio" {
		h.handleStartRadioBtn(event, *guildID)
		return
	}

	if customID == "open_effects_menu" {
		h.handleOpenEffectsMenu(event, *guildID)
		return
	}

	if strings.HasPrefix(customID, "setup_confirm_yes:") {
		h.handleSetupConfirmYes(event, *guildID, customID)
		return
	}

	if customID == "setup_confirm_no" {
		_ = event.UpdateMessage(discord.MessageUpdate{
			Content:    stringPtr("❌ Setup cancelado."),
			Embeds:     &[]discord.Embed{},
			Components: &[]discord.LayoutComponent{},
		})
		return
	}
}

func (h *ButtonHandler) handleOpenEffectsMenu(event *events.ComponentInteractionCreate, guildID snowflake.ID) {
	player := h.manager.GetPlayer(guildID)
	currentFilter := player.ActiveFilter()

	components := ui.GetEffectsSelectMenu(currentFilter)

	_ = event.CreateMessage(discord.NewMessageCreate().
		WithContent("🎛️ **Painel de Efeitos de Áudio**\nSelecione um efeito abaixo para aplicar à reprodução em tempo real:").
		WithComponents(components...).
		WithFlags(discord.MessageFlagEphemeral))
}

func (h *ButtonHandler) handleViewQueue(event *events.ComponentInteractionCreate, guildID snowflake.ID, customID string) {
	page := 0
	parts := strings.Split(customID, ":")
	if len(parts) > 1 {
		if p, err := strconv.Atoi(parts[1]); err == nil {
			page = p
		}
	}

	player := h.manager.GetPlayer(guildID)
	current := player.Queue.Current()
	tracks := player.Queue.Tracks()
	history := player.Queue.History()

	pageInfo := ui.GetQueuePageInfo(current, tracks, history, page)
	embed := ui.CreatePlayerEmbed(current, tracks, history, player.Queue.IsPaused(), player.Queue.RepeatMode(), player.ActiveFilter(), true, pageInfo.Page)
	buttons := ui.GetQueueButtons(pageInfo.Page, pageInfo.TotalPages)

	var attachments []discord.AttachmentUpdate = []discord.AttachmentUpdate{}

	_ = event.UpdateMessage(discord.MessageUpdate{
		Embeds:      &[]discord.Embed{embed},
		Components:  &buttons,
		Attachments: &attachments,
	})
}

func (h *ButtonHandler) handleBackToPlayer(event *events.ComponentInteractionCreate, guildID snowflake.ID) {
	player := h.manager.GetPlayer(guildID)
	current := player.Queue.Current()
	tracks := player.Queue.Tracks()
	history := player.Queue.History()

	embed := ui.CreatePlayerEmbed(current, tracks, history, player.Queue.IsPaused(), player.Queue.RepeatMode(), player.ActiveFilter(), false, 0)
	buttons := ui.GetPlayerButtons()

	var files []*discord.File
	var attachments []discord.AttachmentUpdate = []discord.AttachmentUpdate{}

	if current == nil {
		if f, err := os.Open(h.cfg.PlaceholderPath); err == nil {
			files = append(files, discord.NewFile("placeholder.png", "", f))
		}
	}

	_ = event.UpdateMessage(discord.MessageUpdate{
		Embeds:      &[]discord.Embed{embed},
		Components:  &buttons,
		Files:       files,
		Attachments: &attachments,
	})
}

func (h *ButtonHandler) handleSavePlaylistBtn(event *events.ComponentInteractionCreate, guildID snowflake.ID) {
	player := h.manager.GetPlayer(guildID)
	current := player.Queue.Current()
	tracks := player.Queue.Tracks()

	if current == nil && len(tracks) == 0 {
		_ = event.CreateMessage(discord.NewMessageCreate().
			WithContent("Não há músicas na fila para salvar!").
			WithFlags(discord.MessageFlagEphemeral))
		return
	}

	modal := discord.NewModalCreate(
		"save_playlist_modal",
		"Salvar Playlist",
	).AddLabel("Nome da Playlist", discord.NewShortTextInput("playlist_name").
		WithPlaceholder("Ex: Minhas Favoritas").
		WithRequired(true),
	)

	_ = event.Modal(modal)
}

func (h *ButtonHandler) handlePlayPlaylistBtn(event *events.ComponentInteractionCreate, guildID snowflake.ID) {
	playlists, err := h.db.GetPlaylists(guildID.String())
	if err != nil || len(playlists) == 0 {
		_ = event.CreateMessage(discord.NewMessageCreate().
			WithContent("Você não tem playlists salvas neste servidor!").
			WithFlags(discord.MessageFlagEphemeral))
		return
	}

	var options []discord.StringSelectMenuOption
	for _, p := range playlists {
		options = append(options, discord.StringSelectMenuOption{
			Label: truncateString(p.Name, 100),
			Value: strconv.FormatInt(p.ID, 10),
		})
		if len(options) >= 25 {
			break
		}
	}

	selectMenu := discord.NewStringSelectMenu("select_playlist", "Escolha uma playlist...", options...)
	row := discord.NewActionRow(selectMenu)

	_ = event.CreateMessage(discord.NewMessageCreate().
		WithContent("Selecione uma playlist para tocar:").
		WithComponents(row).
		WithFlags(discord.MessageFlagEphemeral))
}

func (h *ButtonHandler) handleStartRadioBtn(event *events.ComponentInteractionCreate, guildID snowflake.ID) {
	voiceState, ok := event.Client().Caches.VoiceState(guildID, event.User().ID)
	if !ok || voiceState.ChannelID == nil {
		_ = event.CreateMessage(discord.NewMessageCreate().
			WithContent("Você precisa estar em um canal de voz para iniciar o rádio!").
			WithFlags(discord.MessageFlagEphemeral))
		return
	}

	_ = event.DeferCreateMessage(true)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		_ = event.Client().UpdateVoiceState(ctx, guildID, voiceState.ChannelID, false, false)

		loaded, err := h.manager.StartRadio(ctx, guildID)
		var respContent string
		if err != nil || loaded == 0 {
			respContent = "Ainda não há músicas favoritadas ou não foi possível carregar nenhuma faixa para a rádio."
		} else {
			respContent = fmt.Sprintf("📻 Rádio iniciada com **%d** música(s) favoritadas em modo aleatório e loop.", loaded)
		}

		_, _ = event.Client().Rest.UpdateInteractionResponse(event.ApplicationID(), event.Token(), discord.MessageUpdate{
			Content: &respContent,
		})
	}()
}

func (h *ButtonHandler) handleSetupConfirmYes(event *events.ComponentInteractionCreate, guildID snowflake.ID, customID string) {
	if !h.cfg.IsAdmin(event.User().ID.String()) {
		_ = event.CreateMessage(discord.NewMessageCreate().
			WithContent("❌ Apenas admins podem executar o setup.").
			WithFlags(discord.MessageFlagEphemeral))
		return
	}

	parts := strings.Split(customID, ":")
	if len(parts) < 2 {
		return
	}
	channelID, err := snowflake.Parse(parts[1])
	if err != nil {
		return
	}

	_ = event.UpdateMessage(discord.MessageUpdate{
		Content:    stringPtr("🔧 Reconfigurando canal e player..."),
		Embeds:     &[]discord.Embed{},
		Components: &[]discord.LayoutComponent{},
	})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		player := h.manager.GetPlayer(guildID)
		_ = player.Stop(ctx)

		h.commandHandler.PerformSetupSteps(event.Client(), guildID, event.User().ID, channelID, event.ApplicationID(), event.Token())
	}()
}
