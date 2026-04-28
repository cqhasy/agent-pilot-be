package ioc

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func InitMongoDatabase(uri string, database string) *mongo.Database {
	opts := options.Client().ApplyURI(uri)
	c, err := mongo.Connect(context.Background(), opts)
	if err != nil {
		panic(err)
	}
	return c.Database(database)
}
