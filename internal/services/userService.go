package services

import (
	"fmt"
	"p2pbot/internal/db/models"
	"p2pbot/internal/db/repository"
)

type UserService struct {
	repo *repository.UserRepository
}

func NewUserService(repo *repository.UserRepository) *UserService {
	return &UserService{repo}
}

func (s *UserService) CreateUser(user *models.User) (int, error) {
	return s.repo.Save(user)
}

func (s *UserService) GetUserByChatID(id int64) (*models.User, error) {
	return s.repo.GetByChatID(id)
}

func (s *UserService) GetUserByID(id int) (*models.User, error) {
	return s.repo.GetByID(id)
}

func (s *UserService) GetUserByEmail(email string) (*models.User, error) {
	return s.repo.GetByEmail(email)
}

// GetUser retreives a user by email or chat_id
// Use email == "" to search by chat_id
// Use chat_id == -1 to search by email
// If both email and chat_id are provided, error is returned
func (s *UserService) GetUser(email string, chat_id int64) (*models.User, error) {
	if email != "" {
		return s.GetUserByEmail(email)
	}
	if chat_id != -1 {
		return s.GetUserByChatID(chat_id)
	}

	return nil, fmt.Errorf("email xor chat_id must be provided")
}
