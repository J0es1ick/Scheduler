package admin

import (
	"testing"

	"github.com/J0es1ick/Scheduler/internal/domain"
)

func TestArchivedConnectorCanReturnToDraft(t *testing.T) {
	if !connectorTransitionAllowed(domain.ConnectorStatusArchived, domain.ConnectorStatusDraft) {
		t.Fatal("archived connector cannot return to draft")
	}
	if connectorTransitionAllowed(domain.ConnectorStatusArchived, domain.ConnectorStatusActive) {
		t.Fatal("archived connector bypasses testing and review")
	}
}
