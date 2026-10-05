package dto

import "time"

import "github.com/J0es1ick/Scheduler/internal/domain"

type UserState struct {
	Role                               domain.UserRole
	TeacherID                          string
	TeacherName                        string
	DailyOnboarding                    bool
	UniversityID                       string
	University                         string
	SearchType                         SearchType
	Query                              string
	GroupID                            string
	SearchQuery                        string
	TeacherCandidates                  []string
	TeacherSearchOrigin                string
	HotlineType                        string
	HotlineContext                     string
	Step                               string
	FlowNonce                          string
	GroupChangeDestination             string
	GroupChangePage                    int
	SetSelectedGroupDefault            bool
	PendingDeleteToken                 string
	PendingDeleteExpiresAt             time.Time
	PendingSubscriptionDeleteToken     string
	PendingSubscriptionDeleteGroupID   string
	PendingSubscriptionDeleteExpiresAt time.Time
	PendingChatUnlinkToken             string
	PendingChatUnlinkChatID            string
	PendingChatUnlinkExpiresAt         time.Time
	GroupActive                        bool
}

type SearchType string

const (
	SearchTypeGroup      SearchType = "group"
	SearchTypeTeacher    SearchType = "teacher"
	SearchTypeRoom       SearchType = "room"
	SearchTypeDiscipline SearchType = "discipline"
)
