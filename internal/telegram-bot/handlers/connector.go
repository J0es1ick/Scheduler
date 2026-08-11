package handlers

import (
	"strings"

	"github.com/J0es1ick/Scheduler/internal/telegram-bot/keyboards"
	tele "gopkg.in/telebot.v3"
)

func (h *Handler) HandleConnectorInfo(c tele.Context) error {
	return c.Send(h.connectorInfoText())
}

func (h *Handler) HandleShowConnector(c tele.Context) error {
	_ = c.Respond()
	return editOrSend(c, h.connectorInfoText(), keyboards.BackToMoreMenu())
}

func (h *Handler) connectorInfoText() string {
	projectURL := strings.TrimRight(h.ProjectURL, "/")
	managedDocsURL := projectURL + "/blob/master/docs/managed-parsers.md"
	connectorDocsURL := projectURL + "/blob/master/docs/connector-api.md"

	return "Подключить своё расписание\n\n" +
		"Основной способ не требует от автора сервера или Docker: напишите парсер по компактному контракту и отправьте код проекту. Scheduler сам будет запускать его, повторять временные ошибки и проверять снимки.\n\n" +
		"Как подключиться:\n" +
		"1. Отправьте заявку через /hotline со ссылкой на источник.\n" +
		"2. Реализуйте FetchGroups и FetchSchedule по Parser SDK v1.\n" +
		"3. После review парсер появится в мастере админки и будет выполняться общей инфраструктурой.\n" +
		"4. Первый снимок администратор проверит визуально перед активацией.\n\n" +
		"Если источник уже отдаёт готовый Schedule Snapshot v1, достаточно публичного HTTPS URL. Для организаций со своей инфраструктурой остаётся подписанный внешний Connector API.\n\n" +
		"Управляемые парсеры: " + managedDocsURL + "\n" +
		"Внешний API: " + connectorDocsURL + "\n\n" +
		"Во всех режимах подозрительные изменения автоматически остаются в карантине."
}
