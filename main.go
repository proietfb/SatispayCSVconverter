package main

import (
	"encoding/csv"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/spf13/viper"
)

type Conf struct {
	BotAPIKey  string
	RestrictTo *[]Restrictions
	MountDisks []string
}
type Restrictions struct {
	Username string
	ChatID   int64
}

func parseConf() *Conf {
	var conf Conf
	viper.SetConfigName("conf")
	viper.SetConfigType("json")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/srv/satispayCSV2budgetbanker/")
	err := viper.ReadInConfig()
	if err != nil {
		panic(err)
	}
	if err = viper.Unmarshal(&conf); err != nil {
		panic(err)
	}
	return &conf
}

func (conf *Conf) auth(username string, chatID int64) bool {
	if conf != nil && conf.RestrictTo != nil {
		for _, r := range *conf.RestrictTo {
			if username == r.Username && chatID == r.ChatID {
				return true
			}
		}
	}
	return false
}

var months = []string{"Gen", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

func formatDate(date string) string {
	var formatted string

	for m := 1; m < len(months); m++ {
		found := strings.Contains(date, months[m])
		if found {
			formatted = strings.Replace(date, months[m], "/"+strconv.Itoa(m)+"/", -1)
			idx := strings.Index(formatted, "at")
			formatted = formatted[:idx]
			formatted = strings.ReplaceAll(formatted, " ", "")
			break
		}
	}

	return formatted
}

func convertCSV(content, filename string) (bool, error) {
	converted := false

	r := strings.NewReader(content)

	reader := csv.NewReader(r)

	record, err := reader.ReadAll()

	if err != nil {
		log.Fatal("Error reading record", err)
		return converted, err
	} else {
		for _, element := range record[1:] {
			//fix date
			element[4] = formatDate(element[4])
			//fix amount
			element[5] = strings.ReplaceAll(element[5], ",", ".")
			element[5] = strings.TrimSpace(element[5])
			//remove all commas from description
			element[7] = strings.ReplaceAll(element[7], ",", ".")

			log.Print(element)
		}
		f, err := os.Create(filename)
		defer f.Close()

		if err != nil {
			log.Fatal("Error on creating out file: ", err)
			return converted, err
		}

		writer := csv.NewWriter(f)
		writer.WriteAll(record)
		converted = true
	}

	return converted, nil
}

func main() {
	conf := parseConf()
	bot, err := tgbotapi.NewBotAPI(conf.BotAPIKey)

	if err != nil {
		panic(err)
	}

	bot.Debug = true
	log.Println("Authorized on account ", bot.Self.UserName)
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if conf.auth(update.SentFrom().UserName, update.SentFrom().ID) {
			if update.Message.Document.MimeType == "text/csv" {
				fc := tgbotapi.FileConfig{FileID: update.Message.Document.FileID}
				if file, err := bot.GetFile(fc); err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Cannot parse uploaded file. Error: "+err.Error())
					bot.Send(msg)
				} else {
					if resp, err := http.Get(file.Link(bot.Token)); err == nil && resp.StatusCode == 200 {
						if body, err := io.ReadAll(resp.Body); err == nil {
							str := string(body)
							name := update.Message.Document.FileName + "_converted.csv"
							filename := "/tmp/" + name
							ret, _ := convertCSV(str, filename)

							document := tgbotapi.NewDocument(update.Message.Chat.ID, tgbotapi.FilePath(filename))
							document.Caption = "convertion: " + strconv.FormatBool(ret)
							bot.Send(document)
							bot.Send(tgbotapi.NewDeleteMessage(update.Message.Chat.ID, update.Message.MessageID))
						} else {
							msg := tgbotapi.NewMessage(update.Message.Chat.ID, "cannot read file. Please upload a valid csv")
							bot.Send(msg)
						}
						defer resp.Body.Close()
					}

				}
			} else {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Cannot parse a "+update.Message.Document.MimeType+" please load a csv file")
				bot.Send(msg)
			}
		} else {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "User "+update.SentFrom().UserName+" cannot use this bot")
			bot.Send(msg)
			log.Printf("Unauthenticated")
		}

	}

}
