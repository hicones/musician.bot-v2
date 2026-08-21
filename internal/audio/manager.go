package audio

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"

	"musician-bot-v2/internal/activity"
	"musician-bot-v2/internal/config"
	"musician-bot-v2/internal/database"
	"musician-bot-v2/internal/ui"
)

type Manager struct {
	mu              sync.RWMutex
	players         map[snowflake.ID]*Player
	lyricsMessages  map[snowflake.ID]snowflake.ID
	PresenceUpdater func(trackTitle string, isPlaying bool)
	Lavalink        disgolink.Client
	Client          *bot.Client
	DB              *database.DB
	Config          *config.Config
	Activity        *activity.ActivityManager
}

func NewManager(db *database.DB, cfg *config.Config) *Manager {
	return &Manager{
		players:        make(map[snowflake.ID]*Player),
		lyricsMessages: make(map[snowflake.ID]snowflake.ID),
		DB:             db,
		Config:         cfg,
	}
}

func (m *Manager) SetClientAndActivity(client *bot.Client, act *activity.ActivityManager) {
	m.Client = client
	m.Activity = act
}

func (m *Manager) SetLyricsMessage(guildID, msgID snowflake.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lyricsMessages[guildID] = msgID
}

func (m *Manager) ClearLyricsMessage(guildID snowflake.ID) {
	m.mu.Lock()
	msgID, ok := m.lyricsMessages[guildID]
	delete(m.lyricsMessages, guildID)
	m.mu.Unlock()

	if ok && m.Client != nil {
		cfg, err := m.DB.GetGuildConfig(guildID.String())
		if err == nil && cfg != nil && cfg.MusicRoomID != "" {
			if channelID, err := snowflake.Parse(cfg.MusicRoomID); err == nil {
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_ = m.Client.Rest.DeleteMessage(channelID, msgID, rest.WithCtx(ctx))
				}()
			}
		}
	}
}

func (m *Manager) GetPlayer(guildID snowflake.ID) *Player {
	m.mu.Lock()
	defer m.mu.Unlock()

	player, ok := m.players[guildID]
	if !ok {
		player = NewPlayer(guildID, m)
		m.players[guildID] = player
	}
	return player
}

func (m *Manager) OnTrackStarted(guildID snowflake.ID) {
	m.ClearLyricsMessage(guildID)

	player := m.GetPlayer(guildID)
	if current := player.Queue.Current(); current != nil {
		uri := ""
		if current.Track.Info.URI != nil {
			uri = *current.Track.Info.URI
		}
		author := current.Track.Info.Author
		_ = m.DB.RecordSongPlay(guildID.String(), current.RequestedBy, current.Track.Info.Title, uri, author)

		if m.PresenceUpdater != nil {
			m.PresenceUpdater(current.Track.Info.Title, true)
		}
	}

	if m.Activity != nil {
		m.Activity.OnTrackStarted(guildID)
	}
}

func (m *Manager) OnQueueFinished(guildID snowflake.ID) {
	m.ClearLyricsMessage(guildID)

	if m.PresenceUpdater != nil {
		m.PresenceUpdater("", false)
	}

	if m.Activity != nil {
		m.Activity.OnQueueFinished(guildID)
	}
}

func (m *Manager) IsRadioActive(guildID snowflake.ID) bool {
	player := m.GetPlayer(guildID)
	return player.Queue.IsRadioMode()
}

func (m *Manager) LeaveVoice(ctx context.Context, guildID snowflake.ID) error {
	player := m.GetPlayer(guildID)
	_ = player.Stop(ctx)

	if m.Activity != nil {
		m.Activity.Clear(guildID)
	}

	return m.Client.UpdateVoiceState(ctx, guildID, nil, false, false)
}

func (m *Manager) ResolveQuery(query string) string {
	trimmed := strings.TrimSpace(query)
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	return "ytsearch:" + trimmed
}

func (m *Manager) LoadTracks(ctx context.Context, identifier string) (*lavalink.LoadResult, error) {
	node := m.Lavalink.BestNode()
	if node == nil {
		return nil, fmt.Errorf("nenhum nó lavalink disponível")
	}
	return node.LoadTracks(ctx, identifier)
}

func (m *Manager) StartRadio(ctx context.Context, guildID snowflake.ID) (int, error) {
	favs, err := m.DB.GetFavoriteSongs(guildID.String())
	if err != nil || len(favs) == 0 {
		return 0, err
	}

	shuffled := activity.ShuffleSlice(favs)
	player := m.GetPlayer(guildID)
	player.Queue.Clear()
	player.Queue.SetRepeatMode(ui.RepeatModeQueue)
	player.Queue.SetRadioMode(true)

	loaded := 0
	for _, fav := range shuffled {
		result, err := m.LoadTracks(ctx, fav.URL)
		if err != nil || result == nil {
			continue
		}

		switch result.LoadType {
		case lavalink.LoadTypeTrack:
			if track, ok := result.Data.(lavalink.Track); ok {
				player.Queue.Push(ui.TrackItem{
					Track:       track,
					RequestedBy: "Rádio Automática",
				})
				loaded++
			}
		case lavalink.LoadTypeSearch:
			if search, ok := result.Data.(lavalink.Search); ok && len(search) > 0 {
				player.Queue.Push(ui.TrackItem{
					Track:       search[0],
					RequestedBy: "Rádio Automática",
				})
				loaded++
			}
		case lavalink.LoadTypePlaylist:
			if playlist, ok := result.Data.(lavalink.Playlist); ok && len(playlist.Tracks) > 0 {
				player.Queue.Push(ui.TrackItem{
					Track:       playlist.Tracks[0],
					RequestedBy: "Rádio Automática",
				})
				loaded++
			}
		}
	}

	if loaded > 0 {
		_ = player.PlayNext(ctx)
		m.UpdatePlayerMessage(guildID)
	}

	return loaded, nil
}

func (m *Manager) UpdatePlayerMessage(guildID snowflake.ID) {
	go func() {
		cfg, err := m.DB.GetGuildConfig(guildID.String())
		if err != nil || cfg == nil || cfg.MusicRoomID == "" || cfg.PlayerMessageID == nil || *cfg.PlayerMessageID == "" {
			return
		}

		channelID, err := snowflake.Parse(cfg.MusicRoomID)
		if err != nil {
			return
		}
		messageID, err := snowflake.Parse(*cfg.PlayerMessageID)
		if err != nil {
			return
		}

		player := m.GetPlayer(guildID)
		current := player.Queue.Current()
		tracks := player.Queue.Tracks()
		history := player.Queue.History()
		paused := player.Queue.IsPaused()
		repeatMode := player.Queue.RepeatMode()

		embed := ui.CreatePlayerEmbed(current, tracks, history, paused, repeatMode, player.ActiveFilter(), false, 0)
		buttons := ui.GetPlayerButtons()

		var files []*discord.File
		var attachments []discord.AttachmentUpdate = []discord.AttachmentUpdate{}

		if current == nil {
			if f, err := os.Open(m.Config.PlaceholderPath); err == nil {
				files = append(files, discord.NewFile("placeholder.png", "", f))
			}
		}

		msgUpdate := discord.MessageUpdate{
			Embeds:      &[]discord.Embed{embed},
			Components:  &buttons,
			Files:       files,
			Attachments: &attachments,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, _ = m.Client.Rest.UpdateMessage(channelID, messageID, msgUpdate, rest.WithCtx(ctx))
	}()
}

func GetMusicRequestType(input string) string {
	u, err := url.Parse(input)
	if err == nil && u.Scheme != "" && u.Host != "" {
		host := strings.ToLower(u.Host)
		if strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be") {
			if strings.Contains(input, "list=") || strings.Contains(u.Path, "/playlist") {
				return "Playlist YouTube"
			}
			return "URL YouTube"
		}
		if strings.Contains(host, "spotify.com") {
			if strings.Contains(u.Path, "/playlist/") {
				return "Playlist Spotify"
			}
			return "URL Spotify"
		}
		if strings.Contains(host, "soundcloud.com") {
			return "URL SoundCloud"
		}
		return "URL"
	}
	return "Busca YouTube"
}
