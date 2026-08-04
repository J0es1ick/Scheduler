package repository

import (
	"fmt"
	"strings"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func CanonicalizeSnapshotGroupIDs(
	payload domain.ScheduleSnapshot,
	existing []domain.Group,
) (domain.ScheduleSnapshot, int, error) {
	byName := make(map[string]domain.Group, len(existing))
	byID := make(map[string]domain.Group, len(existing))
	for _, group := range existing {
		byName[strings.TrimSpace(group.Name)] = group
		byID[group.ID] = group
	}

	remapped := 0
	for index := range payload.Groups {
		group := &payload.Groups[index]
		group.Name = strings.TrimSpace(group.Name)
		canonicalID := group.ID
		if current, ok := byName[group.Name]; ok {
			canonicalID = current.ID
		} else if current, ok := byID[group.ID]; ok && current.Name != group.Name {
			return payload, remapped, fmt.Errorf(
				"source group id %s changed name from %q to %q",
				group.ID, current.Name, group.Name,
			)
		}
		if canonicalID != group.ID {
			remapped++
			group.ID = canonicalID
		}
		for lessonIndex := range group.Lessons {
			group.Lessons[lessonIndex].GroupID = canonicalID
		}
	}
	return payload, remapped, nil
}
