package patch

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{`category("Ship")`, `category("Ship")`},
		{`published()`, `published()`},
		{`attribute("a", "b")`, `attribute("a", "b")`},
		{`not isShip`, `not isShip`},
		{`a and b and c`, `(a and b and c)`},
		{`a or b`, `(a or b)`},

		// "and" binds tighter than "or", with or without brackets.
		{`a and b or c`, `((a and b) or c)`},
		{`a and (b or c)`, `(a and (b or c))`},
		{`not a and b`, `(not a and b)`},
		{`not (a and b)`, `not (a and b)`},
	}

	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			tree, err := parse(test.text)
			if err != nil {
				t.Fatal(err)
			}
			if got := tree.String(); got != test.want {
				t.Errorf("parse(%q) = %s, want %s", test.text, got, test.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{``, "empty"},
		{`   `, "empty"},
		{`and`, `unexpected "and"`},
		{`a and`, "want a condition"},
		{`a b`, "unexpected b"},
		{`(a`, `want ")"`},
		{`a)`, "unexpected )"},
		{`nope("x")`, `unknown function "nope"`},
		{`category(x)`, "want a quoted name"},
		{`category("x"`, `missing ")"`},
		{`published("x")`, "takes no names"},
		{`category()`, "needs at least one name"},
		{`category("x`, "unterminated string"},
		{`a & b`, `unexpected character "&"`},
	}

	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			_, err := parse(test.text)
			if err == nil {
				t.Fatalf("parse(%q) did not fail", test.text)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error = %q, want it to mention %q", err, test.want)
			}
		})
	}
}
