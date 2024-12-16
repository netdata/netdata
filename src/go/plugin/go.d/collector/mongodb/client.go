// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	mongos = "mongos"
)

type mongoConn interface {
	serverStatus() (*documentServerStatus, error)
	listDatabaseNames() ([]string, error)
	dbStats(name string) (*documentDBStats, error)
	isReplicaSet() bool
	isMongos() bool
	replSetGetStatus() (*documentReplSetStatus, error)
	shardNodes() (*documentShardNodesResult, error)
	shardDatabasesPartitioning() (*documentPartitionedResult, error)
	shardCollectionsPartitioning() (*documentPartitionedResult, error)
	shardChunks() (map[string]int64, error)
	initClient(uri string, timeout time.Duration) error
	close() error
}

type mongoClient struct {
	client         *mongo.Client
	timeout        time.Duration
	replicaSetFlag *bool
	mongosFlag     *bool
}

func (c *mongoClient) serverStatus() (*documentServerStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	cmd := bson.D{
		{Key: "serverStatus", Value: 1},
		{Key: "repl", Value: 1},
		{Key: "metrics",
			Value: bson.D{
				{Key: "document", Value: true},
				{Key: "cursor", Value: true},
				{Key: "queryExecutor", Value: true},
				{Key: "apiVersions", Value: false},
				{Key: "aggStageCounters", Value: false},
				{Key: "commands", Value: false},
				{Key: "dotsAndDollarsFields", Value: false},
				{Key: "getLastError", Value: false},
				{Key: "mongos", Value: false},
				{Key: "operation", Value: false},
				{Key: "operatorCounters", Value: false},
				{Key: "query", Value: false},
				{Key: "record", Value: false},
				{Key: "repl", Value: false},
				{Key: "storage", Value: false},
				{Key: "ttl", Value: false},
			},
		},
	}
	var status *documentServerStatus

	err := c.client.Database("admin").RunCommand(ctx, cmd).Decode(&status)
	if err != nil {
		return nil, err
	}

	isReplSet := status.Repl != nil
	c.replicaSetFlag = &isReplSet

	isMongos := status.Process == mongos
	c.mongosFlag = &isMongos

	return status, err
}

func (c *mongoClient) listDatabaseNames() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	return c.client.ListDatabaseNames(ctx, bson.M{})
}

func (c *mongoClient) dbStats(name string) (*documentDBStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	cmd := bson.M{"dbStats": 1}
	var stats documentDBStats

	if err := c.client.Database(name).RunCommand(ctx, cmd).Decode(&stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (c *mongoClient) isReplicaSet() bool {
	if c.replicaSetFlag != nil {
		return *c.replicaSetFlag
	}

	status, err := c.serverStatus()
	if err != nil {
		return false
	}

	return status.Repl != nil
}

func (c *mongoClient) isMongos() bool {
	if c.mongosFlag != nil {
		return *c.mongosFlag
	}

	status, err := c.serverStatus()
	if err != nil {
		return false
	}

	return status.Process == mongos
}

func (c *mongoClient) replSetGetStatus() (*documentReplSetStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	var status *documentReplSetStatus
	cmd := bson.M{"replSetGetStatus": 1}

	err := c.client.Database("admin").RunCommand(ctx, cmd).Decode(&status)
	if err != nil {
		return nil, err
	}

	return status, err
}

func (c *mongoClient) shardNodes() (*documentShardNodesResult, error) {
	collection := "shards"
	groupStage := bson.D{{Key: "$sortByCount", Value: "$state"}}

	nodesByState, err := c.shardCollectAggregation(collection, []bson.D{groupStage})
	if err != nil {
		return nil, err
	}

	return &documentShardNodesResult{nodesByState.True, nodesByState.False}, nil
}

func (c *mongoClient) shardDatabasesPartitioning() (*documentPartitionedResult, error) {
	collection := "databases"
	groupStage := bson.D{{Key: "$sortByCount", Value: "$partitioned"}}

	partitioning, err := c.shardCollectAggregation(collection, []bson.D{groupStage})
	if err != nil {
		return nil, err
	}

	return &documentPartitionedResult{partitioning.True, partitioning.False}, nil
}

func (c *mongoClient) shardCollectionsPartitioning() (*documentPartitionedResult, error) {
	collection := "collections"
	matchStage := bson.D{{Key: "$match", Value: bson.D{{Key: "dropped", Value: false}}}}
	countStage := bson.D{{Key: "$sortByCount", Value: bson.D{{Key: "$eq", Value: bson.A{"$distributionMode", "sharded"}}}}}

	partitioning, err := c.shardCollectAggregation(collection, []bson.D{matchStage, countStage})
	if err != nil {
		return nil, err
	}

	return &documentPartitionedResult{partitioning.True, partitioning.False}, nil
}

func (c *mongoClient) shardCollectAggregation(collection string, aggr []bson.D) (*documentAggrResult, error) {
	rows, err := c.dbAggregate(collection, aggr)
	if err != nil {
		return nil, err
	}

	result := &documentAggrResult{}

	for _, row := range rows {
		if row.Bool {
			result.True = row.Count
		} else {
			result.False = row.Count
		}
	}

	return result, err
}

func (c *mongoClient) shardChunks() (map[string]int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	col := c.client.Database("config").Collection("chunks")

	cursor, err := col.Aggregate(ctx, mongo.Pipeline{bson.D{{Key: "$sortByCount", Value: "$shard"}}})
	if err != nil {
		return nil, err
	}

	var shards []bson.M
	if err = cursor.All(ctx, &shards); err != nil {
		return nil, err
	}

	defer func() { _ = cursor.Close(ctx) }()

	result := map[string]int64{}

	for _, row := range shards {
		k, ok := row["_id"].(string)
		if !ok {
			return nil, fmt.Errorf("shard name is not a string: %v", row["_id"])
		}
		v, ok := row["count"].(int32)
		if !ok {
			return nil, fmt.Errorf("shard chunk count is not a int32: %v", row["count"])
		}
		result[k] = int64(v)
	}

	return result, err
}

func (c *mongoClient) initClient(uri string, timeout time.Duration) error {
	if c.client != nil {
		return nil
	}

	c.timeout = timeout

	ctxConn, cancelConn := context.WithTimeout(context.Background(), c.timeout)
	defer cancelConn()

	client, err := mongo.Connect(ctxConn, options.Client().ApplyURI(uri))
	if err != nil {
		return err
	}

	ctxPing, cancelPing := context.WithTimeout(context.Background(), c.timeout)
	defer cancelPing()

	if err := client.Ping(ctxPing, nil); err != nil {
		_ = client.Disconnect(ctxConn)
		return err
	}

	c.client = client

	return nil
}

func (c *mongoClient) close() error {
	if c.client == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	if err := c.client.Disconnect(ctx); err != nil {
		return err
	}

	c.client = nil

	return nil
}

func (c *mongoClient) dbAggregate(collection string, aggr []bson.D) ([]documentAggrResults, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	cursor, err := c.client.Database("config").Collection(collection).Aggregate(ctx, aggr)
	if err != nil {
		return nil, err
	}

	defer func() { _ = cursor.Close(ctx) }()

	var rows []documentAggrResults
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}

	return rows, nil
}
