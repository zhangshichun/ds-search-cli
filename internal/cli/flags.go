package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

// flagSet 包装 flag.FlagSet,解决标准库 flag 的经典限制:
// 位置参数(如 query)后跟的 flag(如 --count 5)会被当成位置参数而失效。
// AI 与人调用时最自然的写法是 `search "查询" --jsonl --count 5`,必须支持。
//
// 实现:Parse 前把「已注册」flag token 重排到位置参数之前;
// bool flag 无独立取值,其余 flag 取下一个 token 作为取值;
// 以 "=" 自带取值的 token 视为自包含;未注册 token 一律视为位置参数,
// 因此 query 中的 "-site:github.com" 等搜索语法不会被误伤。
type flagSet struct {
	fs    *flag.FlagSet
	bools map[string]bool // 已注册的布尔 flag 名(无值形态)
	names map[string]bool // 已注册的全部 flag 名
}

// newFlagSet 创建包装器;错误静默输出,由调用方统一归类。
func newFlagSet(name string) *flagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return &flagSet{fs: fs, bools: map[string]bool{}, names: map[string]bool{}}
}

// register 记录 flag 名并登记是否为无值(布尔)flag。
func (g *flagSet) register(name string, isBool bool) {
	g.names[name] = true
	if isBool {
		g.bools[name] = true
	}
}

// Bool 注册布尔 flag(允许 --flag 与 --flag=value 两种写法)。
func (g *flagSet) Bool(name string, def bool, usage string) *bool {
	g.register(name, true)
	return g.fs.Bool(name, def, usage)
}

// Int 注册整数 flag。
func (g *flagSet) Int(name string, def int, usage string) *int {
	g.register(name, false)
	return g.fs.Int(name, def, usage)
}

// String 注册字符串 flag。
func (g *flagSet) String(name string, def string, usage string) *string {
	g.register(name, false)
	return g.fs.String(name, def, usage)
}

// DurationVar 注册 time.Duration flag(如 --timeout 45s)。
func (g *flagSet) DurationVar(p *time.Duration, name string, def time.Duration, usage string) {
	g.register(name, false)
	g.fs.DurationVar(p, name, def, usage)
}

// IntVar 注册整数 flag,绑定到已有变量。
func (g *flagSet) IntVar(p *int, name string, def int, usage string) {
	g.register(name, false)
	g.fs.IntVar(p, name, def, usage)
}

// StringVar 注册字符串 flag,绑定到已有变量。
func (g *flagSet) StringVar(p *string, name string, def string, usage string) {
	g.register(name, false)
	g.fs.StringVar(p, name, def, usage)
}

// NArg 返回位置参数个数。
func (g *flagSet) NArg() int { return g.fs.NArg() }

// Arg 返回第 i 个位置参数。
func (g *flagSet) Arg(i int) string { return g.fs.Arg(i) }

// Args 返回位置参数。
func (g *flagSet) Args() []string { return g.fs.Args() }

// Name 返回子命令名。
func (g *flagSet) Name() string { return g.fs.Name() }

// Parse 重排后解析;-h/--help 未注册时 flag 包返回 ErrHelp,
// 调用方应把 usage 输出到 stdout 并以 0 退出。
func (g *flagSet) Parse(args []string) error {
	return g.fs.Parse(reorder(g.names, g.bools, args))
}

// parseOrHelp 各子命令共用的解析协议:
// --help → helpText 输出 stdout,返回 (ExitOK, false);
// 其他解析错误 → writeUsageErr,返回 (ExitUsage, false);成功返回 (0, true)。
func parseOrHelp(fs *flagSet, stdout, stderr io.Writer, args []string, helpText string) (int, bool) {
	err := fs.Parse(args)
	switch {
	case errors.Is(err, flag.ErrHelp):
		fmt.Fprint(stdout, helpText)
		return ExitOK, false
	case err != nil:
		return writeUsageErr(stderr, fs, err), false
	}
	return 0, true
}

// isFlagToken 判断 token 是否为 flag 写法。单独 "-" 不算
// (query 可能以 "-site:" 等语法开头);单独 "--" 由 reorder 作为终止符处理。
func isFlagToken(tok string) bool {
	return len(tok) > 1 && tok[0] == '-' && tok != "--"
}

// splitToken 归一 flag 名;"--name=value" 的取值随 token 走(返回 selfValued)。
func splitToken(tok string) (name string, selfValued bool) {
	name = strings.TrimPrefix(tok, "-")
	name = strings.TrimPrefix(name, "-")
	if i := strings.IndexByte(name, '='); i >= 0 {
		return name[:i], true
	}
	return name, false
}

// reorder 把已注册 flag token(含取值)搬到位置参数之前,保持相对顺序。
// "--" 及其后所有 token 一律视为位置参数。
func reorder(names, bools map[string]bool, args []string) []string {
	var flagArgs, positional []string
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if tok == "--" { // 终止符:其后全部为位置参数
			positional = append(positional, args[i+1:]...)
			break
		}
		if !isFlagToken(tok) {
			positional = append(positional, tok)
			continue
		}
		name, selfValued := splitToken(tok)
		if !names[name] {
			positional = append(positional, tok) // 未注册(如搜索语法 -site:x)→ 位置参数
			continue
		}
		if selfValued || bools[name] {
			flagArgs = append(flagArgs, tok)
			continue
		}
		if i+1 < len(args) && !isFlagToken(args[i+1]) && args[i+1] != "--" {
			flagArgs = append(flagArgs, tok, args[i+1]) // 带取值
			i++
			continue
		}
		flagArgs = append(flagArgs, tok) // 缺取值:交给 flag 包报错
	}
	return append(flagArgs, positional...)
}
