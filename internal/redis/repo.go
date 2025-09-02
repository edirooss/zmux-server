package redis

import "go.uber.org/zap"

type Repository struct {
	log    *zap.Logger
	client *Client

	Channels *ChannelRepository
	Remuxers *RemuxRepository
}

func NewRepository(log *zap.Logger) *Repository {
	log = log.Named("repo")
	client := NewClient("localhost:6379", 0, log)

	return &Repository{
		log,
		client,
		newChannelRepository(log, client),
		newRemuxRepository(log, client),
	}
}
