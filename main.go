package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/elliotchance/pie/v2"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/encoding/json"
	"golang.org/x/exp/slices"
)

var (
	rd *redis.Client
)

func main() {
	rd = redis.NewClient(&redis.Options{
		Addr: os.Getenv("REDIS_ADDR"),
	})

	defer func(rd *redis.Client) {
		_ = rd.Close()
	}(rd)

	// Create a new Discord session using the provided bot token.
	dg, err := discordgo.New("Bot " + os.Getenv("DISCORD_TOKEN"))
	if err != nil {
		log(map[string]string{"msg": "error creating Discord session", "err": err.Error(), "level": "error"})
		return
	}

	// Register the messageCreate func as a callback for MessageCreate events.
	dg.AddHandler(messageCreate)

	// Register funcs as a callback for fixing Twitter/X URL.
	dg.AddHandler(messageReactionAddHandlerForTwitterUrl)
	dg.AddHandler(messageCreateTwitterUrlChecker)

	// In this example, we only care about receiving message events.
	dg.Identify.Intents = discordgo.IntentsAllWithoutPrivileged

	// Open a websocket connection to Discord and begin listening.
	err = dg.Open()
	defer func(dg *discordgo.Session) {
		_ = dg.Close()
	}(dg)

	if err != nil {
		log(map[string]string{"msg": "error opening connection,", "err": err.Error(), "level": "error"})
		return
	}

	// Wait here until CTRL-C or other term signal is received.
	log(map[string]string{"msg": "Bot is now running.  Press CTRL-C to exit."})
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

func messageReactionAddHandlerForTwitterUrl(s *discordgo.Session, r *discordgo.MessageReactionAdd) {
	if r.GuildID == "" {
		return
	}
	msg, err := s.ChannelMessage(r.ChannelID, r.MessageID)
	if err != nil {
		fmt.Println("Error fetching message:", err)
		return
	}
	if r.UserID != msg.Author.ID {
		return
	}

	err = s.MessageReactionsRemoveAll(r.ChannelID, r.MessageID)
	if err != nil {
		fmt.Println("Error removing reactions:", err)
		return
	}

	modifiedContent := strings.ReplaceAll(msg.Content, "//x.com/", "//vxtwitter.com/")
	modifiedContent = strings.ReplaceAll(modifiedContent, "//twitter.com/", "//vxtwitter.com/")

	_, err = s.ChannelMessageSendReply(r.ChannelID, modifiedContent, msg.Reference())
	if err != nil {
		fmt.Println("Error sending message:", err)
		return
	}
}

func messageCreateTwitterUrlChecker(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.GuildID == "" {
		return
	}
	if !strings.Contains(m.Content, "//x.com/") && !strings.Contains(m.Content, "//twitter.com/") {
		return
	}

	err := s.MessageReactionAdd(m.ChannelID, m.ID, "❌")
	if err != nil {
		fmt.Println("Error adding reaction:", err)
		return
	}
}


// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the authenticated bot has access to.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	// This isn't required in this specific example, but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	msg := m.Content

	prefix := strings.TrimSpace(os.Getenv("CMD_PREFIX"))
	r := regexp.MustCompile(fmt.Sprintf(`(?s:^%s(\S+)(.*))`, prefix))
	match := r.FindAllStringSubmatch(msg, -1)

	if match == nil || len(match[0]) != 3 {
		return
	}

	ctx := context.Background()

	cmd := strings.TrimSpace(match[0][1])
	args := strings.TrimSpace(match[0][2])

	log(map[string]string{"type": "req", "cmd": cmd, "args": args})

	servID := m.GuildID
	chanID := m.Message.ChannelID

	servKey := fmt.Sprintf("%s-keywords", servID)
	chanKey := fmt.Sprintf("%s-%s-keywords", servID, chanID)
	reserved := []string{"등록", "삭제", "목록", "격리", "이동"}

	chanIsolationKey := fmt.Sprintf("%s-%s-isolated", servID, chanID)
	_, err := rd.Get(ctx, chanIsolationKey).Result()
	if err != nil && err != redis.Nil {
		panic("redis error")
	}
	isolated := err != redis.Nil

	var hkey string
	if isolated {
		hkey = chanKey
	} else {
		hkey = servKey
	}

	keywords := rd.HKeys(ctx, hkey).Val()
	switch cmd {
	case "목록":
		var msg string
		if keywords == nil || len(keywords) == 0 {
			msg = "등록된 키워드가 없습니다"
		} else {
			msg = strings.Join(keywords, ", ")
		}

		replyX(s, m, msg)

	case "등록":
		if len(args) == 0 {
			fmt.Println("empty args")
			return
		}

		var k, v string
		if len(m.Attachments) == 0 {
			r := regexp.MustCompile(`(?s:^(\S+)(.*))`)
			match := r.FindAllStringSubmatch(args, -1)
			if match == nil || len(match[0]) != 3 {
				return
			}

			k = strings.TrimSpace(match[0][1])
			v = strings.TrimSpace(match[0][2])
		} else {
			r := regexp.MustCompile(`^(\S+)`)
			match := r.FindAllStringSubmatch(args, -1)
			if match == nil || len(match[0]) != 2 {
				return
			}

			k = strings.TrimSpace(match[0][1])
			v = m.Attachments[0].URL
		}

		if len(v) == 0 {
			replyX(s, m, "내용을 입력해주세요")
			return
		}

		if strings.HasPrefix(k, "http") {
			replyX(s, m, "?")
			return
		}

		if pie.Contains(reserved, k) {
			replyX(s, m, "예약된 키워드입니다")
			return
		}

		rd.HSet(ctx, hkey, k, v)
		replyX(s, m, fmt.Sprintf("키워드 %s 등록을 완료했습니다", k))

	case "삭제":
		r := regexp.MustCompile(`^(\S+)`)
		match := r.FindAllStringSubmatch(args, -1)
		if match == nil || len(match[0]) != 2 {
			return
		}

		k := strings.TrimSpace(match[0][1])
		rd.HDel(ctx, hkey, k)
		replyX(s, m, fmt.Sprintf("키워드 %s 삭제를 완료했습니다", k))

	case "격리":
		if isolated {
			rd.Del(ctx, chanIsolationKey)
			replyX(s, m, "채널 격리 해제")
		} else {
			rd.Set(ctx, chanIsolationKey, 1, 0)
			replyX(s, m, "채널 격리 완료")
		}
	case "이동":
		r := regexp.MustCompile(`^(\S+)`)
		match := r.FindAllStringSubmatch(args, -1)
		if match == nil || len(match[0]) != 2 {
			return
		}

		var moved []string

		for _, key := range strings.Split(match[0][1], ",") {
			k := strings.TrimSpace(key)
			v := rd.HGet(ctx, servKey, k).Val()
			if len(v) == 0 {
				continue
			}

			rd.HDel(ctx, servKey, k)
			rd.HSet(ctx, chanKey, k, v)

			moved = append(moved, k)
		}

		if len(moved) > 0 {
			replyX(s, m, fmt.Sprintf("키워드 %s 이동 완료", strings.Join(moved, ", ")))
		}
	default:
		v := rd.HGet(ctx, hkey, cmd).Val()
		if len(v) > 0 {
			replyX(s, m, v)
			return
		}

		var targets []string
		for _, kw := range keywords {
			if strings.Contains(strings.ToLower(kw), strings.ToLower(cmd)) {
				targets = append(targets, kw)
			}
		}

		switch len(targets) {
		case 0:
		case 1:
			v := rd.HGet(ctx, hkey, targets[0]).Val()
			replyX(s, m, fmt.Sprintf("%s\n%s", targets[0], v))
		default:
			slices.Sort(targets)
			replyX(s, m, strings.Join(targets, ", "))
		}
	}

	//dump, err := discordgo.Marshal(m)
	//if err != nil {
	//	panic(err)
	//}
	//
	//fmt.Println(string(dump))
}

func replyX(s *discordgo.Session, m *discordgo.MessageCreate, msg string) {
	log(map[string]string{"type": "res", "msg": msg})
	_, err := s.ChannelMessageSendReply(m.ChannelID, msg, m.Reference())
	if err != nil {
		panic(err)
	}
}

func log(msg map[string]string) {
	m, _ := json.Marshal(msg)
	fmt.Println(string(m))
}
