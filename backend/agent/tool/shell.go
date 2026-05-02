package tool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf16"

	"github.com/agent-pilot/agent-pilot-be/pkg/larkctx"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type ShellTool struct{}

func (t *ShellTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "shell",
		Desc: "Execute shell command. When the request has a Lark user access token in context, lark-cli commands run with that token as user identity.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"cmd": {
				Type:     schema.String,
				Required: true,
			},
		}),
	}, nil
}

func (t *ShellTool) InvokableRun(
	ctx context.Context,
	args string,
	opts ...tool.Option,
) (string, error) {

	var input struct {
		Cmd string `json:"cmd"`
	}

	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "invalid shell arguments: " + err.Error(), nil
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		encoded := encodePowerShellCommand(wrapPowerShellScript(input.Cmd))
		cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encoded)
	} else {
		cmd = exec.Command("bash", "-c", input.Cmd)
	}
	cmd.Env = shellEnv(ctx)

	out, err := cmd.CombinedOutput()
	result := redactToken(ctx, string(out))
	if runtime.GOOS == "windows" {
		result = stripPowerShellCLIXML(result)
	}
	if err != nil {
		return result + "\n" + err.Error(), nil
	}

	return result, nil
}

func shellEnv(ctx context.Context) []string {
	env := os.Environ()
	token, ok := larkctx.UserAccessToken(ctx)
	if !ok {
		return env
	}

	return append(env,
		"USER_ACCESS_TOKEN="+token,
		"LARK_USER_ACCESS_TOKEN="+token,
		"FEISHU_USER_ACCESS_TOKEN="+token,
		"LARK_ACCESS_TOKEN="+token,
	)
}

func redactToken(ctx context.Context, out string) string {
	token, ok := larkctx.UserAccessToken(ctx)
	if !ok || token == "" {
		return out
	}
	return strings.ReplaceAll(out, token, "[REDACTED_USER_ACCESS_TOKEN]")
}

// encodePowerShellCommand encodes script text as UTF-16LE Base64 for -EncodedCommand,
// which preserves newlines and quotes more reliably than cmd /c on Windows.
func encodePowerShellCommand(script string) string {
	if script == "" {
		return ""
	}
	utf16Units := utf16.Encode([]rune(script))
	buf := make([]byte, 0, len(utf16Units)*2)
	for _, unit := range utf16Units {
		buf = append(buf, byte(unit), byte(unit>>8))
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func wrapPowerShellScript(script string) string {
	var b strings.Builder
	b.WriteString("$ProgressPreference='SilentlyContinue'\n")
	b.WriteString("$InformationPreference='SilentlyContinue'\n")
	b.WriteString("[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)\n")
	b.WriteString("$OutputEncoding = [Console]::OutputEncoding\n")
	b.WriteString(script)
	return b.String()
}

var (
	powerShellCLIXMLHeader = regexp.MustCompile(`(?m)^#<\s*CLIXML\r?\n?`)
	powerShellCLIXMLObjs   = regexp.MustCompile(`(?s)<Objs\b[^>]*>.*?</Objs>`)
)

func stripPowerShellCLIXML(out string) string {
	clean := powerShellCLIXMLHeader.ReplaceAllString(out, "")
	clean = powerShellCLIXMLObjs.ReplaceAllString(clean, "")
	return strings.TrimSpace(clean)
}
