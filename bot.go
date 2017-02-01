package main

import (
	"log"

	"net/http"

	"github.com/go-telegram-bot-api/telegram-bot-api"
	"strings"
	"math/rand"
	"time"
	"strconv"
	"fmt"
	"errors"
	"math"
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
	Choice int
}

type WhoMessage struct {
	Users []WhoUser
	Question string
	Choices []string
}

var WhoMap = map[string]WhoMessage{}
var ArticleIDMap = map[string]WhoMessage{}

func generateInlineKeyboard(whoMsg WhoMessage) *tgbotapi.InlineKeyboardMarkup {
	ikb := [][]tgbotapi.InlineKeyboardButton{}
	idx := 0
	for i := 0; i < int(math.Ceil(float64(len(whoMsg.Choices))/2)); i++ {
		ibkA := []tgbotapi.InlineKeyboardButton{}
		nxtIdx := idx+2
		if len(whoMsg.Choices) < nxtIdx {
			nxtIdx = len(whoMsg.Choices)
		}
		choices := whoMsg.Choices[idx:nxtIdx]
		for _, choice := range choices {
			ibkA = append(ibkA, tgbotapi.InlineKeyboardButton{
				Text: choice,
				CallbackData: strToPointer(strconv.Itoa(idx)),
			})
			idx++
		}
		ikb = append(ikb, ibkA)
	}
	return &tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: ikb,
	}

}

func NewWhoMessage(question string, choices ...string) WhoMessage {
	if len(choices) == 0 {
		choices = []string{"Yes", "No"}
	}
	return WhoMessage{
		Question: question,
		Choices: choices,
	}
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
			runInlineQueryWhoCommand(bot, update.InlineQuery)
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

type Choice struct{
	Choice string
	Users []string
}

func generateWhoList(whoMsg WhoMessage) string {
	choices := map[int]Choice{}
	for idx, choiceStr := range whoMsg.Choices {
		choices[idx] = Choice{
			Choice: choiceStr,
			Users: []string{},
		}
	}

	for _, user := range whoMsg.Users {
		choice, ok := choices[user.Choice]

		newChoice := Choice{}
		newChoice.Users = append(choice.Users, "\n• " + formatUser(user.User))

		if ok {
			newChoice.Choice = choice.Choice
		} else {
			if len(whoMsg.Choices) > user.Choice {
				newChoice.Choice = whoMsg.Choices[user.Choice]
			} else {
				newChoice.Choice = "Unknown Choice"
			}
		}
		choices[user.Choice] = newChoice
	}

	retStr := whoMsg.Question + "\n"
	for i := range whoMsg.Choices {
		choice := choices[i]
		retStr += fmt.Sprintf("\n<b>%s (%d):</b>%s", choice.Choice, len(choice.Users), strings.Join(choice.Users, ""))
	}

	return retStr
}

func whoCBQuery(bot *tgbotapi.BotAPI, query *tgbotapi.CallbackQuery) {
	var err error

	defer func(){
		var cbConfig tgbotapi.CallbackConfig
		if err != nil {
			cbConfig = tgbotapi.NewCallbackWithAlert(query.ID, "Error! " + err.Error())
		} else {
			cbConfig = tgbotapi.NewCallback(query.ID, "")
		}
		_, botErr := bot.AnswerCallbackQuery(cbConfig)
		if botErr != nil {
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
		err = errors.New("Cannot parse message")
		return
	}

	if !ok {
		err = errors.New("Cannot find question message")
		return
	}

	theIdx := -1
	for i, user := range whoMsg.Users {
		if user.ID == query.From.ID {
			theIdx = i
			break
		}
	}

	choiceIdx, err := strconv.Atoi(query.Data)
	if err != nil {
		return
	}

	if len(whoMsg.Choices) <= choiceIdx {
		err = errors.New("Cannot find choice in question")
		return
	}

	if theIdx != -1 {
		userChoice := whoMsg.Users[theIdx].Choice
		if userChoice == choiceIdx {
			whoMsg.Users[theIdx] = whoMsg.Users[len(whoMsg.Users)-1]
			whoMsg.Users = whoMsg.Users[:len(whoMsg.Users)-1]
		} else {
			whoMsg.Users[theIdx].Choice = choiceIdx
		}
	} else {
		whoMsg.Users = append(whoMsg.Users, WhoUser{
			User: query.From,
			Choice: choiceIdx,
		})
	}

	var msg tgbotapi.EditMessageTextConfig
	if query.InlineMessageID != "" {
		msg.Text = generateWhoList(whoMsg)
		msg.InlineMessageID = query.InlineMessageID
	} else {
		msg = tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, generateWhoList(whoMsg))
	}

	msg.ReplyMarkup = generateInlineKeyboard(whoMsg)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	_, botErr := bot.Send(msg)
	if botErr != nil {
		log.Println("Error:", err)
	}

	WhoMap[msgID] = whoMsg
}

func strToPointer(str string) *string {
	return &str
}

func runWhoCommand(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	question, options, err := commandQuerySplit(message.CommandArguments())
	if err != nil {
		return
	}

	whoMsg := NewWhoMessage(question, options...)

	msg := tgbotapi.NewMessage(message.Chat.ID, generateWhoList(whoMsg))
	msg.ReplyMarkup = generateInlineKeyboard(whoMsg)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	sentMsg, err := bot.Send(msg)
	if err != nil {
		log.Println("Error:", err)
	}

	WhoMap[strconv.Itoa(sentMsg.MessageID)] = whoMsg
}

func commandQuerySplit(query string) (question string, options []string, err error) {
	if query != "" {
		if strings.Contains(query, "##") {
			splitQ := strings.Split(query, "##")
			if len(splitQ) != 2 {
				question = query
			} else {
				question = splitQ[0]
				options = strings.Split(splitQ[1], "#")
			}

		} else {
			question = query
		}
	} else {
		question = "Who's Down"
	}

	if len(options) > 10 {
		err = errors.New("Too many options")
		return
	}

	for i, val := range options {
		options[i] = strings.TrimSpace(val)
	}
	question = strings.TrimSpace(question)
	return
}

func runInlineQueryWhoCommand(bot *tgbotapi.BotAPI, iq *tgbotapi.InlineQuery) {
	question, options, err := commandQuerySplit(iq.Query)
	if err != nil {
		return
	}

	whoMsg := NewWhoMessage(question, options...)
	articleID := RandStringBytesMaskImprSrc(32)
	ArticleIDMap[articleID] = whoMsg

	res := tgbotapi.NewInlineQueryResultArticleHTML(articleID, question, generateWhoList(whoMsg))
	res.ReplyMarkup = generateInlineKeyboard(whoMsg)
	bot.AnswerInlineQuery(tgbotapi.InlineConfig{
		InlineQueryID: iq.ID,
		Results: []interface{}{
			res,
		},
	})
}