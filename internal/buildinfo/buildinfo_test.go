package buildinfo

import (
	"strings"
	"testing"
)

func TestStringIncluyeTodo(t *testing.T) {
	Version, Commit, Date = "v1.2.3", "abc1234", "2026-10-30T10:00:00Z"
	s := String()
	for _, want := range []string{"v1.2.3", "abc1234", "2026-10-30T10:00:00Z"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() = %q, falta %q", s, want)
		}
	}
}
