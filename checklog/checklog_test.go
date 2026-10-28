package checklog

import "testing"

func TestIsWhiteListed(t *testing.T) {
	const mysql = "github.com/matrixorigin/matrixone/pkg/sql/parsers/dialect/mysql"
	const regexp = "github.com/matrixorigin/matrixone/pkg/sql/plan/function/internal/regexp2"
	for _, tt := range []struct {
		name, pkg, owner, function string
		want                       bool
	}{
		{"exact package", mysql, mysql, "F", true},
		{"method in exact package", mysql, mysql + ".T", "F", true},
		{"sibling prefix", mysql + "_extra", mysql + "_extra", "F", false},
		{"child package", mysql + "/unaudited", mysql + "/unaudited", "F", false},
		{"example package", "example.org/foo/example", "example.org/foo/example.T", "F", true},
		{"example lookalike", "example.org/foo/example_extra", "example.org/foo/example_extra", "F", false},
		{"example child", "example.org/foo/example/other", "example.org/foo/example/other", "F", false},
		{"approved method", regexp, regexp + ".runner", "scan", true},
		{"neighbor method", regexp, regexp + ".runner", "other", false},
		{"neighbor receiver", regexp, regexp + ".other", "scan", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWhiteListed(tt.pkg, tt.owner, tt.function); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
