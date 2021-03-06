package dispatcher

import (
	"bytes"
	"encoding/json"
	"strings"
)

func init() {
	allDispatchers["factorio"] = factorioDispatcher{}
}

type jsObj map[string]interface{}

type factorioDispatcher struct{}

func (factorioDispatcher) setup(event messageEvent) error {
	// we're not yet using this file, this is just to see how to do it
	initFile, err := json.Marshal(jsObj{"launch": "factorio"})
	if err != nil {
		return err
	}
	err = event.putS3file("init.json", bytes.NewReader(initFile))
	if err != nil {
		return err
	}

	// respond :D
	message := []string{
		"All right, let's build an awesome factory!",
		// "If you want an initial save game, send your save zip file.",
		// "If you want mods, zip your `%appdata%\\Factorio\\mods` folder and send it over.",
		// "Some server json files are accepted too, including world settings with world seed.",
		"When you are ready, say `>start`",
	}
	return event.reply(strings.Join(message, "\n"))
}

func (factorioDispatcher) play(event messageEvent) error {
	guild := store.guild(event.message.GuildID)
	channel := store.channel(event.message.ChannelID)
	channel.session = randString()
	err := s3uploadSelf(guild)
	if err != nil {
		return err
	}
	variables := map[string]string{
		"LAUNCH":  "factorio",
		"BUCKET":  guild.Bucket,
		"PREFIX":  channel.Prefix,
		"SESSION": channel.session,
	}
	return ec2makeServer(guild, variables)
}
