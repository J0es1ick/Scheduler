package migration

import "embed"

// Files contains all forward-only SQL migrations shipped with the bot.
//
//go:embed *.up.sql
var Files embed.FS
