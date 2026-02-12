package migration

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	ErrCollectionNameRequired = errors.New("collection name is required")
	ErrCreateCollectionFailed = errors.New("create collection failed")
	ErrListCollectionsFailed  = errors.New("list collections failed")
)

type CollectionOption func(*options.CreateCollectionOptionsBuilder)

func WithValidator(validator interface{}) CollectionOption {
	return func(opts *options.CreateCollectionOptionsBuilder) {
		opts.SetValidator(validator)
	}
}

func WithValidationLevel(level string) CollectionOption {
	return func(opts *options.CreateCollectionOptionsBuilder) {
		opts.SetValidationLevel(level)
	}
}

func WithValidationAction(action string) CollectionOption {
	return func(opts *options.CreateCollectionOptionsBuilder) {
		opts.SetValidationAction(action)
	}
}

func WithCapped(maxBytes int64, maxDocs int64) CollectionOption {
	return func(opts *options.CreateCollectionOptionsBuilder) {
		opts.SetCapped(true)
		if maxBytes > 0 {
			opts.SetSizeInBytes(maxBytes)
		}
		if maxDocs > 0 {
			opts.SetMaxDocuments(maxDocs)
		}
	}
}

func WithTimeSeries(timeField string, metaField string, granularity string) CollectionOption {
	return func(opts *options.CreateCollectionOptionsBuilder) {
		ts := options.TimeSeries()
		ts.SetTimeField(timeField)
		if metaField != "" {
			ts.SetMetaField(metaField)
		}
		if granularity != "" {
			ts.SetGranularity(granularity)
		}
		opts.SetTimeSeriesOptions(ts)
	}
}

func EnsureCollection(ctx context.Context, db *mongo.Database, name string,
	opts ...CollectionOption) (*mongo.Collection, error) {
	if name == "" {
		return nil, ErrCollectionNameRequired
	}

	exists, err := collectionExists(ctx, db, name)
	if err != nil {
		return nil, err
	}

	if !exists {
		createOpts := options.CreateCollection()
		for _, opt := range opts {
			if opt != nil {
				opt(createOpts)
			}
		}
		if err := db.CreateCollection(ctx, name, createOpts); err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrCreateCollectionFailed, name, err)
		}
	}

	return db.Collection(name), nil
}

func collectionExists(ctx context.Context, db *mongo.Database, name string) (bool, error) {
	names, err := db.ListCollectionNames(ctx, bson.M{"name": name})
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrListCollectionsFailed, err)
	}
	return len(names) > 0, nil
}
