package scrapper

import (
	"context"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

// SourceAdapter — единый интерфейс, который обязан реализовать каждый парсер университета.
// Все адаптеры должны быть независимы друг от друга и от конкретного транспорта (HTTP, файл и т.д.).
//
// Правила для реализаторов:
//   - FetchGroups возвращает актуальный список групп университета. Может вызываться
//     периодически для синхронизации справочника.
//   - FetchSchedule возвращает срез занятий для конкретной группы.
//     groupID — внешний идентификатор группы, который адаптер сам присвоил при FetchGroups.
//     Адаптер не обязан фильтровать по dateFrom/dateTo — он может вернуть всё расписание
//     семестра, а ParserService сам выберет нужный диапазон. Если адаптер поддерживает
//     фильтрацию на стороне источника — он должен её использовать для экономии трафика.
//   - Оба метода должны уважать ctx.Done() и завершаться при его отмене.
type SourceAdapter interface {
	// Name возвращает человекочитаемое название адаптера (напр. "ИГХТУ").
	// Используется в логах и ошибках.
	Name() string

	// UniversityID возвращает slug университета, совпадающий с domain.University.ID.
	// Нужен ParserService-у, чтобы знать, к какому университету привязывать группы и занятия.
	UniversityID() string

	// SetSemesterID задаёт ID текущего семестра перед вызовом FetchSchedule.
	// ParserService вызывает его каждый раз перед запуском парсинга, чтобы занятия
	// всегда привязывались к актуальному семестру из БД.
	SetSemesterID(id string)

	// FetchGroups возвращает все активные учебные группы университета.
	// Поле domain.Group.ID должно содержать стабильный внешний ключ (например, числовой ID
	// на сайте вуза), а НЕ новый UUID — это позволяет ParserService-у делать upsert без дублей.
	FetchGroups(ctx context.Context) ([]domain.Group, error)

	// FetchSchedule возвращает список занятий для группы с идентификатором groupID.
	// groupID совпадает с domain.Group.ID, возвращённым FetchGroups.
	// Поле domain.Lesson.ID тоже должно быть стабильным (хэш от ключевых полей),
	// чтобы SaveLessonsBatch мог делать upsert.
	FetchSchedule(ctx context.Context, groupID string) ([]domain.Lesson, error)
}
