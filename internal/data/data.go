package data

import (
	"Tipster/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewData, NewMongoDB)

// Data .
type Data struct {
	// TODO wrapped database client
	Mongo *MongoDB
}

// NewData .
func NewData(c *conf.Data, logger log.Logger) (*Data, func(), error) {
	mongoDB, cleanupMongo, err := NewMongoDB(logger)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func() {
		cleanupMongo()
		log.NewHelper(logger).Info("closing the data resources")
	}
	return &Data{Mongo: mongoDB}, cleanup, nil
}
