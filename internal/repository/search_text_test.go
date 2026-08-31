package repository

import "testing"

func TestCloseSearchMatch(t *testing.T) {
	for _, test := range []struct {
		value, query string
		want         bool
	}{
		{"  АБ   147  ", "аб 147", true}, {"4/147", "4/174", true},
		{"4/147", "4/17", true}, {"ГРУППА", "група", true},
		{"4/147", "1/999", false}, {"АБ", "АВ", false},
		{"4/147", "4/%", false}, {"ГРУППА", "ГРППУ", false},
	} {
		if got := closeSearchMatch(test.value, test.query); got != test.want {
			t.Errorf("%q / %q: got %t", test.value, test.query, got)
		}
	}
}
