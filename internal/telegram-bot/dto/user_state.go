package dto

// UserState хранит состояние диалога конкретного пользователя в Telegram.
// Живёт только в памяти бота, в БД не хранится.
type UserState struct {
	UniversityID string
	University   string // человекочитаемое имя для отображения
	SearchType   SearchType
	Query        string // основной запрос (группа/ФИО) для регулярного расписания
	GroupID      string // ID группы в БД (заполняется после FindOrCreateGroup)
	SearchQuery  string // временный запрос для команды /search
	Step         string // "awaiting_role" | "awaiting_query" | "awaiting_search_query" | "done"
}

type SearchType string

const (
	SearchTypeGroup      SearchType = "group"
	SearchTypeTeacher    SearchType = "teacher"
	SearchTypeRoom       SearchType = "room"
	SearchTypeDiscipline SearchType = "discipline"
)
