package gitlab

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/aliramazanov/ghostci/internal/pipeline"
)

type Vars map[string]pipeline.Value

func (v Vars) lookup(name string) pipeline.Value {
	if got, ok := v[name]; ok {
		return got
	}

	if strings.HasPrefix(name, "CI_") || strings.HasPrefix(name, "GITLAB_") {
		return pipeline.Unknown("$" + name)
	}

	return pipeline.Undefined()
}

func EvalRule(src string, vars Vars) (bool, error) {
	p := &parser{input: src, vars: vars}

	p.skipSpace()
	if p.eof() {
		return true, nil
	}

	v, err := p.parseOr()
	if err != nil {
		return false, err
	}

	p.skipSpace()
	if !p.eof() {
		return false, fmt.Errorf("gitlab: unexpected %q in rule", p.rest())
	}

	if v.Unknown {
		return false, undecidable(v)
	}

	return truthy(v), nil
}

type parser struct {
	input string
	pos   int
	vars  Vars
}

type operand struct {
	pipeline.Value
	regex *regexp.Regexp
	isNil bool
}

func (p *parser) parseOr() (operand, error) {
	left, err := p.parseAnd()
	if err != nil {
		return operand{}, err
	}

	for {
		if !p.accept("||") {
			return left, nil
		}

		if !left.Unknown && truthy(left) {
			if _, err := p.parseAnd(); err != nil {
				return operand{}, err
			}
			left = boolOperand(true)

			continue
		}

		right, err := p.parseAnd()
		if err != nil {
			return operand{}, err
		}
		if left.Unknown {
			return operand{}, undecidable(left)
		}
		left = boolOperand(truthy(right))
	}
}

func (p *parser) parseAnd() (operand, error) {
	left, err := p.parseComparison()
	if err != nil {
		return operand{}, err
	}

	for {
		if !p.accept("&&") {
			return left, nil
		}

		if !left.Unknown && !truthy(left) {
			if _, err := p.parseComparison(); err != nil {
				return operand{}, err
			}
			left = boolOperand(false)

			continue
		}

		right, err := p.parseComparison()
		if err != nil {
			return operand{}, err
		}
		if left.Unknown {
			return operand{}, undecidable(left)
		}
		left = boolOperand(truthy(right))
	}
}

func (p *parser) parseComparison() (operand, error) {
	left, err := p.parseUnary()
	if err != nil {
		return operand{}, err
	}

	p.skipSpace()

	for _, op := range []string{"==", "!=", "=~", "!~"} {
		if !p.accept(op) {
			continue
		}

		right, err := p.parseUnary()
		if err != nil {
			return operand{}, err
		}
		if left.Unknown {
			return operand{}, undecidable(left)
		}
		if right.Unknown {
			return operand{}, undecidable(right)
		}

		return compare(op, left, right)
	}

	return left, nil
}

func (p *parser) parseUnary() (operand, error) {
	p.skipSpace()

	if p.accept("!") {
		v, err := p.parseUnary()
		if err != nil {
			return operand{}, err
		}
		if v.Unknown {
			return operand{}, undecidable(v)
		}

		return boolOperand(!truthy(v)), nil
	}

	return p.parsePrimary()
}

func (p *parser) parsePrimary() (operand, error) {
	p.skipSpace()

	switch {
	case p.eof():
		return operand{}, fmt.Errorf("gitlab: rule ends early")

	case p.accept("("):
		v, err := p.parseOr()
		if err != nil {
			return operand{}, err
		}
		p.skipSpace()
		if !p.accept(")") {
			return operand{}, fmt.Errorf("gitlab: unclosed ( in rule")
		}

		return v, nil

	case p.peek() == '$':
		return p.parseVariable()

	case p.peek() == '"' || p.peek() == '\'':
		return p.parseString()

	case p.peek() == '/':
		return p.parseRegex()
	}

	word := p.parseWord()
	switch strings.ToLower(word) {
	case "null":
		return operand{isNil: true}, nil
	case "true":
		return boolOperand(true), nil
	case "false":
		return boolOperand(false), nil
	case "":
		return operand{}, fmt.Errorf("gitlab: unexpected %q in rule", p.rest())
	}

	return operand{Value: pipeline.Known(word)}, nil
}

func (p *parser) parseVariable() (operand, error) {
	p.pos++

	braced := p.accept("{")
	name := p.parseWord()
	if braced && !p.accept("}") {
		return operand{}, fmt.Errorf("gitlab: unclosed ${ in rule")
	}
	if name == "" {
		return operand{}, fmt.Errorf("gitlab: $ without a name in rule")
	}

	return operand{Value: p.vars.lookup(name)}, nil
}

func (p *parser) parseString() (operand, error) {
	quote := p.input[p.pos]
	p.pos++

	var sb strings.Builder
	for !p.eof() {
		c := p.input[p.pos]
		if c == '\\' && p.pos+1 < len(p.input) {
			sb.WriteByte(p.input[p.pos+1])
			p.pos += 2

			continue
		}
		if c == quote {
			p.pos++

			return operand{Value: pipeline.Known(sb.String())}, nil
		}
		sb.WriteByte(c)
		p.pos++
	}

	return operand{}, fmt.Errorf("gitlab: unterminated string in rule")
}

func (p *parser) parseRegex() (operand, error) {
	p.pos++

	var sb strings.Builder
	for !p.eof() {
		c := p.input[p.pos]
		if c == '\\' && p.pos+1 < len(p.input) {
			sb.WriteByte(c)
			sb.WriteByte(p.input[p.pos+1])
			p.pos += 2

			continue
		}
		if c == '/' {
			p.pos++

			flags := p.parseWord()
			pattern := sb.String()
			if strings.Contains(flags, "i") {
				pattern = "(?i)" + pattern
			}

			re, err := regexp.Compile(pattern)
			if err != nil {
				return operand{}, fmt.Errorf("gitlab: rule regex %q: %w", sb.String(), err)
			}

			return operand{regex: re}, nil
		}
		sb.WriteByte(c)
		p.pos++
	}

	return operand{}, fmt.Errorf("gitlab: unterminated regex in rule")
}

func (p *parser) parseWord() string {
	start := p.pos
	for !p.eof() {
		c := rune(p.input[p.pos])
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' && c != '-' && c != '.' && c != '/' {
			break
		}
		p.pos++
	}

	return p.input[start:p.pos]
}

func (p *parser) skipSpace() {
	for !p.eof() && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t' || p.input[p.pos] == '\n') {
		p.pos++
	}
}

func (p *parser) accept(tok string) bool {
	p.skipSpace()
	if strings.HasPrefix(p.input[p.pos:], tok) {
		p.pos += len(tok)

		return true
	}

	return false
}

func (p *parser) peek() byte {
	if p.eof() {
		return 0
	}

	return p.input[p.pos]
}

func (p *parser) eof() bool { return p.pos >= len(p.input) }

func (p *parser) rest() string { return strings.TrimSpace(p.input[p.pos:]) }

func compare(op string, left, right operand) (operand, error) {
	switch op {
	case "=~", "!~":
		re := right.regex
		if re == nil {
			return operand{}, fmt.Errorf("gitlab: %s needs a /regex/ on the right", op)
		}
		matched := left.Defined && re.MatchString(left.Text)

		return boolOperand(matched == (op == "=~")), nil
	}

	equal := left.Defined == right.Defined && left.Text == right.Text
	if left.isNil || right.isNil {
		equal = nilLike(left) && nilLike(right)
	}

	return boolOperand(equal == (op == "==")), nil
}

func nilLike(v operand) bool { return v.isNil || !v.Defined }

func truthy(v operand) bool { return v.Defined && v.Text != "" }

func boolOperand(b bool) operand {
	if b {
		return operand{Value: pipeline.Known("true")}
	}

	return operand{Value: pipeline.Known("")}
}

func undecidable(v operand) error { return v.Err() }
