package expr

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokString
	tokNumber
	tokDot
	tokStar
	tokLParen
	tokRParen
	tokLBracket
	tokRBracket
	tokComma
	tokEq
	tokNeq
	tokLt
	tokLte
	tokGt
	tokGte
	tokAnd
	tokOr
	tokNot
)

type token struct {
	kind tokenKind
	text string
	num  float64
	pos  int
}

func lex(src string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			toks = append(toks, token{kind: tokLParen, pos: i})
			i++
		case c == ')':
			toks = append(toks, token{kind: tokRParen, pos: i})
			i++
		case c == '[':
			toks = append(toks, token{kind: tokLBracket, pos: i})
			i++
		case c == ']':
			toks = append(toks, token{kind: tokRBracket, pos: i})
			i++
		case c == ',':
			toks = append(toks, token{kind: tokComma, pos: i})
			i++
		case c == '.':
			toks = append(toks, token{kind: tokDot, pos: i})
			i++
		case c == '*':
			toks = append(toks, token{kind: tokStar, pos: i})
			i++
		case c == '\'':
			s, n, err := lexString(src[i:])
			if err != nil {
				return nil, fmt.Errorf("expr: at %d: %w", i, err)
			}
			toks = append(toks, token{kind: tokString, text: s, pos: i})
			i += n
		case c == '=' && i+1 < len(src) && src[i+1] == '=':
			toks = append(toks, token{kind: tokEq, pos: i})
			i += 2
		case c == '!' && i+1 < len(src) && src[i+1] == '=':
			toks = append(toks, token{kind: tokNeq, pos: i})
			i += 2
		case c == '!':
			toks = append(toks, token{kind: tokNot, pos: i})
			i++
		case c == '<' && i+1 < len(src) && src[i+1] == '=':
			toks = append(toks, token{kind: tokLte, pos: i})
			i += 2
		case c == '<':
			toks = append(toks, token{kind: tokLt, pos: i})
			i++
		case c == '>' && i+1 < len(src) && src[i+1] == '=':
			toks = append(toks, token{kind: tokGte, pos: i})
			i += 2
		case c == '>':
			toks = append(toks, token{kind: tokGt, pos: i})
			i++
		case c == '&' && i+1 < len(src) && src[i+1] == '&':
			toks = append(toks, token{kind: tokAnd, pos: i})
			i += 2
		case c == '|' && i+1 < len(src) && src[i+1] == '|':
			toks = append(toks, token{kind: tokOr, pos: i})
			i += 2
		case isDigit(c) || (c == '-' && i+1 < len(src) && isDigit(src[i+1])):
			j := scanNumber(src, i)

			f, err := strconv.ParseFloat(src[i:j], 64)
			if err != nil {
				if n := parseNumber(src[i:j]); !math.IsNaN(n) {
					f = n
				} else {
					return nil, fmt.Errorf("expr: at %d: bad number %q", i, src[i:j])
				}
			}

			toks = append(toks, token{kind: tokNumber, num: f, pos: i})
			i = j
		case isIdentStart(c):
			j := i
			for j < len(src) && isIdentPart(src[j]) {
				j++
			}
			toks = append(toks, token{kind: tokIdent, text: src[i:j], pos: i})
			i = j
		default:
			return nil, fmt.Errorf("expr: at %d: unexpected character %q", i, string(c))
		}
	}
	return append(toks, token{kind: tokEOF, pos: len(src)}), nil
}

func lexString(src string) (string, int, error) {
	var sb strings.Builder
	i := 1
	for i < len(src) {
		if src[i] == '\'' {
			if i+1 < len(src) && src[i+1] == '\'' {
				sb.WriteByte('\'')
				i += 2
				continue
			}
			return sb.String(), i + 1, nil
		}
		sb.WriteByte(src[i])
		i++
	}
	return "", 0, fmt.Errorf("expr: unterminated string")
}

func scanNumber(src string, i int) int {
	j := i

	if src[j] == '-' {
		j++
	} else if j+2 < len(src) && src[j] == '0' && isRadix(src[j+1]) {
		j += 2

		for j < len(src) && isHexDigit(src[j]) {
			j++
		}

		return j
	}

	for j < len(src) && (isDigit(src[j]) || src[j] == '.') {
		j++
	}

	if j < len(src) && (src[j] == 'e' || src[j] == 'E') {
		k := j + 1

		if k < len(src) && (src[k] == '-' || src[k] == '+') {
			k++
		}

		if k < len(src) && isDigit(src[k]) {
			for k < len(src) && isDigit(src[k]) {
				k++
			}

			j = k
		}
	}

	return j
}

func isRadix(c byte) bool { return c|32 == 'x' || c|32 == 'o' || c|32 == 'b' }

func isHexDigit(c byte) bool { return isDigit(c) || (c|32 >= 'a' && c|32 <= 'f') }

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool { return c == '_' || (c|32 >= 'a' && c|32 <= 'z') }
func isIdentPart(c byte) bool  { return isIdentStart(c) || isDigit(c) || c == '-' }
