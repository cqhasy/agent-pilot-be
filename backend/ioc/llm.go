package ioc

import (
	"context"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"

	"github.com/agent-pilot/agent-pilot-be/pkg/llm"
)

// NewOpenAIModelClient 创建一个使用eino openai.model的client
func NewOpenAIModelClient(ctx context.Context, model, baseUrl, apikey string) *llm.ChatClient {
	om, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  apikey,
		Model:   model,
		BaseURL: baseUrl,
		Timeout: 360 * time.Second,
	})

	if err != nil {
		panic(err)
	}

	return llm.NewChatClient(om)
}
