//go:build integration

package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/helpers"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
	"strings"
	"testing"
)

func TestPersonalChangesReachBotDailyInlineAndEveryExport(t *testing.T) {
	j := newRuntimeJourney(t)
	j.onboard(t)
	editor := service.NewScheduleService(repository.NewLessonRepository(j.adminDB), repository.NewSemesterRepository(j.adminDB), repository.NewGroupRepository(j.adminDB))
	subject, room := "Лично изменённый предмет", "Личная аудитория"
	input := service.PersonalChangeInput{TargetID: "journey-a", LessonID: fmt.Sprintf("journey-%d-0", helpers.Weekday(j.date)), Date: j.date.Format("2006-01-02"), Scope: "day", Patch: domain.PersonalLessonPatch{Subject: &subject, Room: &room}}
	if _, err := editor.SavePersonalChange(j.ctx, "42", input); err != nil {
		t.Fatal(err)
	}
	input.LessonID = fmt.Sprintf("journey-%d-1", helpers.Weekday(j.date))
	input.Patch = domain.PersonalLessonPatch{}
	input.Cancelled = true
	if _, err := editor.SavePersonalChange(j.ctx, "42", input); err != nil {
		t.Fatal(err)
	}
	j.callback(t, j.h.HandleSetScheduleView, "journey-a|compact|0")
	j.message(t, j.h.HandleToday, "/today", "")
	if !strings.Contains(j.telegram.text, subject) || !strings.Contains(j.telegram.text, room) || strings.Contains(j.telegram.text, "Предмет подгруппы 1") {
		t.Fatalf("manual lost personal changes: %s", j.telegram.text)
	}
	messages, err := j.h.PrepareDailySchedule(j.ctx, "42", j.date)
	if err != nil {
		t.Fatal(err)
	}
	text := ""
	for _, message := range messages {
		text += message.Text
	}
	if !strings.Contains(text, subject) || strings.Contains(text, "Предмет подгруппы 1") {
		t.Fatalf("daily: %s", text)
	}
	for _, format := range []string{"json", "csv", "ics", "png"} {
		args := fmt.Sprintf("%s|%s|%s|1", format, keyboards.GroupToken("journey-a"), j.date.Format("2006-01-02"))
		j.callback(t, j.h.HandleDownloadSchedule, args)
		payload := j.telegram.files[scheduleFileName("4/147", j.date, 1, "."+format)]
		if len(payload) == 0 {
			t.Fatalf("missing %s", format)
		}
		if format != "png" && (!strings.Contains(payload, subject) || strings.Contains(payload, "Предмет подгруппы 1")) {
			t.Fatalf("export %s: %s", format, payload)
		}
	}
	c := j.telegram.bot.NewContext(tele.Update{Query: &tele.Query{ID: "personal-inline", Sender: &tele.User{ID: 42}, Text: j.date.Format("2006-01-02")}})
	if err = j.h.HandleInlineQuery(c); err != nil {
		t.Fatal(err)
	}
	var results []struct {
		Content struct {
			Text string `json:"message_text"`
		} `json:"input_message_content"`
	}
	if err = json.Unmarshal([]byte(j.telegram.payloads[len(j.telegram.payloads)-1]["results"]), &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !strings.Contains(results[0].Content.Text, subject) || strings.Contains(results[0].Content.Text, "Предмет подгруппы 1") {
		t.Fatalf("inline: %+v", results)
	}
	official, err := j.h.getScheduleForTarget(j.ctx, &scheduleTarget{GroupID: "journey-a"}, j.date, j.date)
	if err != nil || len(official) != 1 || len(official[0].Lessons) != 3 {
		t.Fatalf("group chat affected: %+v %v", official, err)
	}
}
