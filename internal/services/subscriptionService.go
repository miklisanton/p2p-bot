package services

import (
	"fmt"
	"p2pbot/internal/db/models"
	"p2pbot/internal/db/repository"
	"time"
)

type SubscriptionService struct {
	repo *repository.SubscriptionRepository
}

func NewSubscriptionService(repo *repository.SubscriptionRepository) *SubscriptionService {
	return &SubscriptionService{
		repo: repo,
	}
}

func (s *SubscriptionService) Create(subscription *models.Subscription) error {
	if subscription.Id != 0 {
		return fmt.Errorf("subscription already exists")
	}
	return s.repo.Save(subscription)
}

func (s *SubscriptionService) AddMonth(subscription *models.Subscription) error {
	if subscription.Id == 0 {
		return fmt.Errorf("subscription does not exist")
	}
	if subscription.ValidUntil.After(time.Now()) {
		subscription.ValidUntil = subscription.ValidUntil.AddDate(0, 1, 0)
	} else {
		subscription.ValidUntil = time.Now().AddDate(0, 1, 0)
	}

	return s.repo.Save(subscription)
}

func (s *SubscriptionService) GetByID(id int) (*models.Subscription, error) {
	return s.repo.GetByID(id)
}

func (s *SubscriptionService) GetByUserId(id int) (*models.Subscription, error) {
	return s.repo.GetByUserID(id)
}
