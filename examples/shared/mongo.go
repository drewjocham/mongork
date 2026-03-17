package shared

import (
	"context"
	"fmt"
	"time"

	"github.com/drewjocham/mongork/internal/config"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func Connect(ctx context.Context, cfg *config.Config, timeout time.Duration) (*mongo.Client, *mongo.Database, error) {
	connCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.GetConnectionString()))
	if err != nil {
		return nil, nil, err
	}

	if err := client.Ping(connCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, fmt.Errorf("ping mongodb: %w", err)
	}

	return client, client.Database(cfg.Mongo.Database), nil
}

func Disconnect(client *mongo.Client) error {
	if client == nil {
		return nil
	}
	return client.Disconnect(context.Background())
}
