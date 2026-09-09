package domain

type UserRestrictions struct {
	BotBlocked     bool `db:"bot_blocked" json:"bot_blocked"`
	SupportBlocked bool `db:"support_blocked" json:"support_blocked"`
}
