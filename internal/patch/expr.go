package patch

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// Expr is a selector expression, as written in "on" and "match". It looks
// like:
//
//	category("Module") and not attribute("speed")
//
// The functions are category, group, name, attribute, effect and published.
// They take any number of names and match when one of them holds. A bare word
// is the name of a selector declared elsewhere.
//
// The text is parsed as soon as it is read; the names in it are only looked up
// once the SDE is loaded.
type Expr struct {
	Text string
	tree Selector
}

func (e *Expr) UnmarshalYAML(node *yaml.Node) error {
	if err := node.Decode(&e.Text); err != nil {
		return err
	}

	tree, err := parse(e.Text)
	if err != nil {
		return fmt.Errorf("line %d: %w", node.Line, err)
	}
	e.tree = tree
	return nil
}

func (e Expr) MarshalYAML() (any, error) { return e.Text, nil }

func (e Expr) MarshalJSON() ([]byte, error) { return json.Marshal(e.Text) }

func (e *Expr) UnmarshalJSON(raw []byte) error {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return err
	}
	parsed, err := ParseExpr(text)
	if err != nil {
		return err
	}
	*e = parsed
	return nil
}

func (e Expr) String() string { return e.Text }

// ParseExpr is how anything outside this package builds an expression, so
// that an editor can check one before it is saved.
func ParseExpr(text string) (Expr, error) {
	tree, err := parse(text)
	if err != nil {
		return Expr{}, err
	}
	return Expr{Text: text, tree: tree}, nil
}

type tokenKind int

const (
	tokenEnd tokenKind = iota
	tokenWord
	tokenString
	tokenOpen
	tokenClose
	tokenComma
)

type token struct {
	kind tokenKind
	text string
	pos  int
}

func (t token) String() string {
	switch t.kind {
	case tokenEnd:
		return "end of expression"
	case tokenString:
		return fmt.Sprintf("%q", t.text)
	default:
		return t.text
	}
}

func lex(text string) ([]token, error) {
	var tokens []token
	runes := []rune(text)

	for i := 0; i < len(runes); {
		start := i
		switch c := runes[i]; {
		case unicode.IsSpace(c):
			i++

		case c == '(':
			tokens = append(tokens, token{kind: tokenOpen, text: "(", pos: start})
			i++
		case c == ')':
			tokens = append(tokens, token{kind: tokenClose, text: ")", pos: start})
			i++
		case c == ',':
			tokens = append(tokens, token{kind: tokenComma, text: ",", pos: start})
			i++

		case c == '"':
			i++
			var value strings.Builder
			for i < len(runes) && runes[i] != '"' {
				if runes[i] == '\\' && i+1 < len(runes) {
					i++
				}
				value.WriteRune(runes[i])
				i++
			}
			if i == len(runes) {
				return nil, fmt.Errorf("unterminated string at position %d", start+1)
			}
			i++
			tokens = append(tokens, token{kind: tokenString, text: value.String(), pos: start})

		case isWordRune(c):
			for i < len(runes) && isWordRune(runes[i]) {
				i++
			}
			tokens = append(tokens, token{kind: tokenWord, text: string(runes[start:i]), pos: start})

		default:
			return nil, fmt.Errorf("unexpected character %q at position %d", string(c), start+1)
		}
	}

	return append(tokens, token{kind: tokenEnd, pos: len(runes)}), nil
}

func isWordRune(c rune) bool {
	return c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c)
}

type parser struct {
	tokens []token
	at     int
}

// parse reads a whole expression. The grammar is the usual three levels:
// "or" binds loosest, then "and", then "not" and the primaries.
func parse(text string) (Selector, error) {
	tokens, err := lex(text)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 1 {
		return nil, fmt.Errorf("expression is empty")
	}

	p := &parser{tokens: tokens}
	tree, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if next := p.peek(); next.kind != tokenEnd {
		return nil, fmt.Errorf("unexpected %s at position %d", next, next.pos+1)
	}
	return tree, nil
}

func (p *parser) peek() token { return p.tokens[p.at] }
func (p *parser) next() token { t := p.tokens[p.at]; p.at++; return t }
func (p *parser) keyword(word string) bool {
	if t := p.peek(); t.kind == tokenWord && t.text == word {
		p.at++
		return true
	}
	return false
}

func (p *parser) parseOr() (Selector, error) {
	terms, err := p.parseSequence("or", (*parser).parseAnd)
	if err != nil || len(terms) == 1 {
		return single(terms), err
	}
	return &anyOf{selectors: terms}, nil
}

func (p *parser) parseAnd() (Selector, error) {
	terms, err := p.parseSequence("and", (*parser).parseUnary)
	if err != nil || len(terms) == 1 {
		return single(terms), err
	}
	return &allOf{selectors: terms}, nil
}

func (p *parser) parseSequence(word string, term func(*parser) (Selector, error)) ([]Selector, error) {
	first, err := term(p)
	if err != nil {
		return nil, err
	}

	terms := []Selector{first}
	for p.keyword(word) {
		next, err := term(p)
		if err != nil {
			return nil, err
		}
		terms = append(terms, next)
	}
	return terms, nil
}

func single(terms []Selector) Selector {
	if len(terms) == 0 {
		return nil
	}
	return terms[0]
}

func (p *parser) parseUnary() (Selector, error) {
	if p.keyword("not") {
		inner, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &not{selector: inner}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (Selector, error) {
	t := p.next()
	switch t.kind {
	case tokenOpen:
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if close := p.next(); close.kind != tokenClose {
			return nil, fmt.Errorf("want %q at position %d, got %s", ")", close.pos+1, close)
		}
		return inner, nil

	case tokenWord:
		if t.text == "and" || t.text == "or" || t.text == "not" {
			return nil, fmt.Errorf("unexpected %q at position %d", t.text, t.pos+1)
		}
		if p.peek().kind != tokenOpen {
			return &ref{name: t.text}, nil
		}
		return p.parseCall(t)

	default:
		return nil, fmt.Errorf("want a condition at position %d, got %s", t.pos+1, t)
	}
}

func (p *parser) parseCall(name token) (Selector, error) {
	p.next() // the "(" we already peeked at

	var args []string
	for p.peek().kind != tokenClose {
		arg := p.next()
		if arg.kind != tokenString {
			return nil, fmt.Errorf("want a quoted name at position %d, got %s", arg.pos+1, arg)
		}
		if p.peek().kind == tokenEnd {
			return nil, fmt.Errorf("missing %q at position %d", ")", p.peek().pos+1)
		}
		args = append(args, arg.text)

		if p.peek().kind == tokenComma {
			p.next()
		} else if p.peek().kind != tokenClose {
			return nil, fmt.Errorf("want %q or %q at position %d", ",", ")", p.peek().pos+1)
		}
	}
	p.next() // the ")"

	return newCall(name.text, args, name.pos)
}
