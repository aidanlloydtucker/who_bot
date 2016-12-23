package main

import (
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/urfave/cli"
	"encoding/gob"
)

// Build Vars
var (
	Version   string
	BuildTime string
)

func main() {
	app := cli.NewApp()

	app.Name = "Who Bot"
	app.Usage = "Telegram bot"

	app.Authors = []cli.Author{
		{
			Name: "Aidan Lloyd-Tucker",
		},
	}
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "token, t",
			Usage: "The telegram bot api token",
		},
		cli.StringFlag{
			Name:  "ip",
			Usage: "The IP for the webhook port",
		},
		cli.StringFlag{
			Name:  "webhook_port",
			Usage: "The telegram bot api webhook port",
			Value: "8443",
		},
		cli.StringFlag{
			Name:  "webhook_cert",
			Usage: "The telegram bot api webhook cert",
			Value: "./ignored/cert.pem",
		},
		cli.StringFlag{
			Name:  "webhook_key",
			Usage: "The telegram bot api webhook key",
			Value: "./ignored/key.key",
		},
		cli.BoolFlag{
			Name:  "enable_webhook, w",
			Usage: "Enables webhook if true",
		},
		cli.BoolFlag{
			Name:  "prod",
			Usage: "Sets bot to production mode",
		},
		cli.StringFlag{
			Name:  "save",
			Usage: "Filepath for whomap save",
			Value: "",
		},
	}

	app.Version = Version

	num, err := strconv.ParseInt(BuildTime, 10, 64)
	if err == nil {
		app.Compiled = time.Unix(num, 0)
	}

	app.Action = runApp
	app.Run(os.Args)
}

func decodeSaveFile(filename string) (map[string]WhoMessage, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	newWhoMap := map[string]WhoMessage{}

	decoder := gob.NewDecoder(file)
	err = decoder.Decode(&newWhoMap)
	if err != nil {
		return nil, err
	}

	return newWhoMap, nil
}

func encodeSaveFile(filename string, saveWhoMap map[string]WhoMessage) error {
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	return encoder.Encode(saveWhoMap)
}

func runApp(c *cli.Context) error {
	log.Println("Running app")

	// Start bot

	var webhookConf *WebhookConfig = nil

	if c.IsSet("ip") && c.Bool("enable_webhook") {
		webhookConf = &WebhookConfig{
			IP:       c.String("ip"),
			CertPath: c.String("webhook_cert"),
			KeyPath:  c.String("webhook_key"),
			Port:     c.String("webhook_port"),
		}
	}

	log.Println("Decoding save file")

	if c.IsSet("save") {
		newWhoMap, err := decodeSaveFile(c.String("save"))
		if err != nil {
			log.Println("Error decoding save file:", err)
		} else {
			WhoMap = newWhoMap
		}
	}

	log.Println("Starting bot and website")

	go startBot(c.String("token"), webhookConf)

	// Safe Exit

	var Done = make(chan bool, 1)

	sigs := make(chan os.Signal, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs

		if runningWebhook {
			mainBot.RemoveWebhook()
		}

		if c.IsSet("save") {
			err := encodeSaveFile(c.String("save"), WhoMap)
			if err != nil {
				log.Println("Error encoding save file:", err)
			}
		}

		Done <- true
	}()
	<-Done

	log.Println("Safe Exit")
	return nil
}