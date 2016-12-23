package main

import (
	"log"

	"net/http"

	"github.com/go-telegram-bot-api/telegram-bot-api"
	"strings"
	"math/rand"
	"time"
	"strconv"
)

var mainBot *tgbotapi.BotAPI
var runningWebhook bool

type WebhookConfig struct {
	IP       string
	Port     string
	KeyPath  string
	CertPath string
}

type WhoUser struct {
	*tgbotapi.User
	Yes bool
}

type WhoMessage struct {
	Users []WhoUser
	Question string
}

var WhoMap = map[string]WhoMessage{}
var ArticleIDMap = map[string]WhoMessage{}

var inlineKeyboardWho = tgbotapi.InlineKeyboardMarkup{
	InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
		[]tgbotapi.InlineKeyboardButton{
			{
				Text: "Yes",
				CallbackData: strToPointer("1"),
			},
			{
				Text: "No",
				CallbackData: strToPointer("0"),
			},
		},
	},
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// Copied from Stack Overflow
func RandStringBytesMaskImprSrc(n int) string {
	b := make([]byte, n)
	// A src.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, rand.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = rand.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

func startBot(token string, webhookConf *WebhookConfig) {
	log.Println("Starting Bot")

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalln(err)
	}

	mainBot = bot

	bot.Debug = false

	log.Printf("Authorized on account %s", bot.Self.UserName)

	var updates <-chan tgbotapi.Update
	var webhookErr error

	if webhookConf != nil {
		_, webhookErr = bot.SetWebhook(tgbotapi.NewWebhookWithCert(webhookConf.IP+":"+webhookConf.Port+"/"+bot.Token, webhookConf.CertPath))
		if webhookErr != nil {
			log.Println("Webhook Error:", webhookErr, "Switching to poll")
		} else {
			runningWebhook = true
			updates = bot.ListenForWebhook("/" + bot.Token)
			go http.ListenAndServeTLS(webhookConf.IP+":"+webhookConf.Port, webhookConf.CertPath, webhookConf.KeyPath, nil)
			log.Println("Running on Webhook")
		}
	}

	if webhookErr != nil || webhookConf == nil {
		u := tgbotapi.NewUpdate(0)
		u.Timeout = 60

		updates, err = bot.GetUpdatesChan(u)
		if err != nil {
			log.Fatalln("Error found on getting poll updates:", err, "HALTING")
		}
		log.Println("Running on Poll")
	}

	for update := range updates {
		if update.Message == nil && update.CallbackQuery == nil && update.InlineQuery == nil && update.ChosenInlineResult == nil {
			continue
		}

		if update.CallbackQuery != nil {
			whoCBQuery(bot, update.CallbackQuery)
		} else if update.Message != nil {
			log.Printf("[%s] %s", update.Message.From.String(), update.Message.Text)

			if update.Message.Text != "" && update.Message.IsCommand() {
				if update.Message.Command() == "who" {
					runWhoCommand(bot, update.Message)
				}
			}
		} else if update.InlineQuery != nil {
			question := update.InlineQuery.Query
			if question == "" {
				question = "Who's Down"
			}

			whoMsg := WhoMessage{
				Question: question,
			}
			articleID := RandStringBytesMaskImprSrc(32)
			ArticleIDMap[articleID] = whoMsg

			res := tgbotapi.NewInlineQueryResultArticleHTML(articleID, question, generateWhoList(whoMsg))
			res.ReplyMarkup = &inlineKeyboardWho
			bot.AnswerInlineQuery(tgbotapi.InlineConfig{
				InlineQueryID: update.InlineQuery.ID,
				Results: []interface{}{
					res,
				},
			})
		}
		if update.ChosenInlineResult != nil {
			whoMsg, ok := ArticleIDMap[update.ChosenInlineResult.ResultID]
			if !ok {
				continue
			}
			delete(ArticleIDMap, update.ChosenInlineResult.ResultID)

			WhoMap[update.ChosenInlineResult.InlineMessageID] = whoMsg
		}
	}
}

func formatUser(user *tgbotapi.User) string {
	var name string
	if user.FirstName != "" {
		name = user.FirstName
		if user.LastName != "" {
			name += " " + user.LastName
		}
	} else if user.LastName != "" {
		name = user.LastName
	} else if user.UserName != "" {
		name = user.UserName
	} else {
		name = "Unknown"
	}

	if user.UserName != "" {

		return `<a href="http://telegram.me/` + user.UserName + `">` + name + `</a>`
	} else {
		return name
	}
}

func generateWhoList(whoMsg WhoMessage) string {
	yesNames := []string{}
	noNames := []string{}
	for _, user := range whoMsg.Users {
		if user.Yes {
			yesNames = append(yesNames, "\n• " + formatUser(user.User))
		} else {
			noNames = append(noNames, "\n• " + formatUser(user.User))
		}
	}

	return whoMsg.Question + "\n\n<b>Yes (" + strconv.Itoa(len(yesNames)) + "):</b>" + strings.Join(yesNames, "") + "\n<b>No (" + strconv.Itoa(len(noNames)) + "):</b>" + strings.Join(noNames, "")
}

func whoCBQuery(bot *tgbotapi.BotAPI, query *tgbotapi.CallbackQuery) {
	defer func(){
		_, err := bot.AnswerCallbackQuery(tgbotapi.NewCallback(query.ID, ""))
		if err != nil {
			log.Println("Error:", err)
		}
	}()

	var whoMsg WhoMessage
	var ok bool
	var msgID string
	if query.Message != nil && query.Message.MessageID != 0 {
		msgID = strconv.Itoa(query.Message.MessageID)
		whoMsg, ok = WhoMap[msgID]
	} else if query.InlineMessageID != "" {
		msgID = query.InlineMessageID
		whoMsg, ok = WhoMap[msgID]
	} else {
		return
	}

	if !ok {
		return
	}

	theIdx := -1
	for i, user := range whoMsg.Users {
		if user.ID == query.From.ID {
			theIdx = i
			break
		}
	}

	if query.Data == "1" {
		// If YES
		if theIdx != -1 {
			if whoMsg.Users[theIdx].Yes {
				whoMsg.Users[theIdx] = whoMsg.Users[len(whoMsg.Users)-1]
				whoMsg.Users = whoMsg.Users[:len(whoMsg.Users)-1]
			} else {
				whoMsg.Users[theIdx].Yes = true
			}
		} else {
			whoMsg.Users = append(whoMsg.Users, WhoUser{
				User: query.From,
				Yes: true,
			})
		}
	} else if query.Data == "0" {
		// If NO
		if theIdx != -1 {
			if !whoMsg.Users[theIdx].Yes {
				whoMsg.Users[theIdx] = whoMsg.Users[len(whoMsg.Users)-1]
				whoMsg.Users = whoMsg.Users[:len(whoMsg.Users)-1]
			} else {
				whoMsg.Users[theIdx].Yes = false
			}
		} else {
			whoMsg.Users = append(whoMsg.Users, WhoUser{
				User: query.From,
				Yes: false,
			})
		}
	}

	var msg tgbotapi.EditMessageTextConfig
	if query.InlineMessageID != "" {
		msg.Text = generateWhoList(whoMsg)
		msg.InlineMessageID = query.InlineMessageID
	} else {
		msg = tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, generateWhoList(whoMsg))
	}

	msg.ReplyMarkup = &inlineKeyboardWho
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	_, err := bot.Send(msg)
	if err != nil {
		log.Println("Error:", err)
	}

	WhoMap[msgID] = whoMsg
}

func strToPointer(str string) *string {
	return &str
}

func runWhoCommand(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	question := message.CommandArguments()
	if question == "" {
		question = "Who's Down"
	}

	whoMsg := WhoMessage{
		Question: question,
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, generateWhoList(whoMsg))
	msg.ReplyMarkup = inlineKeyboardWho
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	sentMsg, err := bot.Send(msg)
	if err != nil {
		log.Println("Error:", err)
	}

	WhoMap[strconv.Itoa(sentMsg.MessageID)] = whoMsg
}