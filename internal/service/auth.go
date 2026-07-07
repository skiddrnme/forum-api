package service

import "github.com/google/uuid"

type AuthService struct {
	users map[string]string
}


func NewAuthService() *AuthService {
	return &AuthService{
		users: make(map[string]string),
	}
}

func (s *AuthService) Login(username, password string) (string, error) {
	id, ok := s.users[username]
	if !ok {
		id = uuid.New().String()
		s.users[username] = id
	}
	return id, nil
}

