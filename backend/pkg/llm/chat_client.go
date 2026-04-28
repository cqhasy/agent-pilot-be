package llm

import (
	"context"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type ChatClient struct {
	Model model.ToolCallingChatModel
}

func (chatClient *ChatClient) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return chatClient.Model.Generate(ctx, input, opts...)
}

func (chatClient *ChatClient) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return chatClient.Model.Stream(ctx, input, opts...)
}

func NewChatClient(chatModel model.ToolCallingChatModel) *ChatClient {
	return &ChatClient{
		Model: chatModel,
	}
}

func (chatClient *ChatClient) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return chatClient.Model.WithTools(tools)
}
