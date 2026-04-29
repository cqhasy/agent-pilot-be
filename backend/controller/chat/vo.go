package chat

type request struct {
	Message      string `json:"message"`
	CheckpointID string `json:"checkpoint_id"`
}
