package hlb

import (
	"bytes"
	"io"
	"os"

	"github.com/alecthomas/participle"
	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/participle/lexer/regex"
	"github.com/logrusorgru/aurora"
)

var (
	hlbLexer = lexer.Must(regex.New(`
		Keyword  = state
		Ident    = [a-zA-Z_][a-zA-Z0-9_]*
		Int      = [0-9][0-9]*
		String   = '(?:\\.|[^'])*'|"(?:\\.|[^"])*"
		Operator = {|}|;|\(|\)|,

	        whitespace = \s+
	        comment    = //[^\n]*
	`))

	parser = participle.MustBuild(
		&AST{},
		participle.Lexer(hlbLexer),
		participle.Unquote("String"),
	)
)

func Parse(r io.Reader, opts ...ParseOption) (*AST, error) {
	info := ParseInfo{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Color:  aurora.NewAurora(false),
	}

	for _, opt := range opts {
		err := opt(&info)
		if err != nil {
			return nil, err
		}
	}

	name := lexer.NameOfReader(r)
	if name == "" {
		name = "<stdin>"
	}

	ib := &indexedBuffer{buf: new(bytes.Buffer)}
	r = io.TeeReader(r, ib)

	lex, err := parser.Lexer().Lex(&namedReader{r, name})
	if err != nil {
		return nil, err
	}

	peeker, err := lexer.Upgrade(lex)
	if err != nil {
		nerr, err := newLexerError(info.Color, ib, peeker, err)
		if err != nil {
			return nil, err
		}

		return nil, nerr
	}

	ast := &AST{}
	err = parser.ParseFromLexer(peeker, ast)
	if err != nil {
		perr, ok := err.(participle.Error)
		if !ok {
			return ast, err
		}

		nerr, err := newSyntaxError(info.Color, ib, peeker, perr)
		if err != nil {
			return ast, err
		}

		return ast, nerr
	}

	return ast, nil
}

type ParseOption func(*ParseInfo) error

type ParseInfo struct {
	Stdout io.Writer
	Stderr io.Writer
	Color  aurora.Aurora
}

func WithStdout(stdout io.Writer) ParseOption {
	return func(i *ParseInfo) error {
		i.Stdout = stdout
		return nil
	}
}

func WithStderr(stderr io.Writer) ParseOption {
	return func(i *ParseInfo) error {
		i.Stderr = stderr
		return nil
	}
}

func WithColor(color bool) ParseOption {
	return func(i *ParseInfo) error {
		i.Color = aurora.NewAurora(color)
		return nil
	}
}

type AST struct {
	Pos     lexer.Position
	Entries []*Entry `( @@ ( ";" )?)*`
}

type Entry struct {
	Pos   lexer.Position
	State *StateEntry `"state"  @@`
	// Option *OptionEntry `| "option" @@`
	// Result *ResultEntry `| "result" @@`
	// Frontend *NamedFrontend `| "frontend" @@`
}

type StateEntry struct {
	Pos       lexer.Position
	Name      string     `@Ident`
	Signature *Signature `( @@ )?`
	Body      *StateBody `@@`
}

type Signature struct {
	Args []Arg `"(" ( @@ )* ")"`
}

type Arg struct {
	Type string `@Ident`
	Name string `@Ident`
}

type State struct {
	Pos      lexer.Position
	Explicit *string    `@( "state" )?`
	Body     *StateBody `@@`
}

type StateBody struct {
	Pos      lexer.Position
	Source   *Source  `"{" @@ ( ";" )?`
	Ops      []*Op    `( @@ ( ";" )? )*`
	BlockEnd BlockEnd `@@`
}

type Source struct {
	Pos       lexer.Position
	FromState *FromState ` ( "from" @@`
	From      *From      ` | "from" @@`
	Scratch   *string    `| @"scratch"`
	Image     *Image     `| "image" @@`
	HTTP      *HTTP      `| "http" @@`
	Git       *Git       `| "git" @@ )`
}

type FromState struct {
	Pos   lexer.Position
	Input *State `@@`
}

type From struct {
	Pos   lexer.Position
	Input string `@Ident`
}

type Image struct {
	Pos    lexer.Position
	Ref    string       `@String`
	Option *ImageOption `( "with" @@ )?`
}

type ImageOption struct {
	Pos         lexer.Position
	ImageFields []*ImageField `( "option" "{" ( @@ [ ";" ] )*`
	BlockEnd    BlockEnd      `@@`
	Name        *string       `| @Ident )`
}

type ImageField struct {
	Pos     lexer.Position
	Resolve *bool `@"resolve"`
}

type HTTP struct {
	Pos    lexer.Position
	URL    string      `@String`
	Option *HTTPOption `( "with" @@ )?`
}

type HTTPOption struct {
	Pos        lexer.Position
	HTTPFields []*HTTPField `( "option" "{" ( @@ [ ";" ] )*`
	BlockEnd   BlockEnd     `@@`
	Name       *string      `| @Ident )`
}

type HTTPField struct {
	Pos      lexer.Position
	Checksum *Checksum `( "checksum" @@`
	Chmod    *Chmod    `| "chmod" @@`
	Filename *Filename `| "filename" @@ )`
}

type Checksum struct {
	Pos lexer.Position

	// TODO: Use regex lexer to define custom types like digest.Digest?
	Digest string `@String`
}

type Chmod struct {
	Pos  lexer.Position
	Mode *FileMode `@@`
}

type Filename struct {
	Pos  lexer.Position
	Name string `@String`
}

type Git struct {
	Pos    lexer.Position
	Remote string     `@String`
	Ref    string     `@String`
	Option *GitOption `( "with" @@ )?`
}

type GitOption struct {
	Pos       lexer.Position
	GitFields []*GitField `( "option" "{" ( @@ [ ";" ] )*`
	BlockEnd  BlockEnd    `@@`
	Name      *string     `| @Ident )`
}

type GitField struct {
	Pos        lexer.Position
	KeepGitDir *bool `@"keepGitDir"`
}

type Op struct {
	Pos       lexer.Position
	Exec      *Exec      `( "exec" @@`
	Env       *Env       `| "env" @@`
	Dir       *Dir       `| "dir" @@`
	User      *User      `| "user" @@`
	Mkdir     *Mkdir     `| "mkdir" @@`
	Mkfile    *Mkfile    `| "mkfile" @@`
	Rm        *Rm        `| "rm" @@`
	CopyState *CopyState `| "copy" @@`
	Copy      *Copy      `| "copy" @@ )`
}

type Exec struct {
	Pos    lexer.Position
	Shlex  string      `@String`
	Option *ExecOption `( "with" @@ )?`
}

type ExecOption struct {
	Pos        lexer.Position
	ExecFields []*ExecField `( "option" "{" ( @@ [ ";" ] )*`
	BlockEnd   BlockEnd     `@@`
	Name       *string      `| @Ident )`
}

type ExecField struct {
	Pos            lexer.Position
	ReadonlyRootfs *bool       `( @"readonlyRootfs"`
	Env            *Env        `| "env" @@`
	Dir            *Dir        `| "dir" @@`
	User           *User       `| "user" @@`
	Network        *Network    `| "network" @@`
	Security       *Security   `| "security" @@`
	Host           *Host       `| "host" @@`
	SSH            *SSH        `| "ssh" @@`
	Secret         *Secret     `| "secret" @@`
	MountState     *MountState `| "mount" @@`
	Mount          *Mount      `| "mount" @@ )`
}

type Network struct {
	Pos  lexer.Position
	Mode string `@("unset" | "host" | "none")`
}

type Security struct {
	Pos  lexer.Position
	Mode string `@("sandbox" | "insecure")`
}

type Host struct {
	Pos  lexer.Position
	Name string `@String`

	// TODO: Use regex lexer to define custom types like IP?
	Address string `@String`
}

type SSH struct {
	Pos    lexer.Position
	Option *SSHOption `( "with" @@ )?`
}

type SSHOption struct {
	Pos       lexer.Position
	SSHFields []*SSHField `( "option" "{" ( @@ [ ";" ] )*`
	BlockEnd  BlockEnd    `@@`
	Name      *string     `| @Ident )`
}

type SSHField struct {
	Pos        lexer.Position
	Mountpoint *Mountpoint `( "mountpoint" @@`
	ID         *CacheID    `| @@`
	UID        *SystemID   `| "uid" @@`
	GID        *SystemID   `| "gid" @@`
	Mode       *FileMode   `| "mode" @@`
	Optional   *bool       `| @"optional" )`
}

type CacheID struct {
	Pos lexer.Position
	ID  string `"id" @String`
}

type SystemID struct {
	Pos lexer.Position
	ID  int `@Int`
}

type Mountpoint struct {
	Pos  lexer.Position
	Path string `@String`
}

type Secret struct {
	Pos        lexer.Position
	Mountpoint string        `@String`
	Option     *SecretOption `( "with" @@ )?`
}

type SecretOption struct {
	Pos          lexer.Position
	SecretFields []*SecretField `( "option" "{" ( @@ [ ";" ] )*`
	BlockEnd     BlockEnd       `@@`
	Name         *string        `| @Ident )`
}

type SecretField struct {
	Pos      lexer.Position
	ID       *CacheID  `( @@`
	UID      *SystemID `| "uid" @@`
	GID      *SystemID `| "gid" @@`
	Mode     *FileMode `| "mode" @@`
	Optional *bool     `| @"optional" )`
}

type MountState struct {
	Pos   lexer.Position
	Input *State `@@`
	MountBase
}

type Mount struct {
	Pos   lexer.Position
	Input string `@Ident`
	MountBase
}

type MountBase struct {
	Mountpoint string       `@String`
	Option     *MountOption `( "with" @@ )?`
}

type MountOption struct {
	Pos         lexer.Position
	MountFields []*MountField `( "option" "{" ( @@ [ ";" ] )*`
	BlockEnd    BlockEnd      `@@`
	Name        *string       `| @Ident )`
}

type MountField struct {
	Pos      lexer.Position
	Readonly *bool       `( @"readonly"`
	Tmpfs    *bool       `| @"tmpfs"`
	Source   *Mountpoint `| "source" @@`
	Cache    *Cache      `| "cache" @@ )`
}

type SourcePath struct {
	Pos  lexer.Position
	Path string `@String`
}

type Cache struct {
	Pos     lexer.Position
	ID      string `@String`
	Sharing string `@("shared" | "private" | "locked")`
}

type Env struct {
	Pos   lexer.Position
	Key   string `@String`
	Value string `@String`
}

type Dir struct {
	Pos  lexer.Position
	Path string `@String`
}

type User struct {
	Pos  lexer.Position
	Name string `@String`
}

type Chown struct {
	Pos lexer.Position

	// TODO: Use regex lexer to define custom types like user:group?
	Owner string `@String`
}

type Time struct {
	Pos lexer.Position

	// TODO: Use regex lexer to define custom types like time.Time?
	Value string `@String`
}

type Mkdir struct {
	Pos    lexer.Position
	Path   string       `@String`
	Mode   *FileMode    `@@`
	Option *MkdirOption `( "with" @@ )?`
}

type MkdirOption struct {
	Pos         lexer.Position
	MkdirFields []*MkdirField `( "option" "{" ( @@ [ ";" ] )*`
	BlockEnd    BlockEnd      `@@`
	Name        *string       `| @Ident )`
}

type MkdirField struct {
	Pos           lexer.Position
	CreateParents *bool  `( @"createParents"`
	Chown         *Chown `| "chown" @@`
	CreatedTime   *Time  `| "createdTime" @@ )`
}

type Mkfile struct {
	Pos     lexer.Position
	Path    string        `@String`
	Mode    *FileMode     `@@`
	Content string        `@String`
	Option  *MkfileOption `( "with" @@ )?`
}

type MkfileOption struct {
	Pos          lexer.Position
	MkfileFields []*MkfileField `( "option" "{" ( @@ [ ";" ] )*`
	BlockEnd     BlockEnd       `@@`
	Name         *string        `| @Ident )`
}

type MkfileField struct {
	Pos         lexer.Position
	Chown       *Chown `( "chown" @@`
	CreatedTime *Time  `| "createdTime" @@ )`
}

type FileMode struct {
	Pos   lexer.Position
	Value uint32 `@Int`
}

type Rm struct {
	Pos    lexer.Position
	Path   string    `@String`
	Option *RmOption `( "with" @@ )?`
}

type RmOption struct {
	Pos      lexer.Position
	RmFields []*RmField `( "option" "{" ( @@ [ ";" ] )*`
	BlockEnd BlockEnd   `@@`
	Name     *string    `| @Ident )`
}

type RmField struct {
	Pos           lexer.Position
	AllowNotFound *bool `( @"allowNotFound"`
	AllowWildcard *bool `| @"allowWildcard" )`
}

type CopyState struct {
	Pos   lexer.Position
	Input *State `@@`
	CopyBase
}

type Copy struct {
	Pos   lexer.Position
	Input string `@Ident`
	CopyBase
}

type CopyBase struct {
	Src    string      `@String`
	Dst    string      `@String`
	Option *CopyOption `( "with" @@ )?`
}

type CopyOption struct {
	Pos        lexer.Position
	CopyFields []*CopyField `( "option" "{" ( @@ [ ";" ] )*`
	BlockEnd   BlockEnd     `@@`
	Name       *string      `| @Ident )`
}

type CopyField struct {
	Pos                lexer.Position
	FollowSymlinks     *bool  `( @"followSymlinks"`
	ContentsOnly       *bool  `| @"contentsOnly"`
	Unpack             *bool  `| @"unpack"`
	CreateDestPath     *bool  `| @"createDestPath"`
	AllowWildcard      *bool  `| @"allowWildcard"`
	AllowEmptyWildcard *bool  `| @"allowEmptyWildcard"`
	Chown              *Chown `| "chown" @@`
	CreatedTime        *Time  `| "createdTime" @@ )`
}

type ResultEntry struct {
	Pos lexer.Position
}

type OptionEntry struct {
	Pos lexer.Position
}

type BlockEnd struct {
	Pos   lexer.Position
	Brace string `@"}"`
}
