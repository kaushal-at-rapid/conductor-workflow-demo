package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/conductor-sdk/conductor-go/sdk/model"
	"github.com/conductor-sdk/conductor-go/sdk/settings"
	"github.com/conductor-sdk/conductor-go/sdk/worker"
	"github.com/lib/pq"
)

var db *sql.DB

// getEnv returns the value of the environment variable if set, otherwise the provided default.
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// initDB initializes the Postgres connection and sets up tables.
func initDB() {
	// Read DB configuration from environment with sensible defaults
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "user")
	password := getEnv("DB_PASSWORD", "password")
	dbname := getEnv("DB_NAME", "conductor")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	if err = db.Ping(); err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}

	// Set up tables
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS enterprise (
            id SERIAL PRIMARY KEY,
            name VARCHAR(255) UNIQUE NOT NULL,
            details TEXT
        );
        CREATE TABLE IF NOT EXISTS "user" (
            id SERIAL PRIMARY KEY,
            enterprise_id INT REFERENCES enterprise(id),
            username VARCHAR(255) UNIQUE NOT NULL
        );
        CREATE TABLE IF NOT EXISTS worker_state (
            task_id VARCHAR(128) PRIMARY KEY,
            workflow_id VARCHAR(128),
            task_type VARCHAR(255),
            status VARCHAR(32),
            input JSONB,
            output JSONB,
            error TEXT,
            created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW()
        );
        CREATE OR REPLACE FUNCTION set_updated_at()
        RETURNS TRIGGER AS $$
        BEGIN
          NEW.updated_at = NOW();
          RETURN NEW;
        END;
        $$ LANGUAGE plpgsql;
        DO $$ BEGIN
          IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'worker_state_set_updated_at') THEN
            CREATE TRIGGER worker_state_set_updated_at
            BEFORE UPDATE ON worker_state
            FOR EACH ROW EXECUTE FUNCTION set_updated_at();
          END IF;
        END $$;
    `)
	if err != nil {
		log.Fatalf("Error creating tables: %v", err)
	}
	log.Println("Database connection successful and tables checked.")
}

// createEnterpriseWorker implements the 'create_enterprise_task'
// recordWorkerState persists the worker task state in Postgres
func recordWorkerState(t *model.Task, status string, output map[string]interface{}, errText *string) {
	if db == nil || t == nil {
		return
	}
	inBytes, _ := json.Marshal(t.InputData)
	var outStr *string
	if output != nil {
		ob, _ := json.Marshal(output)
		s := string(ob)
		outStr = &s
	}
	// Build params
	params := []interface{}{t.TaskId, t.WorkflowInstanceId, t.TaskType, status, string(inBytes), outStr, errText}
	_, e := db.Exec(`
		INSERT INTO worker_state (task_id, workflow_id, task_type, status, input, output, error, updated_at)
		VALUES ($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7, NOW())
		ON CONFLICT (task_id) DO UPDATE SET
		  status=EXCLUDED.status,
		  output=EXCLUDED.output,
		  error=EXCLUDED.error,
		  updated_at=NOW()
	`, params...)
	if e != nil {
		log.Printf("failed to record worker state for task %s: %v", t.TaskId, e)
	}
}

// withStateLogging wraps a worker handler to record state transitions
func withStateLogging(fn func(*model.Task) (interface{}, error)) func(*model.Task) (interface{}, error) {
	return func(t *model.Task) (interface{}, error) {
		recordWorkerState(t, "STARTED", nil, nil)
		res, err := fn(t)
		if err != nil {
			errStr := err.Error()
			recordWorkerState(t, "FAILED", nil, &errStr)
			return nil, err
		}
		var out map[string]interface{}
		if m, ok := res.(map[string]interface{}); ok {
			out = m
		}
		recordWorkerState(t, "COMPLETED", out, nil)
		return res, nil
	}
}

func createEnterpriseWorker(t *model.Task) (interface{}, error) {
	entpName, ok := t.InputData["entp_name"].(string)
	if !ok || entpName == "" {
		return nil, fmt.Errorf("missing entp_name in task input")
	}

	var entpID int
	err := db.QueryRow("INSERT INTO enterprise (name, details) VALUES ($1, $2) RETURNING id", entpName, "Enterprise Details Here").Scan(&entpID)
	if err != nil {
		// If insert failed due to unique constraint, fetch existing enterprise id
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			if qerr := db.QueryRow("SELECT id FROM enterprise WHERE name = $1", entpName).Scan(&entpID); qerr != nil {
				log.Printf("Worker 1 FAILED selecting existing enterprise after duplicate error: %v", qerr)
				return nil, fmt.Errorf("failed to find existing enterprise after duplicate error: %v", qerr)
			}
			log.Printf("Worker 1: Enterprise '%s' already exists with ID: %d", entpName, entpID)
			return map[string]interface{}{"enterprise_id": entpID}, nil
		}
		log.Printf("Worker 1 FAILED: %v", err)
		return nil, fmt.Errorf("failed to create enterprise: %v", err)
	}

	log.Printf("Worker 1: Enterprise '%s' created with ID: %d", entpName, entpID)
	return map[string]interface{}{"enterprise_id": entpID}, nil
}

// onboardEmployeeWorker implements the 'create_user_task'
func onboardEmployeeWorker(t *model.Task) (interface{}, error) {
	// Get inputs from the workflow
	entpIDFloat, ok := t.InputData["enterprise_id"].(float64)
	if !ok {
		return nil, fmt.Errorf("missing or invalid enterprise_id in task input")
	}
	entpID := int(entpIDFloat)

	userName, ok := t.InputData["user_name"].(string)
	if !ok || userName == "" {
		return nil, fmt.Errorf("missing user_name in task input")
	}

	var userID int
	err := db.QueryRow(`INSERT INTO "user" (enterprise_id, username) VALUES ($1, $2) RETURNING id`, entpID, userName).Scan(&userID)
	if err != nil {
		log.Printf("Worker 2 FAILED: %v", err)
		return nil, fmt.Errorf("failed to create user: %v", err)
	}

	log.Printf("Worker 2: User '%s' created with ID: %d in Enterprise %d", userName, userID, entpID)
	return map[string]interface{}{"user_id": userID}, nil
}

func main() {
	// Initialize DB connection (reads env vars or uses defaults)
	initDB()

	// Conductor Client Setup (conductor-go v1.6.x)
	apiURL := getEnv("CONDUCTOR_API_URL", "http://localhost:8080/api")
	authSettings := &settings.AuthenticationSettings{}
	httpSettings := &settings.HttpSettings{BaseUrl: apiURL}
	taskRunner := worker.NewTaskRunner(authSettings, httpSettings)

	// Register Workers
	log.Println("Starting Conductor Workers...")
	taskRunner.StartWorker("create_enterprise_task", withStateLogging(createEnterpriseWorker), 1, 100*time.Millisecond)
	taskRunner.StartWorker("create_user_task", withStateLogging(onboardEmployeeWorker), 1, 100*time.Millisecond)

	// Keep the worker process running
	select {}
}
