package main

import (
	"log"
	"time"

	"github.com/J0es1ick/Scheduler/internal/config"
	bot "github.com/J0es1ick/Scheduler/internal/telegram-bot"
	tele "gopkg.in/telebot.v3"
)

func main() {
	cfg := config.Load()
	pref := tele.Settings{
		Token:  cfg.BotToken,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}
	b, err := tele.NewBot(pref)
	if err != nil {
		log.Fatal(err)
	}
	bot.Register(b)
	log.Println("Бот запущен...")
	b.Start()
}
