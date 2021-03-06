package mg

const (
	AllLangs Lang = "*"

	ActionScript Lang = "actionscript"
	AppleScript  Lang = "applescript"
	ASP          Lang = "asp"
	C            Lang = "c"
	Clojure      Lang = "clojure"
	CPP          Lang = "c++"
	CSharp       Lang = "cs"
	CSS          Lang = "css"
	D            Lang = "d"
	Diff         Lang = "diff"
	DosBatch     Lang = "dosbatch"
	Dot          Lang = "dot"
	Empty        Lang = "empty"
	Erl          Lang = "erl"
	Erlang       Lang = "erlang"
	Go           Lang = "go"
	Go2          Lang = "go2"
	GoMod        Lang = "go.mod"
	GoSum        Lang = "go.sum"
	Groovy       Lang = "groovy"
	Haskell      Lang = "haskell"
	HTML         Lang = "html"
	Java         Lang = "java"
	JS           Lang = "js"
	JSON         Lang = "json"
	JSX          Lang = "JSX"
	LaTeX        Lang = "latex"
	LISP         Lang = "lisp"
	Lua          Lang = "lua"
	Makefile     Lang = "makefile"
	Matlab       Lang = "matlab"
	ObjC         Lang = "objc"
	Ocaml        Lang = "ocaml"
	Octave       Lang = "octave"
	Pascal       Lang = "pascal"
	Perl         Lang = "perl"
	PHP          Lang = "php"
	Plist        Lang = "plist"
	Python       Lang = "python"
	Rlang        Lang = "r"
	Ruby         Lang = "ruby"
	Rust         Lang = "rust"
	Scala        Lang = "scala"
	ShellScript  Lang = "shell"
	SQL          Lang = "sql"
	SVG          Lang = "svg"
	Tcl          Lang = "tcl"
	TS           Lang = "ts"
	TSX          Lang = "tsx"
	XML          Lang = "xml"
	Yaml         Lang = "yaml"
)

// Lang is the lower-case name of a language i.e. "go", not "Go"
// where possible, the predefined instances in should be used
type Lang string
