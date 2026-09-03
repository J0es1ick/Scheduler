package parserruntime

import (
	"github.com/J0es1ick/Scheduler/integrations/ivgpu"
	"github.com/J0es1ick/Scheduler/internal/declarative"
	"github.com/J0es1ick/Scheduler/internal/managedparser"
	"github.com/J0es1ick/Scheduler/internal/repository"
	"github.com/J0es1ick/Scheduler/internal/scraper/ispu"
	"github.com/J0es1ick/Scheduler/internal/scraper/isuct"
	"github.com/J0es1ick/Scheduler/internal/service"
	"github.com/jmoiron/sqlx"
)

func New(db *sqlx.DB) *service.ParserService {
	groupRepo := repository.NewGroupRepository(db)
	scheduleService := service.NewScheduleService(
		repository.NewLessonRepository(db),
		repository.NewSemesterRepository(db),
		groupRepo,
	)
	parserService := service.NewParserService(
		repository.NewDataSourceRepository(db),
		repository.NewParseLogRepository(db),
		groupRepo,
		scheduleService,
		repository.NewParserSnapshotRepository(db),
		repository.NewNotificationRepository(db),
		repository.NewParserDiagnosticRepository(db),
	)
	parserService.RegisterAdapter(isuct.UniversityID, isuct.New(""))
	parserService.RegisterAdapter(ispu.UniversityID, ispu.New(""))
	parserService.RegisterAdapterFactory("managed:"+ivgpu.ParserID, managedparser.Factory(ivgpu.New))
	parserService.RegisterAdapterFactory(declarative.AdapterType, declarative.AdapterFactory)
	return parserService
}
