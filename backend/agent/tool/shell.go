package tool

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strings"

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
		cmd = exec.Command("cmd", "/c", input.Cmd)
	} else {
		cmd = exec.Command("bash", "-c", input.Cmd)
	}
	cmd.Env = shellEnv(ctx)

	out, err := cmd.CombinedOutput()
	result := redactToken(ctx, string(out))
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
