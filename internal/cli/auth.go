package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// RunAuth `ds-search auth <set|show|clear>` — 管理 API Key。
// stdin 供 `auth set --stdin` 读取 Key(main 传 os.Stdin,测试可传 strings.Reader)。
func RunAuth(stdin io.Reader, stdout, stderr io.Writer, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, authUsageText)
		return ExitUsage
	}
	switch args[0] {
	case "set":
		return authSet(stdin, stdout, stderr, args[1:])
	case "show":
		return authShow(stdout, stderr)
	case "clear":
		return authClear(stdout, stderr)
	default:
		fmt.Fprintf(stderr, "error: unknown auth subcommand %q\n%s\n", args[0], authUsageText)
		return ExitUsage
	}
}

const authUsageText = `usage: ds-search auth <command>

Commands:
  set [--stdin] [key]   保存 API Key 到配置文件(0600)。
                        --stdin: 从 stdin 读取一行,避免出现在 shell history 中。
                        示例:  ds-search auth set --stdin < key.txt
                                 echo sk-xxx | ds-search auth set --stdin
  show                  显示当前生效的 Key 来源与掩码。
  clear                 从配置文件中移除已保存的 Key。

快捷方式(等价 auth set):  ds-search set-token=sk-xxx  或  ds-search set-token sk-xxx
`

// authSet 保存 key:优先 --stdin,否则取第一个非 flag 参数。
func authSet(stdin io.Reader, stdout, stderr io.Writer, args []string) int {
	fs := newFlagSet("auth set")
	stdinFlag := fs.Bool("stdin", false, "从 stdin 读取 Key(防 shell history 泄露)")
	if code, parsed := parseOrHelp(fs, stdout, stderr, args, authUsageText); !parsed {
		return code
	}

	var key string
	if *stdinFlag {
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintf(stderr, "error: reading key from stdin: %v\n", err)
			return ExitUsage
		}
		key = line
	} else if fs.NArg() == 1 {
		key = fs.Arg(0)
	} else {
		return writeUsageErr(stderr, fs, errors.New("auth set expects [key] or --stdin"))
	}
	return saveKey(key, stdout, stderr)
}

// saveKey 校验并写入配置文件(公共出口,auth set 与 set-token 共用)。
// 清洗(BOM、空白)统一由 cleanKeyInput 在此收口,调用方不必预处理。
func saveKey(key string, stdout, stderr io.Writer) int {
	key = cleanKeyInput(key)
	if key == "" {
		fmt.Fprintln(stderr, "error: empty API key")
		return ExitUsage
	}
	cfg, err := LoadConfig()
	if err != nil {
		return writeErr(stderr, err)
	}
	cfg.APIKey = key
	if err := SaveConfig(cfg); err != nil {
		return writeErr(stderr, err)
	}
	path, _ := ConfigPath()
	fmt.Fprintf(stdout, "API key saved to %s (%s)\n", path, MaskKey(key))
	fmt.Fprintln(stdout, "verify with: ds-search auth show")
	return ExitOK
}

// cleanKeyInput 清理 key 输入的常见污染:
// UTF-8 BOM(Windows PowerShell echo 管道常态携带,混入 key 会导致 401)、首尾空白。
// 保存(saveKey)与读取(ResolveKey)两侧共用,清洗规则此处单点演进。
func cleanKeyInput(k string) string {
	k = strings.TrimPrefix(k, string(rune(0xFEFF)))
	return strings.TrimSpace(k)
}

// setTokenUsageText 顶层快捷命令帮助。
const setTokenUsageText = `usage: ds-search set-token <key>          (空格写法)
       ds-search set-token=sk-xxx         (等号写法)

auth set 的单命令快捷方式。
注意:key 作为参数会留在 shell history;更安全: echo sk-xxx | ds-search auth set --stdin
`

// SetTokenUsage 导出 set-token 帮助文本。
func SetTokenUsage() string { return setTokenUsageText }

// RunClearToken `ds-search clear-token` — auth clear 的顶层快捷方式。
func RunClearToken(stdout, stderr io.Writer) int { return authClear(stdout, stderr) }

// RunSetToken `ds-search set-token [=key]` — auth set 的快捷方式。
// 等号一体写法(`set-token=sk-…`/`set-key=sk-…`)与空格写法的解析
// 统一收口在此(main 原样传参,不再自行拆值)。
func RunSetToken(stdout, stderr io.Writer, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, setTokenUsageText)
		return ExitUsage
	}
	key := args[0]
	if strings.HasPrefix(key, "set-token=") || strings.HasPrefix(key, "set-key=") {
		if i := strings.IndexByte(key, '='); i >= 0 {
			key = key[i+1:] // 等号一体写法
		}
	}
	if key == "" {
		fmt.Fprint(stderr, setTokenUsageText)
		return ExitUsage
	}
	return saveKey(key, stdout, stderr)
}

// authShow 显示当前生效 key 的来源与掩码(不落盘完整 key)。
func authShow(stdout, stderr io.Writer) int {
	key, source, err := ResolveKey("")
	if err != nil {
		return writeErr(stderr, err)
	}
	if key == "" {
		fmt.Fprintln(stdout, "no API key configured")
		fmt.Fprintln(stdout, "set with: ds-search set-token=sk-xxx, env DEEPSEEK_API_KEY, or --api-key flag")
		return ExitAuth
	}
	path, _ := ConfigPath()
	fmt.Fprintf(stdout, "key=%s source=%s config_file=%s\n", MaskKey(key), source, path)
	return ExitOK
}

// authClear 移除配置文件中的 key(文件保留其他字段)。
func authClear(stdout, stderr io.Writer) int {
	cfg, err := LoadConfig()
	if err != nil {
		return writeErr(stderr, err)
	}
	if cfg.APIKey == "" {
		fmt.Fprintln(stdout, "no API key in config file; nothing to clear")
		return ExitOK
	}
	cfg.APIKey = ""
	if err := SaveConfig(cfg); err != nil {
		return writeErr(stderr, err)
	}
	fmt.Fprintln(stdout, "API key cleared from config file")
	return ExitOK
}
