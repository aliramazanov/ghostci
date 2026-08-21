package expr

import "fmt"

type parser struct {
	toks []token
	pos  int
}

func Parse(src string) (Node, error) {
	toks, err := lex(src)

	if err != nil {
		return nil, err
	}

	p := &parser{toks: toks}
	n, err := p.parseExpr(0)

	if err != nil {
		return nil, err
	}

	if p.peek().kind != tokEOF {
		return nil, fmt.Errorf("expr: unexpected trailing input at %d", p.peek().pos)
	}

	return n, nil
}

func (p *parser) peek() token { return p.toks[p.pos] }

func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }

func (p *parser) at(k tokenKind) bool { return p.peek().kind == k }

func (p *parser) expect(k tokenKind, what string) error {
	if !p.at(k) {
		return fmt.Errorf("expr: expected %s at %d", what, p.peek().pos)
	}

	p.pos++

	return nil
}

func (p *parser) parseExpr(minPrec int) (Node, error) {
	left, err := p.parseUnary()

	if err != nil {
		return nil, err
	}

	for {
		prec, ok := precedence[p.peek().kind]

		if !ok || prec < minPrec {
			return left, nil
		}

		op := p.next().kind
		right, err := p.parseExpr(prec + 1)

		if err != nil {
			return nil, err
		}

		left = Binary{Op: op, Left: left, Right: right}
	}
}

func (p *parser) parseUnary() (Node, error) {
	if p.at(tokNot) {
		p.next()
		operand, err := p.parseUnary()

		if err != nil {
			return nil, err
		}

		return Unary{Op: tokNot, Operand: operand}, nil
	}

	return p.parsePostfix()
}

func (p *parser) parsePostfix() (Node, error) {
	n, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}

	for {
		switch {
		case p.at(tokDot):
			p.next()
			if p.at(tokStar) {
				p.next()
				if !p.at(tokDot) {
					n = Splat{Target: n, Name: ""}
					continue
				}
				p.next()
				name := p.peek()
				if name.kind != tokIdent {
					return nil, fmt.Errorf("expr: expected property name after .*. at %d", name.pos)
				}
				p.next()
				n = Splat{Target: n, Name: name.text}
				continue
			}
			name := p.peek()
			if name.kind != tokIdent {
				return nil, fmt.Errorf("expr: expected property name at %d", name.pos)
			}
			p.next()
			n = Property{Target: n, Name: name.text}
		case p.at(tokLBracket):
			p.next()
			key, err := p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			if err := p.expect(tokRBracket, "]"); err != nil {
				return nil, err
			}
			n = Index{Target: n, Key: key}
		default:
			return n, nil
		}
	}
}

func (p *parser) parsePrimary() (Node, error) {
	t := p.peek()
	switch t.kind {
	case tokNumber:
		p.next()
		return Literal{Value: t.num}, nil
	case tokString:
		p.next()
		return Literal{Value: t.text}, nil
	case tokLParen:
		p.next()
		n, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if err := p.expect(tokRParen, ")"); err != nil {
			return nil, err
		}
		return n, nil
	case tokIdent:
		p.next()
		switch lower(t.text) {
		case "true":
			return Literal{Value: true}, nil
		case "false":
			return Literal{Value: false}, nil
		case "null":
			return Literal{Value: nil}, nil
		}

		if p.at(tokLParen) {
			p.next()
			var args []Node
			if !p.at(tokRParen) {
				for {
					a, err := p.parseExpr(0)
					if err != nil {
						return nil, err
					}
					args = append(args, a)
					if !p.at(tokComma) {
						break
					}
					p.next()
				}
			}

			if err := p.expect(tokRParen, ")"); err != nil {
				return nil, err
			}

			return Call{Name: lower(t.text), Args: args}, nil
		}

		return Ident{Name: t.text}, nil
	}

	return nil, fmt.Errorf("expr: unexpected token at %d", t.pos)
}
