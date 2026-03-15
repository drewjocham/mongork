package observability

import (
	"context"
	"fmt"
	"time"

	"github.com/dustin/go-humanize"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type HealthReport struct {
	Database    string            `json:"database"`
	Role        string            `json:"role"`
	OplogWindow string            `json:"oplog_window"`
	OplogSize   string            `json:"oplog_size"`
	Connections string            `json:"connections"`
	Lag         map[string]string `json:"lag,omitempty"`
	Warnings    []string          `json:"warnings,omitempty"`
}

type CollectionInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type IndexInfo struct {
	Collection string `json:"collection"`
	Name       string `json:"name"`
	Keys       string `json:"keys"`
	Unique     bool   `json:"unique"`
}

type CollectionStats struct {
	Collection     string  `json:"collection"`
	Count          int64   `json:"count"`
	SizeBytes      int64   `json:"size_bytes"`
	StorageBytes   int64   `json:"storage_bytes"`
	IndexBytes     int64   `json:"index_bytes"`
	AvgObjectBytes float64 `json:"avg_object_bytes"`
}

type CurrentOpInfo struct {
	OpID        string `json:"op_id"`
	Operation   string `json:"operation"`
	Namespace   string `json:"namespace"`
	Client      string `json:"client"`
	RunningSecs int64  `json:"running_seconds"`
	Description string `json:"description"`
}

type UserInfo struct {
	User  string   `json:"user"`
	DB    string   `json:"db"`
	Roles []string `json:"roles"`
}

type ResourceSummary struct {
	ConnectionsCurrent   int64              `json:"connections_current"`
	ConnectionsAvailable int64              `json:"connections_available"`
	ResidentMemoryMB     float64            `json:"resident_memory_mb"`
	VirtualMemoryMB      float64            `json:"virtual_memory_mb"`
	Opcounters           map[string]float64 `json:"opcounters"`
}

func BuildHealthReport(ctx context.Context, client *mongo.Client, dbName string) (HealthReport, error) {
	stats, err := serverStatus(ctx, client)
	if err != nil {
		return HealthReport{}, err
	}
	report := HealthReport{
		Database: dbName,
		Lag:      make(map[string]string),
	}
	if repl, ok := asMap(stats["repl"]); ok {
		if setName, _ := repl["setName"].(string); setName != "" {
			report.Role = fmt.Sprintf("REPLICA (%s)", setName)
		}
	}
	if conn, ok := asMap(stats["connections"]); ok {
		report.Connections = fmt.Sprintf("%v / %v", conn["current"], conn["available"])
	}
	if oplog, ok := asMap(stats["oplog"]); ok {
		if window := number(oplog["windowSeconds"]); window > 0 {
			report.OplogWindow = (time.Duration(window) * time.Second).String()
			if window < 21600 {
				report.Warnings = append(report.Warnings, "Oplog window is under 6 hours")
			}
		}
		if sizeMB := number(oplog["logSizeMB"]); sizeMB > 0 {
			report.OplogSize = humanize.Bytes(uint64(sizeMB) * 1024 * 1024)
		}
	}
	return report, nil
}

func BuildResourceSummary(ctx context.Context, client *mongo.Client) (ResourceSummary, error) {
	stats, err := serverStatus(ctx, client)
	if err != nil {
		return ResourceSummary{}, err
	}
	s := ResourceSummary{Opcounters: map[string]float64{}}
	if conn, ok := asMap(stats["connections"]); ok {
		s.ConnectionsCurrent = int64(number(conn["current"]))
		s.ConnectionsAvailable = int64(number(conn["available"]))
	}
	if mem, ok := asMap(stats["mem"]); ok {
		s.ResidentMemoryMB = number(mem["resident"])
		s.VirtualMemoryMB = number(mem["virtual"])
	}
	if ops, ok := asMap(stats["opcounters"]); ok {
		for k, v := range ops {
			s.Opcounters[k] = number(v)
		}
	}
	return s, nil
}

func ListCollections(ctx context.Context, db *mongo.Database) ([]CollectionInfo, error) {
	c, err := db.ListCollections(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	defer c.Close(ctx)
	var out []CollectionInfo
	for c.Next(ctx) {
		var row bson.M
		if err := c.Decode(&row); err != nil {
			return nil, err
		}
		out = append(out, CollectionInfo{
			Name: fmt.Sprintf("%v", row["name"]),
			Type: fmt.Sprintf("%v", row["type"]),
		})
	}
	return out, c.Err()
}

func ListIndexes(ctx context.Context, db *mongo.Database, collection string) ([]IndexInfo, error) {
	names, err := db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	var out []IndexInfo
	for _, name := range names {
		if collection != "" && collection != name {
			continue
		}
		cur, err := db.Collection(name).Indexes().List(ctx)
		if err != nil {
			return nil, err
		}
		var idxs []bson.M
		if err := cur.All(ctx, &idxs); err != nil {
			_ = cur.Close(ctx)
			return nil, err
		}
		_ = cur.Close(ctx)
		for _, idx := range idxs {
			out = append(out, IndexInfo{
				Collection: name,
				Name:       fmt.Sprintf("%v", idx["name"]),
				Keys:       formatIndexKeys(idx["key"]),
				Unique:     asBool(idx["unique"]),
			})
		}
	}
	return out, nil
}

func CollectionStatistics(ctx context.Context, db *mongo.Database, collection string) ([]CollectionStats, error) {
	names, err := db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	var out []CollectionStats
	for _, name := range names {
		if collection != "" && collection != name {
			continue
		}
		var res bson.M
		if err := db.RunCommand(ctx, bson.D{{Key: "collStats", Value: name}}).Decode(&res); err != nil {
			return nil, err
		}
		out = append(out, CollectionStats{
			Collection:     name,
			Count:          int64(number(res["count"])),
			SizeBytes:      int64(number(res["size"])),
			StorageBytes:   int64(number(res["storageSize"])),
			IndexBytes:     int64(number(res["totalIndexSize"])),
			AvgObjectBytes: number(res["avgObjSize"]),
		})
	}
	return out, nil
}

func CurrentOperations(ctx context.Context, client *mongo.Client, limit int) ([]CurrentOpInfo, error) {
	if limit <= 0 {
		limit = 20
	}
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$currentOp", Value: bson.M{"allUsers": true, "idleConnections": false}}},
		bson.D{{Key: "$limit", Value: limit}},
	}
	cur, err := client.Database("admin").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var rows []bson.M
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	out := make([]CurrentOpInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, CurrentOpInfo{
			OpID:        fmt.Sprintf("%v", row["opid"]),
			Operation:   fmt.Sprintf("%v", row["op"]),
			Namespace:   fmt.Sprintf("%v", row["ns"]),
			Client:      fmt.Sprintf("%v", row["client"]),
			RunningSecs: int64(number(row["secs_running"])),
			Description: fmt.Sprintf("%v", row["desc"]),
		})
	}
	return out, nil
}

func Users(ctx context.Context, client *mongo.Client) ([]UserInfo, error) {
	var res bson.M
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "usersInfo", Value: 1}}).Decode(&res); err != nil {
		return nil, err
	}
	usersRaw, _ := res["users"].(bson.A)
	out := make([]UserInfo, 0, len(usersRaw))
	for _, u := range usersRaw {
		row, ok := asMap(u)
		if !ok {
			continue
		}
		roles := []string{}
		if roleArr, ok := row["roles"].(bson.A); ok {
			for _, r := range roleArr {
				rMap, ok := asMap(r)
				if !ok {
					continue
				}
				role := fmt.Sprintf("%v@%v", rMap["role"], rMap["db"])
				roles = append(roles, role)
			}
		}
		out = append(out, UserInfo{
			User:  fmt.Sprintf("%v", row["user"]),
			DB:    fmt.Sprintf("%v", row["db"]),
			Roles: roles,
		})
	}
	return out, nil
}

func serverStatus(ctx context.Context, client *mongo.Client) (bson.M, error) {
	var stats bson.M
	if err := client.
		Database("admin").
		RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).
		Decode(&stats); err != nil {
		return nil, err
	}
	return stats, nil
}

func asMap(v interface{}) (bson.M, bool) {
	switch t := v.(type) {
	case bson.M:
		return t, true
	case bson.D:
		m := bson.M{}
		for _, e := range t {
			m[e.Key] = e.Value
		}
		return m, true
	default:
		return nil, false
	}
}

func asBool(v interface{}) bool {
	b, ok := v.(bool)
	return ok && b
}

func number(v interface{}) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case uint32:
		return float64(n)
	case uint64:
		return float64(n)
	case float32:
		return float64(n)
	case float64:
		return n
	default:
		return 0
	}
}

func formatIndexKeys(keys any) string {
	m, ok := asMap(keys)
	if !ok || len(m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s:%v", k, v))
	}
	return fmt.Sprintf("%s", parts)
}
