package discord

import (
	"context"
	"fmt"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

// SendTemporaryFeedback sends a notification pinging the user and automatically deletes it after 8 seconds.
func SendTemporaryFeedback(client *bot.Client, channelID, userID snowflake.ID, message string) {
	go func() {
		msgCreate := discord.MessageCreate{
			Content: fmt.Sprintf("<@%s>, %s", userID, message),
			AllowedMentions: &discord.AllowedMentions{
				Users: []snowflake.ID{userID},
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		sentMsg, err := client.Rest.CreateMessage(channelID, msgCreate, rest.WithCtx(ctx))
		if err != nil {
			return
		}

		time.Sleep(8 * time.Second)

		delCtx, delCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer delCancel()

		_ = client.Rest.DeleteMessage(channelID, sentMsg.ID, rest.WithCtx(delCtx))
	}()
}
