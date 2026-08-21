package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgolink/v3/lavalink"
)

const (
	EmbedDescriptionLimit = 4096
	QueueHeader           = "**Fila Atual**\n"
	QueueFooterReserve    = 140
	QueueListLimit        = EmbedDescriptionLimit - len(QueueHeader) - QueueFooterReserve
	QueuePageSize         = 50
)

const (
	RepeatModeOff   = 0
	RepeatModeTrack = 1
	RepeatModeQueue = 2
)

type TrackItem struct {
	Track       lavalink.Track
	RequestedBy string
}

func FormatDuration(ms lavalink.Duration) string {
	d := time.Duration(ms) * time.Millisecond
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func CreatePlayerEmbed(
	current *TrackItem,
	queue []TrackItem,
	history []TrackItem,
	paused bool,
	repeatMode int,
	activeFilter string,
	showQueue bool,
	queuePage int,
) discord.Embed {
	embed := discord.NewEmbed().
		WithColor(0x5865F2).
		WithTitle("Musician Bot")

	if current == nil {
		return embed.
			WithDescription("**Fila vazia**\n\nCole uma URL ou digite o nome de uma música neste canal para começar a tocar!").
			WithImage("attachment://placeholder.png")
	}

	if showQueue {
		queueLines := buildQueueLines(current, queue, history)
		pageInfo := GetQueuePageInfo(current, queue, history, queuePage)
		pageStart := pageInfo.Page * QueuePageSize
		pageEnd := pageStart + QueuePageSize
		if pageEnd > len(queueLines) {
			pageEnd = len(queueLines)
		}

		var pageLines []string
		if pageStart < len(queueLines) {
			pageLines = queueLines[pageStart:pageEnd]
		}

		visibleLines, hiddenCount := fitQueueLines(pageLines)
		queueList := strings.Join(visibleLines, "\n")
		hiddenText := ""
		if hiddenCount > 0 {
			hiddenText = "\n\nAlguns itens desta página foram ocultados pelo limite do Discord."
		}

		if queueList == "" {
			queueList = "Nenhuma música na fila."
		}

		desc := fmt.Sprintf("%sPágina %d/%d - %d músicas\n\n%s%s",
			QueueHeader, pageInfo.Page+1, pageInfo.TotalPages, pageInfo.TotalItems, queueList, hiddenText)
		embed = embed.WithDescription(desc)
	} else {
		trackName := current.Track.Info.Title
		trackURL := ""
		if current.Track.Info.URI != nil {
			trackURL = *current.Track.Info.URI
		}
		embed = embed.WithDescription(fmt.Sprintf("**Tocando agora:**\n[%s](%s)", trackName, trackURL))

		durationStr := FormatDuration(current.Track.Info.Length)
		if current.Track.Info.IsStream {
			durationStr = "🔴 Ao vivo"
		}

		embed = embed.AddField("Duração", durationStr, true)
		embed = embed.AddField("Pedido por", current.RequestedBy, true)

		if current.Track.Info.ArtworkURL != nil && *current.Track.Info.ArtworkURL != "" {
			embed = embed.WithImage(*current.Track.Info.ArtworkURL)
		}
	}

	var status []string
	if paused {
		status = append(status, "Pausado")
	}
	if repeatMode == RepeatModeTrack {
		status = append(status, "Música em loop")
	} else if repeatMode == RepeatModeQueue {
		status = append(status, "Fila em loop")
	}
	if fName := FormatFilterName(activeFilter); fName != "" {
		status = append(status, fName)
	}

	if len(status) > 0 {
		embed = embed.WithFooterText(fmt.Sprintf("Status: %s", strings.Join(status, " | ")))
	}

	return embed
}

func FormatFilterName(filter string) string {
	switch filter {
	case "bass_boost_low":
		return "💥 Bass Boost Leve"
	case "bass_boost_medium":
		return "💥 Bass Boost Médio"
	case "bass_boost_high":
		return "💣 Bass Boost Pesado"
	case "nightcore":
		return "⚡ Nightcore"
	case "vaporwave":
		return "🌌 Vaporwave"
	case "8d":
		return "🎧 8D Audio"
	case "karaoke":
		return "🎤 Karaokê"
	default:
		return ""
	}
}

type PageInfo struct {
	Page       int
	TotalPages int
	TotalItems int
}

func GetQueuePageInfo(current *TrackItem, queue []TrackItem, history []TrackItem, requestedPage int) PageInfo {
	if current == nil {
		return PageInfo{Page: 0, TotalPages: 1, TotalItems: 0}
	}

	queueLines := buildQueueLines(current, queue, history)
	totalItems := len(queueLines)
	totalPages := int(math.Ceil(float64(totalItems) / float64(QueuePageSize)))
	if totalPages < 1 {
		totalPages = 1
	}

	page := requestedPage
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	return PageInfo{
		Page:       page,
		TotalPages: totalPages,
		TotalItems: totalItems,
	}
}

func buildQueueLines(current *TrackItem, queue []TrackItem, history []TrackItem) []string {
	var lines []string

	// History items (skip current if duplicate)
	for i, h := range history {
		if h.Track.Info.Identifier == current.Track.Info.Identifier {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d. %s - %s [tocada]", i+1, h.Track.Info.Title, FormatDuration(h.Track.Info.Length)))
	}

	currentIndex := len(lines) + 1
	lines = append(lines, fmt.Sprintf("%d. %s - %s [tocando]", currentIndex, current.Track.Info.Title, FormatDuration(current.Track.Info.Length)))

	for i, q := range queue {
		lines = append(lines, fmt.Sprintf("%d. %s - %s", currentIndex+i+1, q.Track.Info.Title, FormatDuration(q.Track.Info.Length)))
	}

	return lines
}

func fitQueueLines(lines []string) ([]string, int) {
	var visible []string
	length := 0

	for _, line := range lines {
		nextLength := length + len(line) + 1
		if nextLength > QueueListLimit {
			break
		}
		visible = append(visible, line)
		length = nextLength
	}

	return visible, len(lines) - len(visible)
}

func GetPlayerButtons() []discord.LayoutComponent {
	return []discord.LayoutComponent{
		discord.NewActionRow(
			discord.NewSecondaryButton("Ver Fila", "view_queue:0").WithEmoji(discord.ComponentEmoji{Name: "📋"}),
			discord.NewPrimaryButton("Tocar Playlist", "play_playlist").WithEmoji(discord.ComponentEmoji{Name: "🎵"}),
			discord.NewSuccessButton("Rádio", "start_radio").WithEmoji(discord.ComponentEmoji{Name: "📻"}),
			discord.NewSecondaryButton("Efeitos", "open_effects_menu").WithEmoji(discord.ComponentEmoji{Name: "🎛️"}),
		),
	}
}

func GetEffectsSelectMenu(currentFilter string) []discord.LayoutComponent {
	options := []discord.StringSelectMenuOption{
		discord.NewStringSelectMenuOption("Desativar Efeitos (Padrão)", "effect:clear").
			WithDescription("Restaura o equalizador e velocidade normais").
			WithEmoji(discord.ComponentEmoji{Name: "🔊"}).
			WithDefault(currentFilter == "" || currentFilter == "clear"),
		discord.NewStringSelectMenuOption("Bass Boost (Leve)", "effect:bass_boost_low").
			WithDescription("Aumento suave nas frequências graves").
			WithEmoji(discord.ComponentEmoji{Name: "💥"}).
			WithDefault(currentFilter == "bass_boost_low"),
		discord.NewStringSelectMenuOption("Bass Boost (Médio)", "effect:bass_boost_medium").
			WithDescription("Reforço moderado de graves para batidas marcantes").
			WithEmoji(discord.ComponentEmoji{Name: "💥"}).
			WithDefault(currentFilter == "bass_boost_medium"),
		discord.NewStringSelectMenuOption("Bass Boost (Pesado)", "effect:bass_boost_high").
			WithDescription("Graves intensos e encorpados").
			WithEmoji(discord.ComponentEmoji{Name: "💣"}).
			WithDefault(currentFilter == "bass_boost_high"),
		discord.NewStringSelectMenuOption("Nightcore", "effect:nightcore").
			WithDescription("Aumenta a velocidade e afina o tom (1.25x)").
			WithEmoji(discord.ComponentEmoji{Name: "⚡"}).
			WithDefault(currentFilter == "nightcore"),
		discord.NewStringSelectMenuOption("Vaporwave / Slowed", "effect:vaporwave").
			WithDescription("Desacelera a música com tom mais grave (0.85x)").
			WithEmoji(discord.ComponentEmoji{Name: "🌌"}).
			WithDefault(currentFilter == "vaporwave"),
		discord.NewStringSelectMenuOption("8D Audio", "effect:8d").
			WithDescription("Efeito surround giratório 360° (use fones)").
			WithEmoji(discord.ComponentEmoji{Name: "🎧"}).
			WithDefault(currentFilter == "8d"),
		discord.NewStringSelectMenuOption("Karaokê", "effect:karaoke").
			WithDescription("Atenua os vocais centrais para você cantar").
			WithEmoji(discord.ComponentEmoji{Name: "🎤"}).
			WithDefault(currentFilter == "karaoke"),
	}

	selectMenu := discord.NewStringSelectMenu("select_effect", "Escolha um efeito de áudio...", options...)

	return []discord.LayoutComponent{
		discord.NewActionRow(selectMenu),
	}
}

func GetQueueButtons(page, totalPages int) []discord.LayoutComponent {
	prevPage := page - 1
	if prevPage < 0 {
		prevPage = 0
	}
	nextPage := page + 1
	if nextPage >= totalPages {
		nextPage = totalPages - 1
	}

	buttons := []discord.InteractiveComponent{
		discord.NewPrimaryButton("Voltar", "back_to_player"),
	}

	if totalPages > 1 {
		prevBtn := discord.NewSecondaryButton("Anterior", fmt.Sprintf("view_queue:%d", prevPage))
		if page == 0 {
			prevBtn = prevBtn.AsDisabled()
		}
		nextBtn := discord.NewSecondaryButton("Próxima", fmt.Sprintf("view_queue:%d", nextPage))
		if page >= totalPages-1 {
			nextBtn = nextBtn.AsDisabled()
		}
		buttons = append(buttons, prevBtn, nextBtn)
	}

	buttons = append(buttons, discord.NewSuccessButton("Salvar Playlist", "save_playlist_btn"))

	return []discord.LayoutComponent{
		discord.NewActionRow(buttons...),
	}
}
