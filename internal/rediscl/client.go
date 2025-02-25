package rediscl

import (
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	Client *redis.Client
	Ctx    context.Context
}

var RDB *RedisClient

func InitRedisClient(host, port, password string) {
	RDB = &RedisClient{
		Ctx: context.Background(),
		Client: redis.NewClient(&redis.Options{
			Addr:     fmt.Sprintf("%s:%s", host, port),
			Password: password,
			DB:       0,
		}),
	}
}
