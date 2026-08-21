package discord

import (
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"

	"musician-bot-v2/internal/activity"
	"musician-bot-v2/internal/audio"
)

type VoiceHandler struct {
	activity *activity.ActivityManager
	manager  *audio.Manager
}

func NewVoiceHandler(act *activity.ActivityManager, manager *audio.Manager) *VoiceHandler {
	return &VoiceHandler{
		activity: act,
		manager:  manager,
	}
}

func (h *VoiceHandler) Handle(event *events.GuildVoiceStateUpdate) {
	guildID := event.VoiceState.GuildID
	selfID := event.Client().ID()

	// Check if bot is connected in this guild
	botVoiceState, ok := event.Client().Caches.VoiceState(guildID, selfID)
	if !ok || botVoiceState.ChannelID == nil {
		h.activity.HandleVoiceStateUpdate(guildID, nil, 0)
		return
	}

	botChannelID := *botVoiceState.ChannelID
	var affectedChannel snowflake.ID
	if event.OldVoiceState.ChannelID != nil {
		affectedChannel = *event.OldVoiceState.ChannelID
	}
	if event.VoiceState.ChannelID != nil {
		affectedChannel = *event.VoiceState.ChannelID
	}

	if affectedChannel != botChannelID {
		return
	}

	// Count humans in the bot's channel
	allStates := event.Client().Caches.VoiceStates(guildID)
	humanCount := 0
	for vs := range allStates {
		if vs.ChannelID != nil && *vs.ChannelID == botChannelID {
			if member, ok := event.Client().Caches.Member(guildID, vs.UserID); ok {
				if !member.User.Bot {
					humanCount++
				}
			}
		}
	}

	h.activity.HandleVoiceStateUpdate(guildID, &botChannelID, humanCount)
}
