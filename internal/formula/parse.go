// Package formula is the expression language of formula fields (ADR-0030):
// a small, side-effect free language over the custom fields of one item.
//
// It is parsed by hand (no eval, no reflection), has no loops, no variables
// and no user-defined functions, and every input is bounded: source length,
// node count and nesting depth. Numbers are exact decimals (math/big.Rat with
// a fixed intermediate scale), never float64.
//
// Grammar (lowest precedence first):
//
//	expr    = or
//	or      = and { "or" and }
//	and     = not { "and" not }
//	not     = "not" not | cmp
//	cmp     = add [ ("=" | "!=" | "<>" | "<" | "<=" | ">" | ">=") add ]
//	add     = mul { ("+" | "-") mul }
//	mul     = unary { ("*" | "/") unary }
//	unary   = ("-" | "+") unary | primary
//	primary = number | "true" | "false" | ref | call | "(" expr ")"
//	ref     = key | "@due_date"
//	call    = name "(" [ expr { "," expr } ] ")"
package formula

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Limits. Every one is checked before or while building the tree, so no
// input can make the parser or the evaluator use unbounded time or memory.
const (
	MaxLength    = 1000 // bytes of source
	MaxDepth     = 40   // nesting depth of the tree
	MaxNodes     = 400  // tree nodes
	MaxArgs      = 20   // arguments of one call
	MaxRefs      = 50   // distinct references
	maxNumDigits = 30   // digits of one number literal
)

// DueDateRef is the item's built-in due date. Built-ins start with "@" so
// they never collide with custom field keys.
const DueDateRef = "@due_date"

// Error is a compile error: Pos is the 1-based character (rune) position in
// the source where the problem starts.
type Error struct {
	Pos int
	Msg string
}

func (e *Error) Error() string { return fmt.Sprintf("at %d: %s", e.Pos, e.Msg) }

type tokKind uint8

const (
	tEOF tokKind = iota
	tNum
	tIdent
	tBuiltin // @name
	tOp      // + - * / = != <> < <= > >= ( ) ,
)

type token struct {
	kind tokKind
	text string
	pos  int // 1-based rune position
}

// lex splits src into tokens. Identifiers are lowercase field keys or
// function names; anything else outside the grammar is an error.
func lex(src string) ([]token, error) {
	if len(src) > MaxLength {
		return nil, &Error{Pos: 1, Msg: fmt.Sprintf("formula is too long (at most %d characters)", MaxLength)}
	}
	if !utf8.ValidString(src) {
		return nil, &Error{Pos: 1, Msg: "formula must be valid UTF-8"}
	}
	var toks []token
	pos := 0 // rune index
	i := 0
	for i < len(src) {
		r, size := utf8.DecodeRuneInString(src[i:])
		start := pos + 1
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			i += size
			pos++
			continue
		case r >= '0' && r <= '9' || r == '.':
			j := i
			dot := false
			for j < len(src) && (src[j] >= '0' && src[j] <= '9' || src[j] == '.') {
				if src[j] == '.' {
					if dot {
						return nil, &Error{Pos: start, Msg: "invalid number"}
					}
					dot = true
				}
				j++
			}
			lit := src[i:j]
			if lit == "." || strings.HasPrefix(lit, ".") || strings.HasSuffix(lit, ".") {
				return nil, &Error{Pos: start, Msg: "invalid number " + quote(lit) + " (write 0.5, not .5 or 5.)"}
			}
			if len(lit)-boolInt(dot) > maxNumDigits {
				return nil, &Error{Pos: start, Msg: fmt.Sprintf("number has more than %d digits", maxNumDigits)}
			}
			if j < len(src) && isIdentByte(src[j]) {
				return nil, &Error{Pos: start + (j - i), Msg: "unexpected " + quote(string(src[j]))}
			}
			toks = append(toks, token{tNum, lit, start})
			pos += j - i
			i = j
			continue
		case r >= 'a' && r <= 'z' || r == '@':
			j := i + 1
			for j < len(src) && isIdentByte(src[j]) {
				j++
			}
			name := src[i:j]
			kind := tIdent
			if r == '@' {
				kind = tBuiltin
				if len(name) == 1 {
					return nil, &Error{Pos: start, Msg: "expected a name after @"}
				}
			}
			if len(name) > 64 {
				return nil, &Error{Pos: start, Msg: "name is too long"}
			}
			toks = append(toks, token{kind, name, start})
			pos += j - i
			i = j
			continue
		case r >= 'A' && r <= 'Z':
			return nil, &Error{Pos: start, Msg: "names are lowercase (field keys and functions such as round)"}
		}
		two := ""
		if i+1 < len(src) {
			two = src[i : i+2]
		}
		switch two {
		case "!=", "<>", "<=", ">=":
			toks = append(toks, token{tOp, two, start})
			i += 2
			pos += 2
			continue
		}
		switch r {
		case '+', '-', '*', '/', '=', '<', '>', '(', ')', ',':
			toks = append(toks, token{tOp, string(r), start})
			i += size
			pos++
			continue
		case '"', '\'':
			return nil, &Error{Pos: start, Msg: "text values are not supported in formulas"}
		}
		return nil, &Error{Pos: start, Msg: "unexpected character " + quote(string(r))}
	}
	toks = append(toks, token{tEOF, "", pos + 1})
	return toks, nil
}

func isIdentByte(b byte) bool { return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_' }

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func quote(s string) string { return "\"" + s + "\"" }

// Node kinds.
type nodeKind uint8

const (
	nNum nodeKind = iota
	nBool
	nRef
	nCall
	nNeg
	nNot
	nBin // arithmetic, comparison, and/or
)

type node struct {
	kind nodeKind
	pos  int
	op   string // nBin: operator; nCall: function name
	text string // nNum: literal; nRef: reference; nBool: "true"/"false"
	args []*node
	typ  Type // set by check
}

type parser struct {
	toks  []token
	i     int
	nodes int
	depth int
}

// Parse checks the syntax of src and returns its tree. Use Compile to also
// resolve references and types.
func Parse(src string) (*Tree, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	if len(toks) == 1 {
		return nil, &Error{Pos: 1, Msg: "formula is empty"}
	}
	p := &parser{toks: toks}
	root, err := p.expr()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tEOF {
		return nil, &Error{Pos: t.pos, Msg: "unexpected " + describe(t)}
	}
	if d := depthOf(root); d > MaxDepth {
		return nil, &Error{Pos: 1, Msg: fmt.Sprintf("formula is nested too deeply (at most %d levels)", MaxDepth)}
	}
	return &Tree{root: root}, nil
}

// Tree is a parsed formula.
type Tree struct{ root *node }

func depthOf(n *node) int {
	d := 0
	for _, a := range n.args {
		d = max(d, depthOf(a))
	}
	return d + 1
}

func describe(t token) string {
	if t.kind == tEOF {
		return "end of formula"
	}
	return quote(t.text)
}

func (p *parser) peek() token { return p.toks[p.i] }

func (p *parser) next() token {
	t := p.toks[p.i]
	if t.kind != tEOF {
		p.i++
	}
	return t
}

func (p *parser) isOp(s string) bool {
	t := p.peek()
	return t.kind == tOp && t.text == s
}

func (p *parser) isWord(s string) bool {
	t := p.peek()
	return t.kind == tIdent && t.text == s
}

func (p *parser) newNode(n *node) (*node, error) {
	p.nodes++
	if p.nodes > MaxNodes {
		return nil, &Error{Pos: n.pos, Msg: fmt.Sprintf("formula is too large (at most %d terms)", MaxNodes)}
	}
	return n, nil
}

// enter guards recursion: depth is bounded long before the stack matters.
func (p *parser) enter(pos int) error {
	p.depth++
	if p.depth > MaxDepth {
		return &Error{Pos: pos, Msg: fmt.Sprintf("formula is nested too deeply (at most %d levels)", MaxDepth)}
	}
	return nil
}

func (p *parser) leave() { p.depth-- }

func (p *parser) expr() (*node, error) {
	if err := p.enter(p.peek().pos); err != nil {
		return nil, err
	}
	defer p.leave()
	return p.or()
}

func (p *parser) binLoop(sub func() (*node, error), match func() (string, bool)) (*node, error) {
	left, err := sub()
	if err != nil {
		return nil, err
	}
	for {
		op, ok := match()
		if !ok {
			return left, nil
		}
		t := p.next()
		right, err := sub()
		if err != nil {
			return nil, err
		}
		if left, err = p.newNode(&node{kind: nBin, pos: t.pos, op: op, args: []*node{left, right}}); err != nil {
			return nil, err
		}
	}
}

func (p *parser) or() (*node, error) {
	return p.binLoop(p.and, func() (string, bool) { return "or", p.isWord("or") })
}

func (p *parser) and() (*node, error) {
	return p.binLoop(p.not, func() (string, bool) { return "and", p.isWord("and") })
}

func (p *parser) not() (*node, error) {
	if p.isWord("not") {
		t := p.next()
		if err := p.enter(t.pos); err != nil {
			return nil, err
		}
		defer p.leave()
		x, err := p.not()
		if err != nil {
			return nil, err
		}
		return p.newNode(&node{kind: nNot, pos: t.pos, args: []*node{x}})
	}
	return p.cmp()
}

var cmpOps = map[string]string{"=": "=", "!=": "!=", "<>": "!=", "<": "<", "<=": "<=", ">": ">", ">=": ">="}

func (p *parser) cmp() (*node, error) {
	left, err := p.add()
	if err != nil {
		return nil, err
	}
	t := p.peek()
	op, ok := cmpOps[t.text]
	if t.kind != tOp || !ok {
		return left, nil
	}
	p.next()
	right, err := p.add()
	if err != nil {
		return nil, err
	}
	if n := p.peek(); n.kind == tOp && cmpOps[n.text] != "" {
		return nil, &Error{Pos: n.pos, Msg: "comparisons cannot be chained; use and"}
	}
	return p.newNode(&node{kind: nBin, pos: t.pos, op: op, args: []*node{left, right}})
}

func (p *parser) add() (*node, error) {
	return p.binLoop(p.mul, func() (string, bool) {
		if p.isOp("+") || p.isOp("-") {
			return p.peek().text, true
		}
		return "", false
	})
}

func (p *parser) mul() (*node, error) {
	return p.binLoop(p.unary, func() (string, bool) {
		if p.isOp("*") || p.isOp("/") {
			return p.peek().text, true
		}
		return "", false
	})
}

func (p *parser) unary() (*node, error) {
	if p.isOp("-") || p.isOp("+") {
		t := p.next()
		if err := p.enter(t.pos); err != nil {
			return nil, err
		}
		defer p.leave()
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		if t.text == "+" {
			return x, nil
		}
		return p.newNode(&node{kind: nNeg, pos: t.pos, args: []*node{x}})
	}
	return p.primary()
}

func (p *parser) primary() (*node, error) {
	t := p.next()
	switch t.kind {
	case tNum:
		return p.newNode(&node{kind: nNum, pos: t.pos, text: t.text})
	case tBuiltin:
		return p.newNode(&node{kind: nRef, pos: t.pos, text: t.text})
	case tIdent:
		switch t.text {
		case "true", "false":
			return p.newNode(&node{kind: nBool, pos: t.pos, text: t.text})
		case "and", "or", "not":
			return nil, &Error{Pos: t.pos, Msg: "unexpected " + quote(t.text)}
		}
		if p.isOp("(") {
			return p.call(t)
		}
		return p.newNode(&node{kind: nRef, pos: t.pos, text: t.text})
	case tOp:
		if t.text == "(" {
			x, err := p.expr()
			if err != nil {
				return nil, err
			}
			if !p.isOp(")") {
				return nil, &Error{Pos: p.peek().pos, Msg: "expected \")\" but found " + describe(p.peek())}
			}
			p.next()
			return x, nil
		}
	}
	return nil, &Error{Pos: t.pos, Msg: "expected a number, field or function but found " + describe(t)}
}

func (p *parser) call(name token) (*node, error) {
	p.next() // (
	if err := p.enter(name.pos); err != nil {
		return nil, err
	}
	defer p.leave()
	n := &node{kind: nCall, pos: name.pos, op: name.text}
	if p.isOp(")") {
		p.next()
		return p.newNode(n)
	}
	for {
		a, err := p.expr()
		if err != nil {
			return nil, err
		}
		n.args = append(n.args, a)
		if len(n.args) > MaxArgs {
			return nil, &Error{Pos: a.pos, Msg: fmt.Sprintf("too many arguments (at most %d)", MaxArgs)}
		}
		if p.isOp(",") {
			p.next()
			continue
		}
		if p.isOp(")") {
			p.next()
			return p.newNode(n)
		}
		return nil, &Error{Pos: p.peek().pos, Msg: "expected \",\" or \")\" but found " + describe(p.peek())}
	}
}
