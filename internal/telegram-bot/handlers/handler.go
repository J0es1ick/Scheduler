package handlers

import (
	"context"
	"sync"
	"time"

	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/J0es1ick/Scheduler/internal/telegram-bot/state"
)

const handlerTimeout = 15 * time.Second

func reqCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), handlerTimeout)
}

type Handler struct {
	ScheduleService       scheduleService
	StateManager          *state.Manager
	UniversityService     universityService
	UserService           userService
	GroupService          groupService
	SubscriptionService   subscriptionService
	SupportRequestService *service.SupportRequestService
	MetricsService        *service.MetricsService
	ChatProfileService    chatProfileService
	AdminPublicURL        string
	ProjectURL            string
	scheduleMessagesMu    sync.Mutex
	scheduleMessages      map[string]trackedScheduleMessages
}

type trackedScheduleMessages struct {
	IDs       []int
	CreatedAt time.Time
}

type universityService interface {
	GetAll(context.Context) ([]domain.University, error)
	GetByID(context.Context, string) (*domain.University, error)
	GetSourceFreshness(context.Context, string) (*domain.SourceFreshness, error)
}

type chatProfileService interface {
	Set(context.Context, string, string, string, string) error
	Get(context.Context, string) (*domain.ChatScheduleProfile, error)
	Delete(context.Context, string) error
	SetScheduleView(context.Context, string, domain.ScheduleViewFormat) error
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
	SetSearchScheduleView(context.Context, string, domain.ScheduleViewFormat) error
	ExportData(context.Context, string) (*domain.UserDataExport, error)
	DeleteOwnData(context.Context, string) error
}

type groupService interface {
	GetGroupByID(context.Context, string) (*domain.Group, error)
	GetGroupByName(context.Context, string, string) (*domain.Group, error)
	FindActiveByName(context.Context, string, string) ([]domain.Group, error)
	GetActiveGroupByToken(context.Context, string) (*domain.Group, error)
}

type scheduleService interface {
	GetScheduleForGroupRange(context.Context, string, time.Time, time.Time) (map[time.Time][]domain.Lesson, error)
	GetScheduleForTeacherRange(context.Context, string, string, time.Time, time.Time) (map[time.Time][]domain.Lesson, error)
	GetScheduleForRoomRange(context.Context, string, string, time.Time, time.Time) (map[time.Time][]domain.Lesson, error)
	FindTeachers(context.Context, string, string) ([]string, error)
}

type subscriptionService interface {
	GetGroupSubscriptions(context.Context, string) ([]domain.GroupSubscription, error)
	Subscribe(context.Context, string, string, string) error
	SubscribeAndSetDefault(context.Context, string, string) error
	SetDefaultGroup(context.Context, string, string) error
	UnsubscribeAndSelectDefault(context.Context, string, string) (string, error)
	SetGroupScheduleView(context.Context, string, string, domain.ScheduleViewFormat) error
	SetGroupSubgroup(context.Context, string, string, int) error
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
