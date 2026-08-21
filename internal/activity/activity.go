package activity

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/snowflake/v2"

	"musician-bot-v2/internal/database"
)

type AudioService interface {
	StartRadio(ctx context.Context, guildID snowflake.ID) (int, error)
	LeaveVoice(ctx context.Context, guildID snowflake.ID) error
	IsRadioActive(guildID snowflake.ID) bool
}

type ActivityManager struct {
	mu                   sync.Mutex
	inactivityTimers     map[snowflake.ID]*time.Timer
	emptyVoiceTimers     map[snowflake.ID]*time.Timer
	db                   *database.DB
	client               *bot.Client
	audio                AudioService
	inactivityDuration   time.Duration
	emptyChannelDuration time.Duration
}

func New(db *database.DB, client *bot.Client, audio AudioService) *ActivityManager {
	return &ActivityManager{
		inactivityTimers:     make(map[snowflake.ID]*time.Timer),
		emptyVoiceTimers:     make(map[snowflake.ID]*time.Timer),
		db:                   db,
		client:               client,
		audio:                audio,
		inactivityDuration:   3 * time.Minute,
		emptyChannelDuration: 3 * time.Minute,
	}
}

func (a *ActivityManager) SetClient(client *bot.Client) {
	a.client = client
}

func (a *ActivityManager) OnTrackStarted(guildID snowflake.ID) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if timer, ok := a.inactivityTimers[guildID]; ok {
		timer.Stop()
		delete(a.inactivityTimers, guildID)
	}
}

func (a *ActivityManager) OnQueueFinished(guildID snowflake.ID) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if timer, ok := a.inactivityTimers[guildID]; ok {
		timer.Stop()
	}

	a.inactivityTimers[guildID] = time.AfterFunc(a.inactivityDuration, func() {
		a.handleInactivityTimeout(guildID)
	})
	log.Printf("[Activity] Monitorando inatividade no servidor %s (timeout: 3m)", guildID)
}

func (a *ActivityManager) handleInactivityTimeout(guildID snowflake.ID) {
	a.mu.Lock()
	delete(a.inactivityTimers, guildID)
	a.mu.Unlock()

	if a.audio.IsRadioActive(guildID) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loaded, err := a.audio.StartRadio(ctx, guildID)
	if err != nil {
		log.Printf("[Activity] Erro ao iniciar rádio por inatividade em %s: %v", guildID, err)
		return
	}

	if loaded > 0 {
		log.Printf("[Activity] Rádio iniciada por inatividade com %d música(s) em %s", loaded, guildID)
	}
}

func (a *ActivityManager) HandleVoiceStateUpdate(guildID snowflake.ID, voiceChannelID *snowflake.ID, humanMembersCount int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if voiceChannelID == nil || humanMembersCount > 0 {
		if timer, ok := a.emptyVoiceTimers[guildID]; ok {
			timer.Stop()
			delete(a.emptyVoiceTimers, guildID)
		}
		return
	}

	// Voice channel is empty of humans
	if _, ok := a.emptyVoiceTimers[guildID]; ok {
		return
	}

	log.Printf("[Voice] Canal de voz vazio no servidor %s. Desconexão agendada para 3 minutos.", guildID)
	a.emptyVoiceTimers[guildID] = time.AfterFunc(a.emptyChannelDuration, func() {
		a.handleEmptyVoiceTimeout(guildID)
	})
}

func (a *ActivityManager) handleEmptyVoiceTimeout(guildID snowflake.ID) {
	a.mu.Lock()
	delete(a.emptyVoiceTimers, guildID)
	a.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Printf("[Voice] Desconectando do servidor %s por inatividade de usuários na call.", guildID)
	_ = a.audio.LeaveVoice(ctx, guildID)
}

func (a *ActivityManager) Clear(guildID snowflake.ID) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if timer, ok := a.inactivityTimers[guildID]; ok {
		timer.Stop()
		delete(a.inactivityTimers, guildID)
	}
	if timer, ok := a.emptyVoiceTimers[guildID]; ok {
		timer.Stop()
		delete(a.emptyVoiceTimers, guildID)
	}
}

func ShuffleSlice[T any](slice []T) []T {
	result := make([]T, len(slice))
	copy(result, slice)
	for i := len(result) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		result[i], result[j] = result[j], result[i]
	}
	return result
}
