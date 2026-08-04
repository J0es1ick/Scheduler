package handlers

import "testing"

func TestChatGroupArguments(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantUniversity string
		wantGroup      string
		wantOK         bool
	}{
		{
			name:           "university and group",
			args:           []string{"ISUCT", "3/147"},
			wantUniversity: "isuct",
			wantGroup:      "3/147",
			wantOK:         true,
		},
		{
			name:           "colon notation",
			args:           []string{"ispu:1-40"},
			wantUniversity: "ispu",
			wantGroup:      "1-40",
			wantOK:         true,
		},
		{
			name:      "group only",
			args:      []string{"3/147"},
			wantGroup: "3/147",
			wantOK:    true,
		},
		{name: "empty", args: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			university, group, ok := chatGroupArguments(test.args)
			if university != test.wantUniversity ||
				group != test.wantGroup ||
				ok != test.wantOK {
				t.Errorf(
					"chatGroupArguments() = (%q, %q, %v), want (%q, %q, %v)",
					university,
					group,
					ok,
					test.wantUniversity,
					test.wantGroup,
					test.wantOK,
				)
			}
		})
	}
}
