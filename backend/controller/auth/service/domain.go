package service

type EmailUtil struct {
	SMTPHost string
	SMTPPort int
	Email    string
	Password string
}

type User struct {
	ID     uint
	Name   string
	Email  string
	Avatar string
}
