package audio

import (
	"context"
	"fmt"
	"log"

	"github.com/disgoorg/disgolink/v3/disgolink"
	"github.com/disgoorg/disgolink/v3/lavalink"
	"github.com/disgoorg/snowflake/v2"

	"musician-bot-v2/internal/ui"
)

type Player struct {
	GuildID      snowflake.ID
	Queue        *TrackQueue
	activeFilter string
	manager      *Manager
}

func NewPlayer(guildID snowflake.ID, manager *Manager) *Player {
	return &Player{
		GuildID: guildID,
		Queue:   NewTrackQueue(),
		manager: manager,
	}
}

func (p *Player) getLavalinkPlayer() (disgolink.Player, error) {
	bestNode := p.manager.Lavalink.BestNode()
	if bestNode == nil {
		return nil, fmt.Errorf("nenhum nó Lavalink disponível")
	}

	lp := p.manager.Lavalink.ExistingPlayer(p.GuildID)
	if lp == nil || lp.Node() == nil {
		p.manager.Lavalink.RemovePlayer(p.GuildID)
		lp = p.manager.Lavalink.PlayerOnNode(bestNode, p.GuildID)
	}
	return lp, nil
}

func (p *Player) PlayNext(ctx context.Context) error {
	nextItem := p.Queue.Pop()
	lavalinkPlayer, err := p.getLavalinkPlayer()
	if err != nil {
		log.Printf("[Player] Erro: %v", err)
		return err
	}

	if nextItem == nil {
		p.Queue.SetCurrent(nil)
		// Stop track on lavalink
		_ = lavalinkPlayer.Update(ctx, lavalink.WithNullTrack())
		p.manager.UpdatePlayerMessage(p.GuildID)
		p.manager.OnQueueFinished(p.GuildID)
		return nil
	}

	err = lavalinkPlayer.Update(ctx, lavalink.WithTrack(nextItem.Track))
	if err != nil {
		log.Printf("[Player] Erro ao tocar faixa '%s': %v", nextItem.Track.Info.Title, err)
		// Try next
		return p.PlayNext(ctx)
	}

	p.manager.UpdatePlayerMessage(p.GuildID)
	p.manager.OnTrackStarted(p.GuildID)
	return nil
}

func (p *Player) PlayPrevious(ctx context.Context) error {
	prevItem := p.Queue.Previous()
	if prevItem == nil {
		return fmt.Errorf("nenhuma música no histórico")
	}

	lavalinkPlayer, err := p.getLavalinkPlayer()
	if err != nil {
		return err
	}

	err = lavalinkPlayer.Update(ctx, lavalink.WithTrack(prevItem.Track))
	if err != nil {
		return err
	}

	p.manager.UpdatePlayerMessage(p.GuildID)
	p.manager.OnTrackStarted(p.GuildID)
	return nil
}

func (p *Player) Pause(ctx context.Context, pause bool) error {
	p.Queue.SetPaused(pause)
	lavalinkPlayer, err := p.getLavalinkPlayer()
	if err != nil {
		return err
	}

	err = lavalinkPlayer.Update(ctx, lavalink.WithPaused(pause))
	if err == nil {
		p.manager.UpdatePlayerMessage(p.GuildID)
	}
	return err
}

func (p *Player) Skip(ctx context.Context) error {
	return p.PlayNext(ctx)
}

func (p *Player) Stop(ctx context.Context) error {
	p.Queue.Clear()
	p.Queue.ClearHistory()
	p.Queue.SetPaused(false)
	p.Queue.SetRepeatMode(ui.RepeatModeOff)

	if lavalinkPlayer, err := p.getLavalinkPlayer(); err == nil {
		_ = lavalinkPlayer.Update(ctx, lavalink.WithNullTrack())
	}

	p.manager.UpdatePlayerMessage(p.GuildID)
	p.manager.OnQueueFinished(p.GuildID)
	return nil
}

func (p *Player) ActiveFilter() string {
	return p.activeFilter
}

func (p *Player) SetFilter(ctx context.Context, filterType string) error {
	lavalinkPlayer, err := p.getLavalinkPlayer()
	if err != nil {
		return err
	}

	var filters lavalink.Filters

	switch filterType {
	case "bass_boost_low":
		var eq lavalink.Equalizer
		eq[0] = 0.25
		eq[1] = 0.15
		filters.Equalizer = &eq
	case "bass_boost_medium":
		var eq lavalink.Equalizer
		eq[0] = 0.40
		eq[1] = 0.25
		eq[2] = 0.15
		filters.Equalizer = &eq
	case "bass_boost_high":
		var eq lavalink.Equalizer
		eq[0] = 0.60
		eq[1] = 0.40
		eq[2] = 0.20
		eq[3] = 0.10
		filters.Equalizer = &eq
	case "nightcore":
		filters.Timescale = &lavalink.Timescale{
			Speed: 1.25,
			Pitch: 1.25,
			Rate:  1.0,
		}
	case "vaporwave":
		filters.Timescale = &lavalink.Timescale{
			Speed: 0.85,
			Pitch: 0.80,
			Rate:  1.0,
		}
	case "8d":
		filters.Rotation = &lavalink.Rotation{
			RotationHz: 1,
		}
	case "karaoke":
		filters.Karaoke = &lavalink.Karaoke{
			Level:       1.0,
			MonoLevel:   1.0,
			FilterBand:  220.0,
			FilterWidth: 100.0,
		}
	case "clear", "":
		filterType = ""
		// Empty filters resets all DSP
	}

	err = lavalinkPlayer.Update(ctx, lavalink.WithFilters(filters))
	if err != nil {
		return err
	}

	p.activeFilter = filterType
	p.manager.UpdatePlayerMessage(p.GuildID)
	return nil
}

