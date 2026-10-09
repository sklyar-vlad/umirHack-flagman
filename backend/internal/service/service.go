package service

import "go.uber.org/zap"

// Repository will define the persistence operations used by the service.
type Repository interface{}

type service struct {
	repo   Repository
	logger *zap.Logger
}

func NewService(repo Repository, logger *zap.Logger) *service {
	return &service{repo: repo, logger: logger}
}
