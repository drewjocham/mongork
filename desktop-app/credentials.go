package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// SavedConnection holds a named MongoDB connection for caching.
type SavedConnection struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Database string `json:"database"`
	Username string `json:"username"`
	Password string `json:"password"`
	LastUsed string `json:"last_used"`
}

// configDir returns (and creates) the app config directory.
func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "mongork")
	return dir, os.MkdirAll(dir, 0700)
}

func loadConnections() ([]SavedConnection, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "connections.json"))
	if os.IsNotExist(err) {
		return []SavedConnection{}, nil
	}
	if err != nil {
		return nil, err
	}
	var conns []SavedConnection
	if err := json.Unmarshal(data, &conns); err != nil {
		return nil, err
	}
	return conns, nil
}

func persistConnections(conns []SavedConnection) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(conns, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "connections.json"), data, 0600)
}

func upsertConnection(conn SavedConnection) error {
	conns, err := loadConnections()
	if err != nil {
		return err
	}
	for i, c := range conns {
		if c.Name == conn.Name {
			conns[i] = conn
			return persistConnections(conns)
		}
	}
	return persistConnections(append(conns, conn))
}

func removeConnection(name string) error {
	conns, err := loadConnections()
	if err != nil {
		return err
	}
	filtered := conns[:0]
	for _, c := range conns {
		if c.Name != name {
			filtered = append(filtered, c)
		}
	}
	return persistConnections(filtered)
}
