package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/thehivebot/hive/pkg/db"

	"github.com/bwmarrin/discordgo"

	"github.com/kelseyhightower/envconfig"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(NewCleanCmd())
}

type cleanCmdOptions struct {
	Token string

	MongoDBURL string `envconfig:"MONGODB_URL"`
	MongoDBDB  string `envconfig:"MONGODB_DB"`
	ConfigPath string `default:"./config.json" envconfig:"CONFIG"`

	dg           *discordgo.Session
	shouldRemove map[string]bool
	db           db.Database
}

// NewCleanCmd generates the `clean` command
func NewCleanCmd() *cobra.Command {
	s := cleanCmdOptions{}
	c := &cobra.Command{
		Use:     "clean",
		Short:   "Run the voice channel cleaner",
		Long:    `This is a separate instance cleaning unused channels in The Hive`,
		RunE:    s.RunE,
		PreRunE: s.Validate,
	}

	// TODO: switch to viper
	err := envconfig.Process("hive", &s)
	if err != nil {
		log.Fatalf("Error processing envvars: %q\n", err)
	}

	return c
}

func (v *cleanCmdOptions) Validate(cmd *cobra.Command, args []string) error {
	if v.Token == "" {
		return errors.New("No token specified")
	}

	return nil
}

func (v *cleanCmdOptions) RunE(cmd *cobra.Command, args []string) error {
	log.Println("Starting Cleaner...")

	var err error
	if v.MongoDBDB != "" {
		v.db, err = db.NewMongoDB(v.MongoDBURL, v.MongoDBDB)
		if err != nil {
			return err
		}
	} else {
		// local fallback
		v.db, err = db.NewLocalDB(v.ConfigPath)
		if err != nil {
			return err
		}
	}

	dg, err := discordgo.New("Bot " + v.Token)
	if err != nil {
		return fmt.Errorf("error creating Discord session: %w", err)
	}

	dg.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsGuildPresences)
	dg.StateEnabled = true
	dg.State.TrackVoice = true

	err = dg.Open()
	if err != nil {
		return fmt.Errorf("error opening Discord session: %w", err)
	}

	v.dg = dg

	// small in memory structure to keep candidates to remove
	v.shouldRemove = map[string]bool{}

	go func() {
		for {
			time.Sleep(300 * time.Second)

			guilds, err := dg.UserGuilds(100, "", "")
			for len(guilds) > 0 {
				for _, guild := range guilds {
					log.Println("Checking", guild.Name)
					v.checkGuild(guild.ID)
				}
				guilds, err = dg.UserGuilds(100, "", guilds[len(guilds)-1].ID)
				if err != nil {
					break
				}
			}
		}
	}()

	log.Println("Hive Cleanup is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	return nil
}

func (v *cleanCmdOptions) checkGuild(guildID string) {
	state, err := v.dg.State.Guild(guildID)
	if err != nil {
		log.Println(err)
		return
	}

	channels, err := v.dg.GuildChannels(guildID)
	if err != nil {
		log.Println(err)
		return
	}

	conf, err := v.db.ConfigForGuild(guildID)
	if err != nil {
		log.Println(err)
		return
	}

	if conf == nil || len(conf.Hives) == 0 {
		// hive guser
		return
	}

	for _, channel := range channels {
		if conf, isHive, _ := v.getConfigForRequestCategory(conf, channel); isHive && channel.Type == discordgo.ChannelTypeGuildVoice {
			if conf.Prefix != "" {
				if !strings.HasPrefix(channel.Name, conf.Prefix) {
					continue
				}
			}
			log.Println("looking at", channel.Name)
			inUse := false
			for _, vs := range state.VoiceStates {
				if vs.ChannelID == channel.ID {
					inUse = true
					log.Println(channel.Name, "is in use")
					break
				}
			}

			if !inUse {
				log.Println(channel.Name, "looks sus")
			}

			// on first occurance: mark to remove, on second occurance remove
			if wasMarkedAsRemove := v.shouldRemove[channel.ID]; wasMarkedAsRemove && !inUse {
				log.Println("Deleting", channel.ID, channel.Name)
				j, err := v.dg.Channel(conf.JunkyardCategoryID)
				if err != nil {
					log.Println(err)
					continue
				}

				if conf.JunkyardCategoryID != "" {
					// we have a junkyard so we move the channel
					_, err = v.dg.ChannelEditComplex(channel.ID, &discordgo.ChannelEdit{
						ParentID:             conf.JunkyardCategoryID,
						PermissionOverwrites: j.PermissionOverwrites,
					})
					if err != nil {
						log.Println(err)
					}
				} else {
					_, err := v.dg.ChannelDelete(channel.ID)
					if err != nil {
						log.Println(err)
						continue
					}
				}
				delete(v.shouldRemove, channel.ID)
			}

			v.shouldRemove[channel.ID] = !inUse
		}
	}
}

func (v *cleanCmdOptions) getConfigForRequestCategory(conf *db.Configuration, channel *discordgo.Channel) (*db.HiveConfiguration, bool, error) {
	for _, hive := range conf.Hives {
		if channel.ParentID == hive.VoiceCategoryID || channel.ParentID == hive.TextCategoryID {
			return &hive, true, nil
		}
	}

	// no hive found
	return nil, false, nil
}
