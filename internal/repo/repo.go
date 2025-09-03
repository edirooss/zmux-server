package repo

import "go.uber.org/zap"

type Repository struct {
	log    *zap.Logger
	client *RedisClient

	Channels     *ChannelRepository
	Remuxers     *RemuxRepository
	B2BClntChnls *B2BClntChnlsRepo
	Principal    *PrincipalRepository
}

func NewRepository(log *zap.Logger) *Repository {
	log = log.Named("repo")
	client := newRedisClient("localhost:6379", 0, log)

	return &Repository{
		log,
		client,
		newChannelRepository(log, client),
		newRemuxRepository(log, client),
		newB2BClntChnlsRepo(log, client),
		newPrincipalRepository(log, client),
	}
}
