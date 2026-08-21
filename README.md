# 🎵 Musician Bot v2

Bot de música para Discord moderno, de altíssima performance e baixa latência, reescrito em **Go (Golang)** com **Lavalink v4** como motor de áudio dedicado.

---

## ✨ Destaques da versão v2

- **Motor de Áudio Dedicado (Lavalink v4)**: Desacoplamento total do processamento de áudio do bot. O streaming é transmitido em alta fidelidade via UDP/RTP com os plugins `youtube-plugin` e `lavasrc` (Spotify, Apple Music, Deezer).
- **Consumo Mínimo de Recursos**: Binário compilado nativo em Go consumindo menos de 25MB de memória RAM.
- **Canal Dedicado (`#music-room`)**: Interface interativa em tempo real. Digite links ou nomes de músicas no canal para tocar automaticamente (as mensagens de texto são apagadas para manter o canal limpo).
- **Controles por Reações & Botões**:
  - Reações no Embed: ⏮️ (Anterior), ▶️ (Play/Pause), ⏭️ (Pular), ⏹️ (Parar), 🔀 (Embaralhar), 🔁 (Modo Loop: Off / Faixa / Fila), ⭐ (Favoritar), 🏠 (Sair).
  - Botões: 📋 Ver Fila (com paginação), 🎵 Tocar Playlist, 📻 Rádio de Favoritos.
- **Modo Rádio & Monitor de Inatividade**: Inicia automaticamente a rádio com favoritos em loop aleatório após 3 minutos de inatividade da fila. Desconecta da chamada caso o canal fique vazio.
- **Compatibilidade 100% com Banco SQLite da v1**: Preserva todas as playlists, músicas favoritas e configurações de servidores já salvas no `data/database.sqlite`.
- **Slash Commands Completos**:
  - `/setup`: Criação e configuração do canal `#music-room` com banner visual e player.
  - `/add-current-to-playlist`: Salva a faixa atual em uma playlist existente ou cria uma nova.
  - `/delete-playlist` e `/delete-playlist-song`: Exclusão segura com autocomplete e modal de confirmação.
  - `/remove-favorite`: Remoção de faixas dos favoritos com autocomplete.
  - `/export-playlist` e `/import-playlist`: Exportação e importação de playlists via arquivo `.csv`.

---

## 🛠️ Stack Tecnológica

| Componente | Tecnologia |
| :--- | :--- |
| **Linguagem** | Go 1.24+ |
| **Framework Discord** | [Disgo](https://github.com/disgoorg/disgo) (Discord API v10) |
| **Cliente Lavalink** | [Disgolink v3](https://github.com/disgoorg/disgolink) (Lavalink v4 client) |
| **Audio Server** | [Lavalink v4](https://github.com/lavalink-devs/Lavalink) + JVM 21 |
| **Plugins Lavalink** | `youtube-plugin`, `lavasrc` (Spotify/Apple Music), `sponsorblock` |
| **Banco de Dados** | SQLite via [modernc.org/sqlite](https://modernc.org/sqlite) (Pure Go sem CGO) |
| **Deploy** | Docker & Docker Compose |

---

## 📁 Estrutura do Projeto

```
musician.bot-v2/
├── cmd/
│   └── bot/
│       └── main.go          # Entry point e lifecycle do bot
├── internal/
│   ├── activity/            # Monitor de inatividade e rádio automática
│   │   └── activity.go
│   ├── audio/               # Integração Disgolink / Lavalink e Filas
│   │   ├── manager.go
│   │   ├── player.go
│   │   └── queue.go
│   ├── config/              # Variáveis de ambiente
│   │   └── config.go
│   ├── database/            # SQLite migrations e queries
│   │   ├── db.go
│   │   └── models.go
│   ├── discord/             # Handlers de eventos do Discord
│   │   ├── bot.go
│   │   ├── buttons.go
│   │   ├── commands.go
│   │   ├── messages.go
│   │   ├── modals.go
│   │   ├── reactions.go
│   │   ├── selects.go
│   │   └── voice.go
│   └── ui/                  # Embeds e botões interativos
│       └── player_embed.go
├── assets/                  # banner.png e placeholder.png
├── lavalink/
│   └── application.yml      # Configuração do Lavalink v4 e plugins
├── Dockerfile               # Multi-stage build Go + Alpine
├── docker-compose.yml       # Orquestração do Bot + Lavalink v4
├── go.mod
└── go.sum
```

---

## 🚀 Como Executar

### 1. Pré-requisitos
- [Docker](https://www.docker.com/) e Docker Compose instalados.
- Token de Bot do [Discord Developer Portal](https://discord.com/developers/applications) com as seguintes **Privileged Gateway Intents**:
  - `Server Members Intent`
  - `Message Content Intent`

### 2. Configurar o `.env`
Copie o exemplo de variáveis de ambiente:
```bash
cp .env.example .env
```

Edite o arquivo `.env`:
```env
DISCORD_TOKEN=seu_token_do_bot_aqui
ADMIN_USER_IDS=seu_discord_user_id
```

### 3. Rodar via Docker Compose (Recomendado)
```bash
# Iniciar o Lavalink v4 e o Bot
docker-compose up -d

# Visualizar logs
docker-compose logs -f
```

---

## 🕹️ Desenvolvimento Local (Sem Docker)

Se desejar rodar o binário Go localmente:

1. Inicie o Lavalink v4:
```bash
docker-compose up -d lavalink
```

2. Compile e execute o bot Go:
```bash
go run ./cmd/bot
```
