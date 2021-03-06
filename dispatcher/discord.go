package dispatcher

import (
	cryptoRand "crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"io"
	"log"
	"math/rand"
	"narval/launchers"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"
)

type messageEvent struct {
	session *discordgo.Session
	message *discordgo.MessageCreate
	command []string
}

type dispatcher interface {
	setup(messageEvent) error
	play(messageEvent) error
}

func RunDispatcher() {
	err := loadSettings()
	if err != nil {
		log.Panic(err)
	}

	discord, err := discordgo.New("Bot " + os.Getenv("DISCORD_TOKEN"))
	if err != nil {
		log.Panic(err)
	}
	discord.AddHandler(messageCreate)
	discord.Identify.Intents = discordgo.IntentsDirectMessages | discordgo.IntentsGuildMessages

	err = discord.Open()
	if err != nil {
		log.Panic(err)
	}
	defer launchers.CloseDontCare(discord)

	// We are done; just wait for exit
	fmt.Println("Bot is running. Ctrl-C to exit.")
	signalsChannel := make(chan os.Signal, 1)
	signal.Notify(signalsChannel, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-signalsChannel
}

func init() {
	// Try to seed the RNG with a cryptographically good 64-bit number
	// https://stackoverflow.com/a/54491783/98029
	var buffer [8]byte
	_, err := cryptoRand.Read(buffer[:])
	var seed int64
	if err != nil {
		seed = time.Now().UnixNano() ^ -0xbeef1e57b00b1e5
	} else {
		seed = int64(binary.LittleEndian.Uint64(buffer[:]))
	}
	rand.Seed(seed)
}

func randString() string {
	var buffer [6]byte
	var result [8]byte
	rand.Read(buffer[:])
	base64.URLEncoding.Encode(result[:], buffer[:])
	return string(result[:])
}

func messageCreate(session *discordgo.Session, message *discordgo.MessageCreate) {
	if message.Author.ID == session.State.User.ID { // self messages
		return
	}

	if len(message.Attachments) > 0 {
		for _, v := range message.Attachments {
			log.Printf("%v", v)
		}
	} else if len(message.Content) > 1 && message.Content[:1] == ">" {
		command := strings.Split(message.Content[1:], " ")
		event := messageEvent{session, message, command}
		err := event.commands()
		if err == errUnauthorized {
			_ = event.react(":unamused:")
		} else if err != nil {
			log.Printf("Message errored out: %s", err)
			_ = event.react(":warning:")
		}
	}
}

func (event messageEvent) commands() error {
	// commands
	switch event.command[0] {
	case "opme":
		return event.commandOpme()
	case "aws":
		return event.commandAws()
	case "setup":
		return event.commandSetup()
	case "play":
		return event.commandPlay()
	default:
		return nil
	}
}

func (event messageEvent) commandOpme() error {
	user := store.user(event.message.Author.ID)
	if len(event.command) == 1 {
		if user.IsAdmin {
			return event.react(":white_check_mark:")
		} else {
			author := event.message.Author
			user.confirmation = randString() + randString() + randString()
			log.Printf("Tell %s#%s >opme %s", author.Username, author.Discriminator, user.confirmation)
			return event.react(":thinking:")
		}
	} else {
		password := event.command[1]
		if password == user.confirmation {
			user.IsAdmin = true
			store.store()
			return event.react(":white_check_mark:")
		} else {
			return errUnauthorized
		}
	}
}

func (event messageEvent) commandAws() error {
	user := store.user(event.message.Author.ID)
	if !user.IsAdmin {
		return errUnauthorized
	}
	if len(event.command) != 3 {
		return event.reply("Expected: `>aws region-name bucket-name`")
	}
	guild := store.guild(event.message.GuildID)
	guild.Region = event.command[1]
	guild.Bucket = event.command[2]
	store.store()
	return event.react(":white_check_mark:")
}

func (event messageEvent) commandSetup() error {
	user := store.user(event.message.Author.ID)
	if !user.IsAdmin {
		return errUnauthorized
	}
	channel := store.channel(event.message.ChannelID)
	if channel.SetupComplete {
		return event.reply(fmt.Sprintf("Already set up %s.", channel.Game))
	}

	if len(event.command) > 1 {
		channel.Game = event.command[1]
		channel.dispatcher = allDispatchers[channel.Game]
	}
	if channel.dispatcher == nil {
		return event.reply("Try `>setup factorio`")
	} else {
		return channel.dispatcher.setup(event)
	}
}

func (event messageEvent) commandPlay() error {
	channel := store.channel(event.message.ChannelID)
	if channel.dispatcher != nil {
		return channel.dispatcher.play(event)
	} else {
		return event.react(":shrug:")
	}
}

func (event messageEvent) reply(message string) error {
	_, err := event.session.ChannelMessageSendReply(event.message.ChannelID, message, event.message.Reference())
	return err
}

func (event messageEvent) react(emoji string) error {
	return event.session.MessageReactionAdd(event.message.ChannelID, event.message.ID, emoji)
}

func (event messageEvent) putS3file(filename string, reader io.Reader) error {
	guild := store.guild(event.message.GuildID)
	key := path.Join(event.message.ChannelID, filename)
	return s3upload(guild, key, reader)
}
