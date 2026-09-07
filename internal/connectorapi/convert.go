package connectorapi

import (
	connector "github.com/J0es1ick/Scheduler/connector/v1"
	"github.com/J0es1ick/Scheduler/internal/domain"
	"github.com/J0es1ick/Scheduler/internal/snapshotconvert"
)

func convertSnapshot(sourceID, universityID string, input connector.Snapshot) (domain.ScheduleSnapshot, error) {
	return snapshotconvert.Convert(sourceID, universityID, input)
}
