package cursor

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"margo.sh/golang/goutil"
	"margo.sh/mg"
	"margo.sh/mgutil"
	yotsuba "margo.sh/why_would_you_make_yotsuba_cry"
	"reflect"
	"strings"
)

var (
	_ ast.Node = (*DocNode)(nil)
)

type DocNode struct {
	Node ast.Node
	ast.CommentGroup
}

type CurCtx struct {
	Ctx        *mg.Ctx
	View       *mg.View
	Scope      CurScope
	PkgName    string
	IsTestFile bool
	Line       []byte
	Pos        token.Pos
	AstFile    *ast.File
	TokenFile  *token.File
	Doc        *DocNode

	GenDecl    *ast.GenDecl
	ImportSpec *ast.ImportSpec
	Comment    *ast.Comment
	BlockStmt  *ast.BlockStmt
	CallExpr   *ast.CallExpr
	BasicLit   *ast.BasicLit
	Nodes      []ast.Node
	Node       ast.Node
}

func NewViewCurCtx(mx *mg.Ctx) *CurCtx {
	type Key struct{ *mg.View }
	k := Key{mx.View}
	if cx, ok := mx.Store.Get(k).(*CurCtx); ok {
		return cx
	}

	src, pos := k.SrcPos()
	cx := NewCurCtx(mx, src, pos)
	mx.Store.Put(k, cx)
	return cx
}

func NewCurCtx(mx *mg.Ctx, src []byte, pos int) *CurCtx {
	type Key struct {
		hash string
		pos  int
	}
	key := Key{mg.SrcHash(src), pos}
	if cx, ok := mx.Store.Get(key).(*CurCtx); ok {
		return cx
	}

	cx := newCurCtx(mx, src, pos)
	mx.Store.Put(key, cx)
	return cx
}

func newCurCtx(mx *mg.Ctx, src []byte, pos int) *CurCtx {
	pos = mgutil.ClampPos(src, pos)

	// if we're at the end of the line, move the cursor onto the last thing on the line
	space := func(r rune) bool { return r == ' ' || r == '\t' }
	if i := mgutil.RepositionRight(src, pos, space); i < len(src) && src[i] == '\n' {
		pos = mgutil.RepositionLeft(src, pos, space)
		if j := pos - 1; j >= 0 && src[j] != '\n' && src[j] != '}' {
			pos = j
		}
	}

	ll := mgutil.RepositionLeft(src, pos, func(r rune) bool { return r != '\n' })
	lr := mgutil.RepositionRight(src, pos, func(r rune) bool { return r != '\n' })
	cx := &CurCtx{
		Ctx:  mx,
		View: mx.View,
		Line: bytes.TrimSpace(src[ll:lr]),
	}
	cx.init(mx.Store, src, pos)

	af := cx.AstFile
	if af == nil {
		af = goutil.NilAstFile
	}
	cx.PkgName = af.Name.String()

	cx.IsTestFile = strings.HasSuffix(mx.View.Filename(), "_test.go") ||
		strings.HasSuffix(cx.PkgName, "_test")

	if cx.Comment != nil {
		cx.Scope |= CommentScope
	}
	if cx.Doc != nil {
		cx.Scope |= DocScope
		cx.Scope |= CommentScope
	}

	if cx.PkgName == goutil.NilPkgName || cx.PkgName == "" {
		cx.PkgName = goutil.NilPkgName
		cx.Scope |= PackageScope
		return cx
	}

	switch x := cx.Node.(type) {
	case nil:
		cx.Scope |= PackageScope
	case *ast.File:
		cx.Scope |= FileScope
	case *ast.BlockStmt:
		cx.Scope |= BlockScope
	case *ast.CaseClause:
		if goutil.NodeEnclosesPos(goutil.PosEnd{P: x.Colon, E: x.End()}, cx.Pos) {
			cx.Scope |= BlockScope
		}
	case *ast.Ident:
		cx.Scope |= IdentScope
	}

	cx.Each(func(n ast.Node) {
		switch n.(type) {
		case *ast.AssignStmt:
			cx.Scope |= AssignmentScope
		case *ast.SelectorExpr:
			cx.Scope |= SelectorScope
		case *ast.ReturnStmt:
			cx.Scope |= ReturnScope
		case *ast.DeferStmt:
			cx.Scope |= DeferScope
		}
	})

	if gd := cx.GenDecl; gd != nil {
		switch gd.Tok {
		case token.IMPORT:
			cx.Scope |= ImportScope
		case token.CONST:
			cx.Scope |= ConstScope
		case token.VAR:
			cx.Scope |= VarScope
		}
	}

	if lit := cx.BasicLit; lit != nil && lit.Kind == token.STRING {
		cx.Scope |= StringScope
		if cx.ImportSpec != nil {
			cx.Scope |= ImportPathScope
		}
	}

	// we want to allow `kw`, `kw name`, `kw (\n|\n)`
	punct := func(r rune) bool { return r != ' ' && r != '\t' && !goutil.IsLetter(r) }
	if cx.Scope == 0 && bytes.IndexFunc(cx.Line, punct) < 0 {
		switch x := cx.Node.(type) {
		case *ast.FuncType:
			cx.Scope |= FuncDeclScope
		case *ast.GenDecl:
			if x.Tok == token.TYPE {
				cx.Scope |= TypeDeclScope
			}
		}
	}

	exprOk := cx.Scope.Is(
		AssignmentScope,
		BlockScope,
		ConstScope,
		DeferScope,
		ReturnScope,
		VarScope,
	) && !cx.Scope.Is(
		SelectorScope,
		StringScope,
		CommentScope,
	)
	if asn := (*ast.AssignStmt)(nil); exprOk && cx.Set(&asn) {
		exprOk = pos >= cx.TokenFile.Offset(asn.TokPos)+len(asn.Tok.String())
	}
	if exprOk {
		cx.Scope |= ExprScope
	}

	return cx
}

// FuncDeclName returns the name of the FuncDecl iff the cursor is on a func declariton's name.
// isMethod is true if the declaration is a method.
func (cx *CurCtx) FuncDeclName() (name string, isMethod bool) {
	var fd *ast.FuncDecl
	if !cx.Set(&fd) {
		return "", false
	}
	if fd.Name == nil || !goutil.NodeEnclosesPos(fd.Name, cx.Pos) {
		return "", false
	}
	return fd.Name.Name, fd.Recv != nil
}

// FuncName returns the name of function iff the cursor is on a func declariton's name
func (cx *CurCtx) FuncName() string {
	if nm, isMeth := cx.FuncDeclName(); !isMeth {
		return nm
	}
	return ""
}

// FuncName returns the name of function iff the cursor is on a method declariton's name
func (cx *CurCtx) MethodName() string {
	if nm, isMeth := cx.FuncDeclName(); isMeth {
		return nm
	}
	return ""
}

func (cx *CurCtx) Set(destPtr interface{}) bool {
	v := reflect.ValueOf(destPtr).Elem()
	if !v.CanSet() {
		return false
	}
	for i := len(cx.Nodes) - 1; i >= 0; i-- {
		x := reflect.ValueOf(cx.Nodes[i])
		if x.Type() == v.Type() {
			v.Set(x)
			return true
		}
	}
	return false
}

func (cx *CurCtx) Each(f func(ast.Node)) {
	for i := len(cx.Nodes) - 1; i >= 0; i-- {
		f(cx.Nodes[i])
	}
}

func (cx *CurCtx) Some(f func(ast.Node) bool) bool {
	for i := len(cx.Nodes) - 1; i >= 0; i-- {
		if f(cx.Nodes[i]) {
			return true
		}
	}
	return false
}

func (cx *CurCtx) Contains(typ ast.Node) bool {
	t := reflect.TypeOf(typ)
	return cx.Some(func(n ast.Node) bool {
		return reflect.TypeOf(n) == t
	})
}

func (cx *CurCtx) append(n ast.Node) {
	// ignore bad nodes, they usually just make scope detection fail with no obvious benefit
	switch n.(type) {
	case *ast.BadDecl, *ast.BadExpr, *ast.BadStmt:
		return
	}

	for _, x := range cx.Nodes {
		if n == x {
			return
		}
	}
	cx.Nodes = append(cx.Nodes, n)
}

func (cx *CurCtx) init(kvs mg.KVStore, src []byte, offset int) {
	astFileIsValid := func(af *ast.File) bool {
		return af.Package.IsValid() &&
			af.Name != nil &&
			af.Name.End().IsValid() &&
			af.Name.Name != ""
	}
	srcHasComments := func() bool {
		return bytes.Contains(src, []byte("//")) || bytes.Contains(src, []byte("/*"))
	}

	pf := goutil.ParseFile(kvs, "", src)
	if !astFileIsValid(pf.AstFile) && srcHasComments() {
		// we don't want any declaration errors esp. about the package name `_`
		// we don't parse with this mode by default to increase the chance of caching
		s := append(src[:len(src):len(src)], goutil.NilPkgSrc...)
		pf = goutil.ParseFileWithMode(kvs, "", s, parser.ParseComments)
	}

	af := pf.AstFile
	cx.AstFile = af
	cx.TokenFile = pf.TokenFile
	cx.Pos = token.Pos(pf.TokenFile.Base() + offset)

	cx.initDocNode(af)
	if astFileIsValid(af) && cx.Pos > af.Name.End() {
		cx.append(af)
		ast.Inspect(af, func(n ast.Node) bool {
			if goutil.NodeEnclosesPos(n, cx.Pos) {
				cx.append(n)
			}
			cx.initDocNode(n)
			return true
		})
	}

	for _, cg := range af.Comments {
		for _, c := range cg.List {
			if goutil.NodeEnclosesPos(c, cx.Pos) {
				cx.append(c)
			}
		}
	}

	if len(cx.Nodes) == 0 {
		return
	}
	cx.Node = cx.Nodes[len(cx.Nodes)-1]
	cx.Each(func(n ast.Node) {
		switch x := n.(type) {
		case *ast.GenDecl:
			cx.GenDecl = x
		case *ast.BlockStmt:
			cx.BlockStmt = x
		case *ast.BasicLit:
			cx.BasicLit = x
		case *ast.CallExpr:
			cx.CallExpr = x
		case *ast.Comment:
			cx.Comment = x
		case *ast.ImportSpec:
			cx.ImportSpec = x
		}
	})
}

func (cx *CurCtx) initDocNode(n ast.Node) {
	if cx.Doc != nil || yotsuba.IsNil(n) {
		return
	}

	setCg := func(cg *ast.CommentGroup) {
		if cx.Doc != nil || cg == nil || !goutil.NodeEnclosesPos(cg, cx.Pos) {
			return
		}
		cx.Doc = &DocNode{
			Node:         n,
			CommentGroup: *cg,
		}
	}

	switch x := n.(type) {
	case *ast.File:
		setCg(x.Doc)
	case *ast.Field:
		setCg(x.Doc)
	case *ast.GenDecl:
		setCg(x.Doc)
	case *ast.TypeSpec:
		setCg(x.Doc)
	case *ast.FuncDecl:
		setCg(x.Doc)
	case *ast.ValueSpec:
		setCg(x.Doc)
	case *ast.ImportSpec:
		setCg(x.Doc)
	}
}