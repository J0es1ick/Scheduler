package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	connector "github.com/J0es1ick/Scheduler/connector/v1"
)

type config struct {
	BaseURL     string `json:"base_url"`
	ConnectorID string `json:"connector_id"`
	KeyID       string `json:"key_id"`
	PrivateKey  string `json:"private_key"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, _ io.Writer) error {
	if len(args) == 0 {
		printUsage(stdout)
		return nil
	}
	switch args[0] {
	case "validate":
		if len(args) != 2 {
			return errors.New("usage: scheduler-connector validate <snapshot.json>")
		}
		snapshot, err := loadSnapshot(args[1])
		if err != nil {
			return err
		}
		if err = connector.Validate(snapshot); err != nil {
			return err
		}
		lessons := 0
		for _, group := range snapshot.Groups {
			lessons += len(group.Lessons)
		}
		_, _ = fmt.Fprintf(stdout, "valid: schema=%s groups=%d lessons=%d snapshot=%s\n",
			snapshot.SchemaVersion, len(snapshot.Groups), lessons, snapshot.SnapshotID)
		return nil
	case "push":
		if len(args) != 3 {
			return errors.New("usage: scheduler-connector push <connector.json> <snapshot.json>")
		}
		cfg, err := loadConfig(args[1])
		if err != nil {
			return err
		}
		snapshot, err := loadSnapshot(args[2])
		if err != nil {
			return err
		}
		client, err := connector.NewClient(cfg.BaseURL, cfg.ConnectorID, cfg.KeyID, cfg.PrivateKey)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		result, err := client.Submit(ctx, snapshot)
		if err != nil {
			return err
		}
		return printJSON(stdout, result)
	case "status":
		if len(args) != 3 {
			return errors.New("usage: scheduler-connector status <connector.json> <run-id>")
		}
		cfg, err := loadConfig(args[1])
		if err != nil {
			return err
		}
		client, err := connector.NewClient(cfg.BaseURL, cfg.ConnectorID, cfg.KeyID, cfg.PrivateKey)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := client.Run(ctx, args[2])
		if err != nil {
			return err
		}
		return printJSON(stdout, result)
	case "heartbeat":
		if len(args) != 2 {
			return errors.New("usage: scheduler-connector heartbeat <connector.json>")
		}
		cfg, err := loadConfig(args[1])
		if err != nil {
			return err
		}
		client, err := connector.NewClient(cfg.BaseURL, cfg.ConnectorID, cfg.KeyID, cfg.PrivateKey)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := client.Heartbeat(ctx)
		if err != nil {
			return err
		}
		return printJSON(stdout, result)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func loadConfig(path string) (config, error) {
	var value config
	if err := readJSON(path, &value); err != nil {
		return value, err
	}
	if value.BaseURL == "" || value.ConnectorID == "" || value.KeyID == "" || value.PrivateKey == "" {
		return value, errors.New("connector config is missing required fields")
	}
	return value, nil
}

func loadSnapshot(path string) (connector.Snapshot, error) {
	var value connector.Snapshot
	return value, readJSON(path, &value)
}

func readJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 64<<20))
	if err = decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("%s contains trailing JSON data", path)
	}
	return nil
}

func printJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printUsage(writer io.Writer) {
	_, _ = fmt.Fprintln(writer, `Scheduler Connector CLI

Commands:
  validate <snapshot.json>
  push <connector.json> <snapshot.json>
  status <connector.json> <run-id>
  heartbeat <connector.json>`)
}
