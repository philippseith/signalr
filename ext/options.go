package ext

import (
	"github.com/philippseith/signalr"
	"github.com/redis/go-redis/v9"
)

func WithRedisHubLifetimeManager(redisClient *redis.Client, logger signalr.StructuredLogger) func(signalr.Party) error {
	return signalr.WithHubLifetimeManager(func(s signalr.Server) (signalr.HubLifetimeManager, error) {
		return newRedisHubLifetimeManager(s.Context(), logger, redisClient)
	})
}
