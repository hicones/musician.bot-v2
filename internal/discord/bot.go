package discord

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"

	"musician-bot-v2/internal/activity"
	"musician-bot-v2/internal/audio"
	"musician-bot-v2/internal/config"
	"musician-bot-v2/internal/database"
)

type Bot struct {
	Client   *bot.Client
	Lavalink disgolink.Client
	Manager  *audio.Manager
	Activity *activity.ActivityManager
	DB       *database.DB
	Config   *config.Config
}

func NewBot(db *database.DB, cfg *config.Config) (*Bot, error) {
	manager := audio.NewManager(db, cfg)

	b := &Bot{
		DB:      db,
		Config:  cfg,
		Manager: manager,
	}

	manager.PresenceUpdater = func(trackTitle string, isPlaying bool) {
		b.UpdatePresence(trackTitle, isPlaying)
	}

	cmdHandler := NewCommandHandler(db, cfg, manager)
	msgHandler := NewMessageHandler(db, manager)
	reactionHandler := NewReactionHandler(db, cfg, manager)
	buttonHandler := NewButtonHandler(db, cfg, manager, cmdHandler)
	modalHandler := NewModalHandler(db, cfg, manager)
	selectHandler := NewSelectHandler(db, manager)

	listener := &events.ListenerAdapter{
		OnReady: func(event *events.Ready) {
			log.Printf("🤖 Bot logado como %s", event.User.Tag())

			// Register slash commands globally
			log.Println("Registrando comandos slash...")
			_, err := event.Client().Rest.SetGlobalCommands(event.Client().ApplicationID, SlashCommands)
			if err != nil {
				log.Printf("❌ Erro ao registrar comandos globais: %v", err)
			} else {
				log.Println("✅ Slash commands registrados com sucesso.")
			}

			// Set initial presence
			b.UpdatePresence("", false)
		},
		OnMessageCreate: func(event *events.MessageCreate) {
			msgHandler.Handle(event)
		},
		OnGuildMessageReactionAdd: func(event *events.GuildMessageReactionAdd) {
			reactionHandler.Handle(event)
		},
		OnApplicationCommandInteraction: func(event *events.ApplicationCommandInteractionCreate) {
			cmdHandler.HandleSlash(event)
		},
		OnAutocompleteInteraction: func(event *events.AutocompleteInteractionCreate) {
			cmdHandler.HandleAutocomplete(event)
		},
		OnComponentInteraction: func(event *events.ComponentInteractionCreate) {
			switch event.Data.Type() {
			case discord.ComponentTypeButton:
				buttonHandler.Handle(event)
			case discord.ComponentTypeStringSelectMenu:
				selectHandler.Handle(event)
			}
		},
		OnModalSubmit: func(event *events.ModalSubmitInteractionCreate) {
			modalHandler.Handle(event)
		},
		OnGuildVoiceStateUpdate: func(event *events.GuildVoiceStateUpdate) {
			// Forward bot voice state to Lavalink
			if event.VoiceState.UserID == event.Client().ID() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				bestNode := b.Lavalink.BestNode()
				if bestNode != nil {
					player := b.Lavalink.ExistingPlayer(event.VoiceState.GuildID)
					if player == nil || player.Node() == nil {
						b.Lavalink.RemovePlayer(event.VoiceState.GuildID)
						b.Lavalink.PlayerOnNode(bestNode, event.VoiceState.GuildID)
					}
					b.Lavalink.OnVoiceStateUpdate(ctx, event.VoiceState.GuildID, event.VoiceState.ChannelID, event.VoiceState.SessionID)
				}
			}

			// Handle empty call timeouts
			if b.Activity != nil {
				voiceHandler := NewVoiceHandler(b.Activity, b.Manager)
				voiceHandler.Handle(event)
			}
		},
		OnVoiceServerUpdate: func(event *events.VoiceServerUpdate) {
			if event.Endpoint != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				bestNode := b.Lavalink.BestNode()
				if bestNode == nil {
					log.Println("⚠️ Aviso: Nenhum nó Lavalink conectado para receber VoiceServerUpdate.")
					return
				}

				player := b.Lavalink.ExistingPlayer(event.GuildID)
				if player == nil || player.Node() == nil {
					b.Lavalink.RemovePlayer(event.GuildID)
					b.Lavalink.PlayerOnNode(bestNode, event.GuildID)
				}

				b.Lavalink.OnVoiceServerUpdate(ctx, event.GuildID, event.Token, *event.Endpoint)
			}
		},
	}

	client, err := disgo.New(cfg.DiscordToken,
		bot.WithDefaultGateway(),
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				gateway.IntentGuilds,
				gateway.IntentGuildMessages,
				gateway.IntentMessageContent,
				gateway.IntentGuildVoiceStates,
				gateway.IntentGuildMessageReactions,
			),
		),
		bot.WithCacheConfigOpts(
			cache.WithCaches(
				cache.FlagGuilds,
				cache.FlagChannels,
				cache.FlagMembers,
				cache.FlagVoiceStates,
			),
		),
		bot.WithEventListeners(listener),
	)
	if err != nil {
		return nil, err
	}

	b.Client = client

	// Parse Bot Application/User ID from Discord token
	botUserID := parseBotIDFromToken(cfg.DiscordToken)

	// Setup Lavalink Client with valid User ID
	lavalinkClient := disgolink.New(botUserID,
		disgolink.WithListenerFunc(func(p disgolink.Player, e lavalink.TrackStartEvent) {
			manager.OnTrackStarted(p.GuildID())
		}),
		disgolink.WithListenerFunc(func(p disgolink.Player, e lavalink.TrackEndEvent) {
			if e.Reason.MayStartNext() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = manager.GetPlayer(p.GuildID()).PlayNext(ctx)
			}
		}),
		disgolink.WithListenerFunc(func(p disgolink.Player, e lavalink.TrackExceptionEvent) {
			log.Printf("[Lavalink Track Exception] Guild %s: %s", p.GuildID(), e.Exception.Message)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = manager.GetPlayer(p.GuildID()).PlayNext(ctx)
		}),
		disgolink.WithListenerFunc(func(p disgolink.Player, e lavalink.TrackStuckEvent) {
			log.Printf("[Lavalink Track Stuck] Guild %s", p.GuildID())
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = manager.GetPlayer(p.GuildID()).PlayNext(ctx)
		}),
		disgolink.WithListenerFunc(func(p disgolink.Player, e lavalink.WebSocketClosedEvent) {
			log.Printf("[Lavalink WS Closed] Guild %s: Code %d %s", p.GuildID(), e.Code, e.Reason)
		}),
	)

	b.Lavalink = lavalinkClient
	manager.Lavalink = lavalinkClient

	activityMgr := activity.New(db, client, manager)
	b.Activity = activityMgr

	manager.SetClientAndActivity(client, activityMgr)

	return b, nil
}

func parseBotIDFromToken(token string) snowflake.ID {
	parts := strings.Split(token, ".")
	if len(parts) > 0 {
		if rawID, err := base64.RawStdEncoding.DecodeString(parts[0]); err == nil {
			if id, err := snowflake.Parse(string(rawID)); err == nil {
				return id
			}
		}
		if rawID, err := base64.StdEncoding.DecodeString(parts[0]); err == nil {
			if id, err := snowflake.Parse(string(rawID)); err == nil {
				return id
			}
		}
	}
	return 0
}

func (b *Bot) Start(ctx context.Context) error {
	// 1. Connect to Lavalink node FIRST so node is ready before gateway events
	nodeConfig := disgolink.NodeConfig{
		Name:     "default",
		Address:  b.Config.LavalinkHost,
		Password: b.Config.LavalinkPassword,
		Secure:   b.Config.LavalinkSecure,
	}

	log.Printf("Conectando ao nó Lavalink (%s)...", b.Config.LavalinkHost)
	node, err := b.Lavalink.AddNode(ctx, nodeConfig)
	if err != nil {
		log.Printf("⚠️ Aviso ao registrar nó Lavalink: %v", err)
	} else {
		log.Printf("✅ Nó Lavalink registrado e conectado: %s", node.Config().Name)
	}

	// 2. Open Discord Gateway
	if err := b.Client.OpenGateway(ctx); err != nil {
		return err
	}

	return nil
}

func (b *Bot) Close(ctx context.Context) {
	if b.Lavalink != nil {
		b.Lavalink.Close()
	}
	if b.Client != nil {
		b.Client.Close(ctx)
	}
	if b.DB != nil {
		_ = b.DB.Close()
	}
}

func (b *Bot) UpdatePresence(trackTitle string, isPlaying bool) {
	if b.Client == nil || b.Client.Gateway == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if isPlaying && trackTitle != "" {
		_ = b.Client.SetPresence(ctx,
			gateway.WithListeningActivity(fmt.Sprintf("%s 🎵", trackTitle)),
			gateway.WithOnlineStatus(discord.OnlineStatusOnline),
		)
	} else {
		_ = b.Client.SetPresence(ctx,
			gateway.WithListeningActivity("música em #music-room 🎶"),
			gateway.WithOnlineStatus(discord.OnlineStatusOnline),
		)
	}
}

