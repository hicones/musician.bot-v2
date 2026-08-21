package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func New(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("falha ao criar diretorio do banco: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("falha ao abrir banco sqlite: %w", err)
	}

	conn.SetMaxOpenConns(1) // SQLite works best with 1 writer connection

	db := &DB{conn: conn}
	if err := db.initSchema(); err != nil {
		return nil, fmt.Errorf("falha ao inicializar schema: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS guild_config (
		guild_id TEXT PRIMARY KEY,
		music_room_id TEXT NOT NULL,
		player_message_id TEXT,
		setup_by TEXT NOT NULL,
		setup_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_updated DATETIME DEFAULT CURRENT_TIMESTAMP,
		status TEXT DEFAULT 'active'
	);

	CREATE TABLE IF NOT EXISTS playlists (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		user_id TEXT NOT NULL,
		guild_id TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS playlist_songs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		playlist_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		url TEXT NOT NULL,
		duration TEXT,
		thumbnail TEXT,
		FOREIGN KEY (playlist_id) REFERENCES playlists (id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS favorite_songs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		guild_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		title TEXT NOT NULL,
		url TEXT NOT NULL,
		duration TEXT,
		thumbnail TEXT,
		favorited_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE UNIQUE INDEX IF NOT EXISTS favorite_songs_url_unique
	ON favorite_songs (url);

	CREATE TABLE IF NOT EXISTS song_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		guild_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		title TEXT NOT NULL,
		url TEXT NOT NULL,
		author TEXT,
		played_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := db.conn.Exec(schema)
	return err
}

// Guild Config

func (db *DB) SaveGuildConfig(cfg GuildConfig) error {
	query := `
	INSERT INTO guild_config (guild_id, music_room_id, player_message_id, setup_by, status, setup_at, last_updated)
	VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	ON CONFLICT(guild_id) DO UPDATE SET
		music_room_id = excluded.music_room_id,
		player_message_id = excluded.player_message_id,
		setup_by = excluded.setup_by,
		status = excluded.status,
		last_updated = CURRENT_TIMESTAMP;
	`
	_, err := db.conn.Exec(query, cfg.GuildID, cfg.MusicRoomID, cfg.PlayerMessageID, cfg.SetupBy, cfg.Status)
	return err
}

func (db *DB) GetGuildConfig(guildID string) (*GuildConfig, error) {
	query := `SELECT guild_id, music_room_id, player_message_id, setup_by, setup_at, last_updated, status FROM guild_config WHERE guild_id = ?`
	row := db.conn.QueryRow(query, guildID)

	var cfg GuildConfig
	err := row.Scan(&cfg.GuildID, &cfg.MusicRoomID, &cfg.PlayerMessageID, &cfg.SetupBy, &cfg.SetupAt, &cfg.LastUpdated, &cfg.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (db *DB) UpdatePlayerMessageID(guildID, messageID string) error {
	query := `UPDATE guild_config SET player_message_id = ?, last_updated = CURRENT_TIMESTAMP WHERE guild_id = ?`
	_, err := db.conn.Exec(query, messageID, guildID)
	return err
}

// Playlists

func (db *DB) SavePlaylist(name, userID, guildID string, songs []SongData) (int64, error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO playlists (name, user_id, guild_id) VALUES (?, ?, ?)`, name, userID, guildID)
	if err != nil {
		return 0, err
	}

	playlistID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	stmt, err := tx.Prepare(`INSERT INTO playlist_songs (playlist_id, title, url, duration, thumbnail) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	for _, song := range songs {
		if _, err := stmt.Exec(playlistID, song.Title, song.URL, song.Duration, song.Thumbnail); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return playlistID, nil
}

func (db *DB) GetPlaylists(guildID string) ([]Playlist, error) {
	query := `SELECT id, name, user_id, guild_id, created_at FROM playlists WHERE guild_id = ? ORDER BY id DESC`
	rows, err := db.conn.Query(query, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Playlist
	for rows.Next() {
		var p Playlist
		if err := rows.Scan(&p.ID, &p.Name, &p.UserID, &p.GuildID, &p.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, nil
}

func (db *DB) GetPlaylist(playlistID int64, guildID string) (*Playlist, error) {
	query := `SELECT id, name, user_id, guild_id, created_at FROM playlists WHERE id = ? AND guild_id = ?`
	row := db.conn.QueryRow(query, playlistID, guildID)

	var p Playlist
	err := row.Scan(&p.ID, &p.Name, &p.UserID, &p.GuildID, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (db *DB) GetPlaylistSongs(playlistID int64) ([]SongData, error) {
	query := `SELECT title, url, duration, thumbnail FROM playlist_songs WHERE playlist_id = ? ORDER BY id ASC`
	rows, err := db.conn.Query(query, playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []SongData
	for rows.Next() {
		var s SongData
		if err := rows.Scan(&s.Title, &s.URL, &s.Duration, &s.Thumbnail); err != nil {
			return nil, err
		}
		songs = append(songs, s)
	}
	return songs, nil
}

func (db *DB) GetPlaylistSongsWithIDs(playlistID int64) ([]PlaylistSong, error) {
	query := `SELECT id, playlist_id, title, url, duration, thumbnail FROM playlist_songs WHERE playlist_id = ? ORDER BY id ASC`
	rows, err := db.conn.Query(query, playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var songs []PlaylistSong
	for rows.Next() {
		var s PlaylistSong
		if err := rows.Scan(&s.ID, &s.PlaylistID, &s.Title, &s.URL, &s.Duration, &s.Thumbnail); err != nil {
			return nil, err
		}
		songs = append(songs, s)
	}
	return songs, nil
}

func (db *DB) AddPlaylistSong(playlistID int64, guildID string, song SongData) (bool, error) {
	playlist, err := db.GetPlaylist(playlistID, guildID)
	if err != nil || playlist == nil {
		return false, err
	}

	query := `INSERT INTO playlist_songs (playlist_id, title, url, duration, thumbnail) VALUES (?, ?, ?, ?, ?)`
	res, err := db.conn.Exec(query, playlistID, song.Title, song.URL, song.Duration, song.Thumbnail)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	return affected > 0, err
}

func (db *DB) DeletePlaylist(playlistID int64, guildID string) (bool, error) {
	playlist, err := db.GetPlaylist(playlistID, guildID)
	if err != nil || playlist == nil {
		return false, err
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM playlist_songs WHERE playlist_id = ?`, playlistID); err != nil {
		return false, err
	}
	res, err := tx.Exec(`DELETE FROM playlists WHERE id = ? AND guild_id = ?`, playlistID, guildID)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}

	affected, err := res.RowsAffected()
	return affected > 0, err
}

func (db *DB) DeletePlaylistSong(playlistID, songID int64, guildID string) (bool, error) {
	playlist, err := db.GetPlaylist(playlistID, guildID)
	if err != nil || playlist == nil {
		return false, err
	}

	query := `DELETE FROM playlist_songs WHERE id = ? AND playlist_id = ?`
	res, err := db.conn.Exec(query, songID, playlistID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	return affected > 0, err
}

// Favorites

func (db *DB) SaveFavoriteSong(guildID, userID, title, url string, duration, thumbnail *string) (bool, error) {
	query := `INSERT OR IGNORE INTO favorite_songs (guild_id, user_id, title, url, duration, thumbnail) VALUES (?, ?, ?, ?, ?, ?)`
	res, err := db.conn.Exec(query, guildID, userID, title, url, duration, thumbnail)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	return affected > 0, err
}

func (db *DB) GetFavoriteSongs(guildID string) ([]FavoriteSong, error) {
	query := `SELECT id, guild_id, user_id, title, url, duration, thumbnail, favorited_at FROM favorite_songs WHERE guild_id = ? ORDER BY favorited_at ASC`
	rows, err := db.conn.Query(query, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []FavoriteSong
	for rows.Next() {
		var s FavoriteSong
		if err := rows.Scan(&s.ID, &s.GuildID, &s.UserID, &s.Title, &s.URL, &s.Duration, &s.Thumbnail, &s.FavoritedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}

func (db *DB) DeleteFavoriteSong(favoriteID int64, guildID string) (bool, error) {
	query := `DELETE FROM favorite_songs WHERE id = ? AND guild_id = ?`
	res, err := db.conn.Exec(query, favoriteID, guildID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	return affected > 0, err
}

// Statistics / History

func (db *DB) RecordSongPlay(guildID, userID, title, url, author string) error {
	query := `INSERT INTO song_history (guild_id, user_id, title, url, author, played_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`
	_, err := db.conn.Exec(query, guildID, userID, title, url, author)
	return err
}

func (db *DB) GetTopSongs(guildID string, limit int) ([]TopSongStat, error) {
	if limit <= 0 {
		limit = 10
	}
	query := `
	SELECT title, url, COALESCE(author, '') as author, COUNT(*) as play_count, MAX(played_at) as last_played
	FROM song_history
	WHERE guild_id = ?
	GROUP BY url, title
	ORDER BY play_count DESC, last_played DESC
	LIMIT ?
	`
	rows, err := db.conn.Query(query, guildID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []TopSongStat
	for rows.Next() {
		var s TopSongStat
		if err := rows.Scan(&s.Title, &s.URL, &s.Author, &s.PlayCount, &s.LastPlayed); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}

func (db *DB) GetTopUsers(guildID string, limit int) ([]TopUserStat, error) {
	if limit <= 0 {
		limit = 10
	}
	query := `
	SELECT user_id, COUNT(*) as play_count, MAX(played_at) as last_played
	FROM song_history
	WHERE guild_id = ?
	GROUP BY user_id
	ORDER BY play_count DESC, last_played DESC
	LIMIT ?
	`
	rows, err := db.conn.Query(query, guildID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []TopUserStat
	for rows.Next() {
		var u TopUserStat
		if err := rows.Scan(&u.UserID, &u.PlayCount, &u.LastPlayed); err != nil {
			return nil, err
		}
		list = append(list, u)
	}
	return list, nil
}

