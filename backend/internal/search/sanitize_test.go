package search

import "testing"

func TestSanitizeQuery(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{name: "empty", input: "", want: "", wantOK: false},
		{name: "whitespace only", input: "   \t\n  ", want: "", wantOK: false},
		{name: "single word", input: "数学", want: "数学*", wantOK: true},
		{name: "trim and lowercase", input: "  Hello World  ", want: "hello* world*", wantOK: true},
		{name: "multiple spaces", input: "foo    bar", want: "foo* bar*", wantOK: true},
		{name: "special chars stripped", input: `foo"bar(baz)`, want: "foo* bar* baz*", wantOK: true},
		{name: "all special", input: `*"():^{}+-|!`, want: "", wantOK: false},
		{name: "mixed CJK and ASCII", input: "线性代数 Linear", want: "线性代数* linear*", wantOK: true},
		{name: "control characters removed", input: "hello\x00world", want: "hello* world*", wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SanitizeQuery(tt.input)
			if ok != tt.wantOK {
				t.Errorf("SanitizeQuery(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("SanitizeQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeTagName(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{name: "empty", input: "", want: "", wantOK: false},
		{name: "spaces", input: "  ", want: "", wantOK: false},
		{name: "normal", input: "  Math  ", want: "math", wantOK: true},
		{name: "CJK", input: "数学", want: "数学", wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SanitizeTagName(tt.input)
			if ok != tt.wantOK {
				t.Errorf("SanitizeTagName(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("SanitizeTagName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
