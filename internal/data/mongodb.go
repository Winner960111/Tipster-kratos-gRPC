package data

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/go-kratos/kratos/v2/log"
)

// MongoDB holds the connection
type MongoDB struct {
	Client   *mongo.Client
	Database *mongo.Database
}

// NewMongoDB initializes a MongoDB connection
func NewMongoDB(logger log.Logger) (*MongoDB, func(), error) {
	// Load config (assume you have a way to load configs in Kratos)
	uri := "mongodb+srv://BrandonBaker:Atlas12345@cluster0.dgujh.mongodb.net/" // Ideally, fetch from config.yaml
	dbName := "Tipster"

	// Set connection options
	clientOptions := options.Client().ApplyURI(uri)

	// Connect to MongoDB
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Ping to check the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx, nil); err != nil {
		return nil, nil, fmt.Errorf("MongoDB ping failed: %w", err)
	}

	log.NewHelper(logger).Info("Connected to MongoDB")

	// Define cleanup function
	cleanup := func() {
		if err := client.Disconnect(context.TODO()); err != nil {
			log.NewHelper(logger).Error("Failed to disconnect MongoDB", err)
		} else {
			log.NewHelper(logger).Info("Disconnected from MongoDB")
		}
	}

	return &MongoDB{
		Client:   client,
		Database: client.Database(dbName),
	}, cleanup, nil
}
