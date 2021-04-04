package hive

import (
	"fmt"
	"log"
	"strings"

	"github.com/thehivebot/hive/pkg/db"

	"github.com/thehivebot/hive/pkg/embed"

	"github.com/bwmarrin/discordgo"
	"github.com/thehivebot/hive/pkg/command"
)

// HiveCommand contains the handlers for the Hive commandos
type HiveCommand struct {
	db db.Database
}

// NewHiveCommand gives a new HiveCommand
func NewHiveCommand(dbConn db.Database) *HiveCommand {
	return &HiveCommand{
		db: dbConn,
	}
}

// Register registers the handlers
func (h *HiveCommand) Register(registry command.Registry, server command.Server) {
	registry.RegisterMessageReactionAddHandler(h.handleReaction)

	registry.RegisterInteractionCreate("hive", h.HiveCommand)
}

// InstallSlashCommands registers the slash commands handlers
func (h *HiveCommand) InstallSlashCommands(session *discordgo.Session) error {
	_, err := session.ApplicationCommandCreate(session.State.SessionID, "", &discordgo.ApplicationCommand{
		Name:        "hive",
		Description: "creates on-remand voice and text channels",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Name:        "type",
				Description: "type of channel",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "text",
						Description: "text channel",
						Required:    false,
						Choices:     nil,
						Options: []*discordgo.ApplicationCommandOption{
							{
								Type:        discordgo.ApplicationCommandOptionString,
								Name:        "name",
								Description: "name of channel",
								Required:    true,
							},
							{
								Type:        discordgo.ApplicationCommandOptionBoolean,
								Name:        "hidden",
								Description: "is channel not visible for everyone",
								Required:    true,
							},
						},
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "voice",
						Description: "voice channel",
						Required:    false,
						Choices:     nil,
						Options: []*discordgo.ApplicationCommandOption{
							{
								Type:        discordgo.ApplicationCommandOptionString,
								Name:        "name",
								Description: "name of channel",
								Required:    true,
							},
							{
								Type:        discordgo.ApplicationCommandOptionInteger,
								Name:        "size",
								Description: "number of allowed users (1-99)",
								Required:    true,
							},
						},
					},
				},
			},
		},
	})

	return err
}

func (h *HiveCommand) HiveCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
	if err != nil {
		log.Println(err)
		return
	}

	hidden := false
	istext := false
	var size int
	var name string
	if len(i.Data.Options) < 1 || len(i.Data.Options[0].Options) < 1 || len(i.Data.Options[0].Options[0].Options) < 2 {
		log.Println("invalid cmd")
		return // invalid
	}

	if i.Data.Options[0].Options[0].Name == "text" {
		istext = true
	}

	conf, ok := h.precheck(s, i.GuildID, i.ChannelID)
	if !ok {
		return
	}

	for _, option := range i.Data.Options[0].Options[0].Options {
		switch option.Name {
		case "name":
			name, ok = option.Value.(string)
			if !ok {
				return
			}
		case "size":
			fVal, ok := option.Value.(float64)
			if !ok {
				return
			}
			size = int(fVal)
		case "hidden":
			hidden, ok = option.Value.(bool)
			if !ok {
				return
			}
		}
	}

	h.createChannel(s, i.GuildID, i.ChannelID, i.Member.User.ID, name, istext, hidden, conf, size)
}

func (h *HiveCommand) precheck(s *discordgo.Session, guildID, channelID string) (*db.HiveConfiguration, bool) {
	conf, isHive, err := h.getConfigForRequestChannel(guildID, channelID)
	if err != nil {
		log.Println(err)
		return nil, false
	}
	if !isHive {
		s.ChannelMessageSend(channelID, "This command only works in the Requests channels")
		return nil, false
	}

	return conf, true
}

func (h *HiveCommand) createChannel(s *discordgo.Session, guildID, channelID, userID, name string, isText, hidden bool, conf *db.HiveConfiguration, size int) {
	var newChan *discordgo.Channel
	var err error
	if isText {
		newChan, err = h.createTextChannel(s, conf, name, conf.TextCategoryID, userID, guildID, hidden)
	} else {
		newChan, err = h.createVoiceChannel(s, conf, name, conf.VoiceCategoryID, userID, guildID, size, hidden)
	}

	if err != nil {
		s.ChannelMessageSend(channelID, err.Error())
		return
	}

	s.ChannelMessageSend(channelID, "Channel created! Have fun! Reminder: I will delete it when it stays empty for a while")
	if isText {
		s.ChannelMessageSend(newChan.ID, "Welcome to your text channel! If you're finished using this please say `tm!archive`")
	}

	if hidden {
		s.ChannelMessageSend(channelID, fmt.Sprintf("your channel is hidden, react 👋 below to join"))
		e := embed.NewEmbed()
		e.SetTitle("Hive Channel")
		e.AddField("name", conf.Prefix+name)
		e.AddField("id", newChan.ID)

		msg, err := s.ChannelMessageSendEmbed(channelID, e.MessageEmbed)
		if err != nil {
			log.Println(err)
		}
		s.MessageReactionAdd(channelID, msg.ID, "👋")
	}
}

func (h *HiveCommand) createTextChannel(s *discordgo.Session, conf *db.HiveConfiguration, name, catID, userID, guildID string, hidden bool) (*discordgo.Channel, error) {
	cat, err := s.Channel(catID)
	if err != nil {
		return nil, err
	}
	props := discordgo.GuildChannelCreateData{
		Name:                 conf.Prefix + name,
		Type:                 discordgo.ChannelTypeGuildText,
		Position:             99,
		ParentID:             catID,
		NSFW:                 false,
		PermissionOverwrites: cat.PermissionOverwrites,
	}

	if hidden {
		// admin privileges on channel for creator
		var allow int64
		allow |= discordgo.PermissionReadMessageHistory
		allow |= discordgo.PermissionViewChannel
		allow |= discordgo.PermissionSendMessages
		allow |= discordgo.PermissionVoiceConnect
		allow |= discordgo.PermissionManageMessages
		props.PermissionOverwrites = []*discordgo.PermissionOverwrite{
			{
				ID:    userID,
				Type:  discordgo.PermissionOverwriteTypeMember,
				Deny:  0,
				Allow: allow,
			},
			{
				ID:   guildID,
				Type: discordgo.PermissionOverwriteTypeRole,
				Deny: discordgo.PermissionAll,
			},
		}
	}

	return s.GuildChannelCreateComplex(guildID, props)
}

// we filled up on junk quickly, we should recycle a voice channel from junkjard
func (h *HiveCommand) recycleVoiceChannel(s *discordgo.Session, conf *db.HiveConfiguration, name, catID, userID, guildID string, limit int, hidden bool) (*discordgo.Channel, error, bool) {
	channels, err := s.GuildChannels(guildID)
	if err != nil {
		return nil, err, true
	}

	var toRecycle *discordgo.Channel

	for _, channel := range channels {
		if channel.ParentID == conf.JunkyardCategoryID && channel.Type == discordgo.ChannelTypeGuildVoice {
			toRecycle = channel
			break
		}
	}

	// no junk found we can recycle, buy a new one :(
	if toRecycle == nil {
		return nil, nil, false
	}

	cat, err := s.Channel(catID)
	if err != nil {
		return nil, err, true
	}

	edit := &discordgo.ChannelEdit{
		ParentID:             catID,
		PermissionOverwrites: cat.PermissionOverwrites,
		UserLimit:            limit,
		Name:                 conf.Prefix + name,
		Bitrate:              conf.VoiceBitrate,
	}

	if hidden {
		// admin privileges on channel for creator
		var allow int64
		allow |= discordgo.PermissionReadMessageHistory
		allow |= discordgo.PermissionViewChannel
		allow |= discordgo.PermissionSendMessages
		allow |= discordgo.PermissionVoiceConnect
		allow |= discordgo.PermissionManageMessages
		edit.PermissionOverwrites = []*discordgo.PermissionOverwrite{
			{
				ID:    userID,
				Type:  discordgo.PermissionOverwriteTypeMember,
				Deny:  0,
				Allow: allow,
			},
			{
				ID:   guildID,
				Type: discordgo.PermissionOverwriteTypeRole,
				Deny: discordgo.PermissionAll,
			},
		}
	}
	newChan, err := s.ChannelEditComplex(toRecycle.ID, edit)

	return newChan, err, true
}

func (h *HiveCommand) createVoiceChannel(s *discordgo.Session, conf *db.HiveConfiguration, name, catID, userID, guildID string, limit int, hidden bool) (*discordgo.Channel, error) {
	newChan, err, ok := h.recycleVoiceChannel(s, conf, name, catID, userID, guildID, limit, hidden)
	if ok {
		return newChan, err
	}
	props := discordgo.GuildChannelCreateData{
		Name:      conf.Prefix + name,
		Bitrate:   conf.VoiceBitrate,
		NSFW:      false,
		ParentID:  catID,
		Type:      discordgo.ChannelTypeGuildVoice,
		UserLimit: limit,
	}

	if hidden {
		// admin privileges on channel for creator
		var allow int64
		allow |= discordgo.PermissionReadMessageHistory
		allow |= discordgo.PermissionViewChannel
		allow |= discordgo.PermissionSendMessages
		allow |= discordgo.PermissionVoiceConnect
		allow |= discordgo.PermissionManageMessages
		props.PermissionOverwrites = []*discordgo.PermissionOverwrite{
			{
				ID:    userID,
				Type:  discordgo.PermissionOverwriteTypeMember,
				Deny:  0,
				Allow: allow,
			},
			{
				ID:   guildID,
				Type: discordgo.PermissionOverwriteTypeRole,
				Deny: discordgo.PermissionAll,
			},
		}
	}

	return s.GuildChannelCreateComplex(guildID, props)
}

// SayArchive handles the tm!archive command
func (h *HiveCommand) SayArchive(s *discordgo.Session, m *discordgo.MessageCreate) {
	channel, err := s.Channel(m.ChannelID)
	if err != nil {
		log.Println(err)
		return
	}

	conf, isHive, err := h.getConfigForRequestCategory(s, m.GuildID, m.ChannelID)
	if err != nil {
		log.Println(err)
		return
	}
	if !isHive || h.isPrivilegedChannel(channel.ID, conf) {
		s.ChannelMessageSend(m.ChannelID, "This command only works in hive created channels")
		return
	}

	if conf.Prefix != "" {
		if !strings.HasPrefix(channel.Name, conf.Prefix) {
			s.ChannelMessageSend(m.ChannelID, "This command only works in hive created channels with correct prefix")
			return
		}
	}

	j, err := s.Channel(conf.JunkyardCategoryID)
	if err != nil {
		log.Println(err)
		return
	}
	_, err = s.ChannelEditComplex(channel.ID, &discordgo.ChannelEdit{
		ParentID:             conf.JunkyardCategoryID,
		PermissionOverwrites: j.PermissionOverwrites,
	})
	if err != nil {
		log.Println(err)
	}
}

// SayLeave handles the tm!eave command
func (h *HiveCommand) SayLeave(s *discordgo.Session, m *discordgo.MessageCreate) {
	// check if this is allowed
	conf, isHive, err := h.getConfigForRequestCategory(s, m.GuildID, m.ChannelID)
	if err != nil {
		log.Println(err)
		return
	}
	if !isHive || h.isPrivilegedChannel(m.ChannelID, conf) {
		s.ChannelMessageSend(m.ChannelID, "This command only works in hive created channels, consider using Discord's mute instead\"")
		return
	}

	channel, err := s.Channel(m.ChannelID)
	if err != nil {
		log.Println(err)
		return
	}

	newPerms := []*discordgo.PermissionOverwrite{}
	for _, perm := range channel.PermissionOverwrites {
		if perm.ID != m.Author.ID {
			newPerms = append(newPerms, perm)
		}
	}
	_, err = s.ChannelEditComplex(channel.ID, &discordgo.ChannelEdit{
		PermissionOverwrites: newPerms,
	})
	if err != nil {
		log.Println(err)
	}
}

func (h *HiveCommand) handleReaction(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	message, err := s.ChannelMessage(r.ChannelID, r.MessageID)
	if err != nil {
		log.Println("Cannot get message of reaction", r.ChannelID)
		return
	}

	if message.Author.ID != s.State.User.ID {
		return // not the bot user
	}

	if len(message.Embeds) < 1 {
		return // not an embed
	}

	if len(message.Embeds[0].Fields) < 2 {
		return // not the correct embed
	}

	if message.Embeds[0].Title != "Hive Channel" {
		return // not the hive message
	}

	channel, err := s.Channel(message.Embeds[0].Fields[len(message.Embeds[0].Fields)-1].Value)
	if err != nil {
		log.Println(err)
		return
	}

	conf, isHive, err := h.getConfigForRequestChannel(r.GuildID, r.ChannelID)
	if err != nil {
		log.Println(err)
		return
	}

	if !isHive {
		//s.ChannelMessageSend(r.ChannelID, "Sorry category not allowed, try privilege escalating otherwise!")
		return
	}

	if channel.ParentID != conf.VoiceCategoryID && channel.ParentID != conf.TextCategoryID {
		// channel no longer in hive
		return
	}

	var allow int64
	allow |= discordgo.PermissionReadMessageHistory
	allow |= discordgo.PermissionViewChannel
	allow |= discordgo.PermissionSendMessages
	allow |= discordgo.PermissionVoiceConnect
	allow |= discordgo.PermissionAddReactions
	allow |= discordgo.PermissionAttachFiles
	allow |= discordgo.PermissionEmbedLinks

	err = s.ChannelPermissionSet(channel.ID, r.UserID, discordgo.PermissionOverwriteTypeMember, allow, 0)
	if err != nil {
		log.Println("Cannot set permissions", err)
		return
	}

	s.ChannelMessageSend(channel.ID, fmt.Sprintf("Welcome <@%s>, you can leave any time by saying `tm!leave`", r.UserID))
}

// Info return the commands in this package
func (h *HiveCommand) Info() []command.Command {

	return []command.Command{
		command.Command{
			Name:        "hive",
			Description: "Set up temporary meeting rooms",
			Hidden:      false,
		},
	}
}
