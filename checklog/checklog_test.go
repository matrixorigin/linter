package checklog

import "testing"

func Test_isWhiteListed(t *testing.T) {
	type args struct {
		whiteList    []approved
		typeName     string
		functionName string
	}
	tests := []struct {
		name      string
		args      args
		wantMatch bool
	}{
		{
			name: "normal",
			args: args{
				//getFunction type, func: 'github.com/matrixorigin/matrixone/pkg/sql/parsers/example', 'main'
				//pkg/sql/parsers/example/eg_mysql.go:33:2: unsafe log fmt.Printf: in github.com/matrixorigin/matrixone/pkg/sql/parsers/example.main()
				// -----
				// # github.com/matrixorigin/matrixone/pkg/util/trace/example
				//getMethod type, func: 'github.com/matrixorigin/matrixone/pkg/util/trace/example.dummyStringWriter', 'WriteString'
				whiteList:    whiteList4fmt,
				typeName:     "github.com/matrixorigin/matrixone/pkg/sql/parsers/example",
				functionName: "main",
			},
			wantMatch: true,
		},
		{
			name: "normal",
			args: args{
				whiteList:    whiteList4fmt,
				typeName:     "github.com/matrixorigin/matrixone/pkg/util/trace/example.dummyStringWriter",
				functionName: "WriteString",
			},
			wantMatch: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotMatch := isWhiteListed(tt.args.whiteList, tt.args.typeName, tt.args.functionName); gotMatch != tt.wantMatch {
				t.Errorf("isWhiteListed() = %v, want %v", gotMatch, tt.wantMatch)
			}
		})
	}
}
