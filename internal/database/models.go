package database

import "time"

type GuildConfig struct {
	GuildID         string    `json:"guild_id"`
	MusicRoomID     string    `json:"music_room_id"`
	PlayerMessageID *string   `json:"player_message_id"`
	SetupBy         string    `json:"setup_by"`
	SetupAt         time.Time `json:"setup_at"`
	LastUpdated     time.Time `json:"last_updated"`
	Status          string    `json:"status"`
}

type Playlist struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	UserID    string    `json:"user_id"`
	GuildID   string    `json:"guild_id"`
	CreatedAt time.Time `json:"created_at"`
}

type SongData struct {
	Title     string  `json:"title"`
	URL       string  `json:"url"`
	Duration  *string `json:"duration,omitempty"`
	Thumbnail *string `json:"thumbnail,omitempty"`
}

type PlaylistSong struct {
	ID         int64   `json:"id"`
	PlaylistID int64   `json:"playlist_id"`
	Title      string  `json:"title"`
	URL        string  `json:"url"`
	Duration   *string `json:"duration,omitempty"`
	Thumbnail  *string `json:"thumbnail,omitempty"`
}

type FavoriteSong struct {
	ID          int64     `json:"id"`
	GuildID     string    `json:"guild_id"`
	UserID      string    `json:"user_id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Duration    *string   `json:"duration,omitempty"`
	Thumbnail   *string   `json:"thumbnail,omitempty"`
	FavoritedAt time.Time `json:"favorited_at"`
}

type TopSongStat struct {
	Title      string `json:"title"`
	URL        string `json:"url"`
	Author     string `json:"author"`
	PlayCount  int    `json:"play_count"`
	LastPlayed time.Time `json:"last_played"`
}

type TopUserStat struct {
	UserID     string `json:"user_id"`
	PlayCount  int    `json:"play_count"`
	LastPlayed time.Time `json:"last_played"`
}
