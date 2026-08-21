package expr

type Node interface{ node() }

type Literal struct{ Value any }

type Ident struct{ Name string }

type Property struct {
	Target Node
	Name   string
}

type Index struct {
	Target Node
	Key    Node
}

type Splat struct {
	Target Node
	Name   string
}

type Call struct {
	Name string
	Args []Node
}

type Unary struct {
	Op      tokenKind
	Operand Node
}

type Binary struct {
	Op          tokenKind
	Left, Right Node
}

func (Literal) node() {}

func (Ident) node() {}

func (Property) node() {}

func (Index) node() {}

func (Splat) node() {}

func (Call) node() {}

func (Unary) node() {}

func (Binary) node() {}

var precedence = map[tokenKind]int{
	tokOr:  1,
	tokAnd: 2,
	tokEq:  3,
	tokNeq: 3,
	tokLt:  4,
	tokLte: 4,
	tokGt:  4,
	tokGte: 4,
}
