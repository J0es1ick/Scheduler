package declarative

import "testing"

func TestValidateConfig(t *testing.T) {
	valid := Config{URL: "https://schedule.example/snapshot.json", UniversityID: "example", UniversityName: "Example", Timezone: "Europe/Moscow"}
	if err := ValidateConfig(valid); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"http://schedule.example/data", "https://localhost/data", "https://127.0.0.1/data", "https://10.0.0.2/data"} {
		candidate := valid
		candidate.URL = raw
		if err := ValidateConfig(candidate); err == nil {
			t.Errorf("unsafe URL %q accepted", raw)
		}
	}
}
