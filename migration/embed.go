package migration

import (
	"embed"
	"strings"
)

// Files contains all forward-only SQL migrations shipped with the bot.
//
//go:embed *.up.sql
var Files embed.FS

//go:embed checksum-aliases.txt
var checksumAliases string

func IsKnownLegacyChecksum(name, canonical, stored string) bool {
	for line := range strings.Lines(checksumAliases) {
		fields := strings.Split(strings.TrimSpace(line), "|")
		if len(fields) == 3 && fields[0] == name && fields[1] == canonical && fields[2] == stored {
			return true
		}
	}
	return false
}
