package auth

type LoginRequest struct {
	UserEmail string `json:"userEmail"`
	Password  string `json:"password"`
}

type RegisterRequest struct {
	UserName        string `json:"username"`
	UserEmail       string `json:"userEmail"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirmPassword"`
	Code            string `json:"code"`
}

type SendEmailRequest struct {
	UserEmail string `json:"userEmail"`
}

type LoginResponse struct {
	Id       uint   `json:"id"`
	UserName string `json:"userName"`
	Avatar   string `json:"avatar"`
	Email    string `json:"email"`
	Token    string `json:"token"`
}
