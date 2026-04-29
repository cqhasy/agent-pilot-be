package service

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"math/big"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/agent-pilot/agent-pilot-be/repository/mysql/dao"
	mysqlmodel "github.com/agent-pilot/agent-pilot-be/repository/mysql/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	defaultAvatar = "http://lib.cqhasy.top/776D516E0FC97A4BE25EA49D2233DEAA.jpg"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrPassword           = errors.New("password error")
	ErrConfirmPassword    = errors.New("confirm password error")
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrCodeErrors         = errors.New("verification code error")
	ErrTimeOut            = errors.New("verification code timeout")
)

type UserServiceInterface interface {
	Login(ctx context.Context, email string, password string) (User, error)
	Register(ctx context.Context, name, email, password, confirmPassword, code string) (User, error)
	SendEmail(ctx context.Context, email string) error
}

type verificationCode struct {
	Code      string
	ExpiresAt time.Time
}

type UserService struct {
	Smtp  *EmailUtil
	dao   dao.UserDaoInterface
	codes map[string]verificationCode
	mu    sync.RWMutex
}

func NewUserService(smtp *EmailUtil, dao dao.UserDaoInterface) *UserService {
	return &UserService{
		Smtp:  smtp,
		dao:   dao,
		codes: make(map[string]verificationCode),
	}
}

func (s *UserService) Login(ctx context.Context, email string, password string) (User, error) {
	user, err := s.dao.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return User{}, ErrUserNotFound
		}
		return User{}, err
	}

	if !checkPassword(user.Password, password) {
		return User{}, ErrPassword
	}

	return toUser(user), nil
}

func (s *UserService) Register(ctx context.Context, name, email, password, confirmPassword, code string) (User, error) {
	if password != confirmPassword {
		return User{}, ErrConfirmPassword
	}

	if _, err := s.dao.GetUserByEmail(ctx, email); err == nil {
		return User{}, ErrEmailAlreadyExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, err
	}

	if !s.isCodeVerified(email, code) {
		return User{}, ErrCodeErrors
	}
	if s.isExpired(email) {
		return User{}, ErrTimeOut
	}

	hashedPassword, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}

	if strings.TrimSpace(name) == "" {
		name = defaultName(email)
	}

	user, err := s.dao.CreateUser(ctx, mysqlmodel.User{
		Name:     name,
		Email:    email,
		Password: hashedPassword,
		Avatar:   defaultAvatar,
	})
	if err != nil {
		return User{}, err
	}

	s.clearVerificationCode(email)
	return toUser(user), nil
}

func (s *UserService) SendEmail(ctx context.Context, email string) error {
	code, err := generateVerificationCode()
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.codes[email] = verificationCode{
		Code:      code,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
	s.mu.Unlock()

	return s.Smtp.Send(email, code)
}

func (s *UserService) isCodeVerified(email, code string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stored, ok := s.codes[email]
	return ok && stored.Code == code
}

func (s *UserService) isExpired(email string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stored, ok := s.codes[email]
	return !ok || time.Now().After(stored.ExpiresAt)
}

func (s *UserService) clearVerificationCode(email string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.codes, email)
}

func checkPassword(hashedPassword, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)) == nil
}

func hashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func generateVerificationCode() (string, error) {
	var builder strings.Builder
	for i := 0; i < 6; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		builder.WriteByte(byte('0' + n.Int64()))
	}
	return builder.String(), nil
}

func defaultName(email string) string {
	name, _, ok := strings.Cut(email, "@")
	if !ok || strings.TrimSpace(name) == "" {
		return email
	}
	return name
}

func toUser(user mysqlmodel.User) User {
	return User{
		ID:     uint(user.ID),
		Name:   user.Name,
		Email:  user.Email,
		Avatar: user.Avatar,
	}
}

func (e *EmailUtil) Send(to string, code string) error {
	if e == nil {
		return errors.New("smtp config is nil")
	}
	if e.Email == "" || e.Password == "" || e.SMTPHost == "" {
		return errors.New("smtp config is incomplete")
	}

	auth := smtp.PlainAuth("", e.Email, e.Password, e.SMTPHost)
	conn, err := tls.Dial("tcp", e.addr(), &tls.Config{
		ServerName: e.SMTPHost,
		MinVersion: tls.VersionTLS12,
	})
	if err != nil {
		return fmt.Errorf("tls connect failed: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, e.SMTPHost)
	if err != nil {
		return fmt.Errorf("smtp client failed: %w", err)
	}
	defer client.Quit()

	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth failed: %w", err)
	}
	if err = client.Mail(e.Email); err != nil {
		return fmt.Errorf("smtp sender failed: %w", err)
	}
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp receiver failed: %w", err)
	}

	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data failed: %w", err)
	}
	defer wc.Close()

	body := `
		<h1>Verification Code</h1>
		<p>Your verification code is: <strong>` + code + `</strong></p>
		<p>This verification code is valid for 15 minutes.</p>
		<p>If you are not doing it yourself, please ignore it.</p>
	`
	msg := []byte("From: Agent Pilot <" + e.Email + ">\r\n" +
		"To: " + to + "\r\n" +
		"Subject: Agent Pilot Verification Code\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
		"\r\n" +
		body)

	if _, err = wc.Write(msg); err != nil {
		return fmt.Errorf("smtp write failed: %w", err)
	}

	return nil
}

func (e *EmailUtil) addr() string {
	if e.SMTPPort == 0 {
		return e.SMTPHost
	}
	return fmt.Sprintf("%s:%d", e.SMTPHost, e.SMTPPort)
}
