package golang

import (
	"go/ast"
	"go/token"
	"margo.sh/mg"
	"sort"
	"strings"
)

const (
	cursorScopesStart CursorScope = 1 << iota
	AssignmentScope
	BlockScope
	CommentScope
	ConstScope
	DeclScope
	DeferScope
	DocScope
	FileScope
	IdentScope
	ImportPathScope
	ImportScope
	PackageScope
	ReturnScope
	SelectorScope
	StringScope
	TypeScope
	VarScope
	cursorScopesEnd
)

var (
	cursorScopeNames = map[CursorScope]string{
		AssignmentScope: "AssignmentScope",
		BlockScope:      "BlockScope",
		CommentScope:    "CommentScope",
		ConstScope:      "ConstScope",
		DeclScope:       "DeclScope",
		DeferScope:      "DeferScope",
		DocScope:        "DocScope",
		FileScope:       "FileScope",
		IdentScope:      "IdentScope",
		ImportPathScope: "ImportPathScope",
		ImportScope:     "ImportScope",
		PackageScope:    "PackageScope",
		ReturnScope:     "ReturnScope",
		SelectorScope:   "SelectorScope",
		StringScope:     "StringScope",
		TypeScope:       "TypeScope",
		VarScope:        "VarScope",
	}

	_ ast.Node = (*DocNode)(nil)
)

type CursorScope uint64
type CompletionScope = CursorScope

func (cs CursorScope) String() string {
	if cs <= cursorScopesStart || cs >= cursorScopesEnd {
		return "UnknownCursorScope"
	}
	l := []string{}
	for scope, name := range cursorScopeNames {
		if cs.Any(scope) {
			l = append(l, name)
		}
	}
	sort.Strings(l)
	return strings.Join(l, "|")
}

func (cs CursorScope) Is(scopes ...CursorScope) bool {
	for _, s := range scopes {
		if s == cs {
			return true
		}
	}
	return false
}

func (cs CursorScope) Any(scopes ...CursorScope) bool {
	for _, s := range scopes {
		if cs&s != 0 {
			return true
		}
	}
	return false
}

func (cs CursorScope) All(scopes ...CursorScope) bool {
	for _, s := range scopes {
		if cs&s == 0 {
			return false
		}
	}
	return true
}

type DocNode struct {
	Node ast.Node
	ast.CommentGroup
}

type CompletionCtx = CursorCtx
type CursorCtx struct {
	cursorNode
	Ctx        *mg.Ctx
	View       *mg.View
	Scope      CursorScope
	PkgName    string
	IsTestFile bool
}

func NewCompletionCtx(mx *mg.Ctx, src []byte, pos int) *CompletionCtx {
	return NewCursorCtx(mx, src, pos)
}

func NewViewCursorCtx(mx *mg.Ctx) *CursorCtx {
	src, pos := mx.View.SrcPos()
	return NewCursorCtx(mx, src, pos)
}

func NewCursorCtx(mx *mg.Ctx, src []byte, pos int) *CursorCtx {
	cx := &CursorCtx{
		Ctx:  mx,
		View: mx.View,
	}
	cx.init(mx.Store, src, pos)

	af := cx.AstFile
	if af == nil {
		af = NilAstFile
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

	if cx.PkgName == NilPkgName || cx.PkgName == "" {
		cx.PkgName = NilPkgName
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
		if NodeEnclosesPos(PosEnd{x.Colon, x.End()}, cx.Pos) {
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
		case token.TYPE:
			cx.Scope |= TypeScope
		}
	}

	if lit := cx.BasicLit; lit != nil && lit.Kind == token.STRING {
		cx.Scope |= StringScope
		if cx.ImportSpec != nil {
			cx.Scope |= ImportPathScope
		}
	}

	return cx
}

func (cx *CursorCtx) funcName() (name string, isMethod bool) {
	var fd *ast.FuncDecl
	if !cx.Set(&fd) {
		return "", false
	}
	if fd.Name == nil || !NodeEnclosesPos(fd.Name, cx.Pos) {
		return "", false
	}
	return fd.Name.Name, fd.Recv != nil
}

// FuncName returns the name of function iff the cursor is on a func declariton's name
func (cx *CursorCtx) FuncName() string {
	if nm, isMeth := cx.funcName(); !isMeth {
		return nm
	}
	return ""
}

// FuncName returns the name of function iff the cursor is on a method declariton's name
func (cx *CursorCtx) MethodName() string {
	if nm, isMeth := cx.funcName(); isMeth {
		return nm
	}
	return ""
}
