package handlers

import (
	"context"
	"time"

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
	UniversityService     *service.UniversityService
	UserService           *service.UserService
	GroupService          *service.GroupService
	SubscriptionService   *service.SubscriptionService
	SupportRequestService *service.SupportRequestService
	AdminPublicURL        string
}

func NewHandler(
	scheduleService *service.ScheduleService,
	userService *service.UserService,
	groupService *service.GroupService,
	universityService *service.UniversityService,
	stateManager *state.Manager,
	subscriptionService *service.SubscriptionService,
	supportRequestService *service.SupportRequestService,
	adminPublicURL string,
) *Handler {
	return &Handler{
		ScheduleService:       scheduleService,
		StateManager:          stateManager,
		UniversityService:     universityService,
		UserService:           userService,
		GroupService:          groupService,
		SubscriptionService:   subscriptionService,
		SupportRequestService: supportRequestService,
		AdminPublicURL:        adminPublicURL,
	}
}
