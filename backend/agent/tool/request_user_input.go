package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const RequestUserInputToolName = "request_user_input"

type RequestUserInputTool struct{}

type requestUserInputArgs struct {
	Question string   `json:"question"`
	Fields   []string `json:"fields,omitempty"`
	Answer   string   `json:"answer,omitempty"`
}

func (t *RequestUserInputTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: RequestUserInputToolName,
		Desc: "Request missing parameters from the user. First call without answer to pause for input; after user replies, call again with answer populated.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"question": {
				Type:     schema.String,
				Desc:     "The exact question shown to user to collect missing information.",
				Required: true,
			},
			"fields": {
				Type: schema.Array,
				Desc: "Optional list of missing field names.",
				ElemInfo: &schema.ParameterInfo{
					Type: schema.String,
				},
			},
			"answer": {
				Type: schema.String,
				Desc: "User-provided answer after resume.",
			},
		}),
	}, nil
}

func (t *RequestUserInputTool) InvokableRun(ctx context.Context, args string, opts ...einotool.Option) (string, error) {
	var input requestUserInputArgs
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		return "invalid request_user_input arguments: " + err.Error(), nil
	}
	question := strings.TrimSpace(input.Question)
	answer := strings.TrimSpace(input.Answer)
	if answer == "" {
		if question == "" {
			return "missing question for request_user_input", nil
		}
		return fmt.Sprintf("awaiting user input: %s", question), nil
	}
	return answer, nil
}
