package dto

import "time"

type UserState struct {
	UniversityID                       string
	University                         string
	SearchType                         SearchType
	Query                              string
	GroupID                            string
	SearchQuery                        string
	HotlineType                        string
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
