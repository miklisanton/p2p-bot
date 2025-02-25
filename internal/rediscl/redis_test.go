package rediscl_test

import (
	//"github.com/redis/go-redis/v9"
	"context"
	"fmt"
	"github.com/jmoiron/sqlx"
	"os"
	"p2pbot/internal/app"
	"p2pbot/internal/rediscl"
	"path/filepath"
	"testing"
)

var (
	DB     *sqlx.DB
	client *rediscl.RedisClient
)

func findProjectRoot() (string, error) {
	// Get the current working directory of the test
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Traverse upwards to find the project root (where .env file is located)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".env")); os.IsNotExist(err) {
			parent := filepath.Dir(dir)
			if parent == dir {
				// Reached the root of the filesystem, and .env wasn't found
				return "", os.ErrNotExist
			}
			dir = parent
		} else {
			return dir, nil
		}
	}
}

func TestMain(m *testing.M) {
	root, err := findProjectRoot()
	fmt.Println("Root: ", root)
	if err != nil {
		panic("Error finding project root: " + err.Error())
	}

	err = os.Chdir(root)
	if err != nil {
		panic(err)
	}

	DB, cfg, err := app.Init()
	if err != nil {
		panic(err)
	}

	DB.Ping()

	rediscl.InitRedisClient(cfg.Redis.Host, cfg.Redis.Port, "")
	if client == nil {
		panic("client is nil")
	}

	code := m.Run()

	os.Exit(code)
}
func TestRedisSet(t *testing.T) {
	ctx := context.Background()
	err := client.Client.Set(ctx, "key", "value", 0).Err()
	if err != nil {
		t.Error(err)
	}

	val, err := client.Client.Get(ctx, "key").Result()
	if err != nil {
		t.Error(err)
	}

	if val != "value" {
		t.Errorf("expected value to be 'value', got %s", val)
	}
}
