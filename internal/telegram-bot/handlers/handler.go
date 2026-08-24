package handlers

import (
	"context"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
)

// handlerTimeout — таймаут на обработку одного апдейта (включая все DB-запросы).
// telebot.v3.Context не оборачивает context.Context (в отличие от net/http.Request),
// поэтому контекст с таймаутом создаётся вручную на входе в каждый хэндлер через reqCtx.
const handlerTimeout = 15 * time.Second

// reqCtx возвращает context.Context с таймаутом для использования в DB/HTTP-вызовах
// внутри хэндлера. Заменяет context.Background() без таймаута — если апдейт завис
// (например, БД недоступна), обработка прервётся через handlerTimeout вместо
// блокировки навсегда.
//
// Важно: возвращаемый cancel должен быть вызван через defer в каждом хэндлере,
// который его получает, иначе будет утечка таймеров.
func reqCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), handlerTimeout)
}

type Handler struct {
	ScheduleService       *service.ScheduleService
	StateManager          *state.Manager
	UniversityService     universityService
	UserService           userService
	GroupService          groupService
	SubscriptionService   *service.SubscriptionService
	SupportRequestService *service.SupportRequestService
	MetricsService        *service.MetricsService
	ChatProfileService    *service.ChatProfileService
	AdminPublicURL        string
	ProjectURL            string
}

type universityService interface {
	GetAll(context.Context) ([]domain.University, error)
	GetByID(context.Context, string) (*domain.University, error)
	GetSourceFreshness(context.Context, string) (*domain.SourceFreshness, error)
}

type userService interface {
	RegisterOrGetUser(context.Context, string, string) (*domain.User, error)
	GetUser(context.Context, string) (*domain.User, error)
	IsAdmin(context.Context, string) (bool, error)
	MarkTelegramMenuConfigured(context.Context, string, string) error
	SetDefaultGroup(context.Context, string, string) error
	SetNotificationsEnabled(context.Context, string, bool) error
	SetLessonReminder(context.Context, string, bool, int) error
	SetQuietHours(context.Context, string, bool, string, string) error
	ExportData(context.Context, string) (*domain.UserDataExport, error)
	DeleteOwnData(context.Context, string) error
}

type groupService interface {
	GetGroupByID(context.Context, string) (*domain.Group, error)
	GetGroupByName(context.Context, string, string) (*domain.Group, error)
	FindActiveByName(context.Context, string, string) ([]domain.Group, error)
}

func NewHandler(
	scheduleService *service.ScheduleService,
	userService *service.UserService,
	groupService *service.GroupService,
	universityService *service.UniversityService,
	stateManager *state.Manager,
	subscriptionService *service.SubscriptionService,
	supportRequestService *service.SupportRequestService,
	metricsService *service.MetricsService,
	chatProfileService *service.ChatProfileService,
	adminPublicURL string,
	projectURL string,
) *Handler {
	return &Handler{
		ScheduleService:       scheduleService,
		StateManager:          stateManager,
		UniversityService:     universityService,
		UserService:           userService,
		GroupService:          groupService,
		SubscriptionService:   subscriptionService,
		SupportRequestService: supportRequestService,
		MetricsService:        metricsService,
		ChatProfileService:    chatProfileService,
		AdminPublicURL:        adminPublicURL,
		ProjectURL:            projectURL,
	}
}
