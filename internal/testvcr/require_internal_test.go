package testvcr

import "testing"

func TestRequireCassettesPresent(t *testing.T) {
	keys := []string{"CI_REQUIRE_CASSETTES", "CI", "GITHUB_ACTIONS", "BUILDKITE", "CIRCLECI"}
	clear := func(t *testing.T) {
		t.Helper()
		for _, k := range keys {
			t.Setenv(k, "")
		}
	}

	tests := []struct {
		name string
		set  map[string]string
		want bool
	}{
		{"all empty", nil, false},
		{"all falsy", map[string]string{"CI_REQUIRE_CASSETTES": "0", "CI": "false"}, false},
		{"flag truthy", map[string]string{"CI_REQUIRE_CASSETTES": "1"}, true},
		{"CI set", map[string]string{"CI": "true"}, true},
		{"GitHub Actions", map[string]string{"GITHUB_ACTIONS": "true"}, true},
		{"Buildkite", map[string]string{"BUILDKITE": "1"}, true},
		{"CircleCI", map[string]string{"CIRCLECI": "true"}, true},
		{"CI explicitly false", map[string]string{"CI": "false"}, false},
		{"CI explicitly 0", map[string]string{"CI": "0"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clear(t)
			for k, v := range tt.set {
				t.Setenv(k, v)
			}
			if got := requireCassettesPresent(); got != tt.want {
				t.Errorf("requireCassettesPresent() = %v, want %v", got, tt.want)
			}
		})
	}
}
