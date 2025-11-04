package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/conductor-sdk/conductor-go/sdk/client"
	"github.com/conductor-sdk/conductor-go/sdk/model"
	"github.com/conductor-sdk/conductor-go/sdk/settings"
	"github.com/conductor-sdk/conductor-go/sdk/workflow/executor"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

// OnboardRequest Define the request structure for the API
type OnboardRequest struct {
	EntpName string `json:"entp_name"`
	UserName string `json:"user_name"`
}

// getEnv returns the value of the environment variable if set, otherwise the provided default.
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Conductor SDK workflow executor
var wfExecutor *executor.WorkflowExecutor

// Shared DB connection for user service
var db *sql.DB

// TODO: create generic struct for conductor config
// TODO: create a load/new method that return this config

func init() {
	// Configure Conductor API URL via environment (same as worker)
	apiURL := getEnv("CONDUCTOR_API_URL", "http://localhost:8080/api")
	auth := &settings.AuthenticationSettings{}
	httpSettings := &settings.HttpSettings{BaseUrl: apiURL}
	apiClient := client.NewAPIClient(auth, httpSettings)
	wfExecutor = executor.NewWorkflowExecutor(apiClient)
}

// initDB initializes the Postgres connection and ensures tables exist
func initDB() error {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "user")
	password := getEnv("DB_PASSWORD", "password")
	dbname := getEnv("DB_NAME", "conductor")

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("error opening database: %w", err)
	}
	if err = db.Ping(); err != nil {
		return fmt.Errorf("error connecting to database: %w", err)
	}
	// Ensure tables exist (idempotent)
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
    `)
	if err != nil {
		return fmt.Errorf("error creating tables: %w", err)
	}
	log.Println("API: Database connection successful and tables checked.")
	return nil
}

// onboardHandler triggers the Conductor workflow
func onboardHandler(w http.ResponseWriter, r *http.Request) {
	var req OnboardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.EntpName == "" || req.UserName == "" {
		http.Error(w, "entp_name and user_name are required", http.StatusBadRequest)
		return
	}

	// 1. Define the input data for the Conductor workflow
	workflowInput := map[string]interface{}{
		"entp_name": req.EntpName,
		"user_name": req.UserName,
	}

	// 2. Start the workflow via Conductor SDK
	startReq := &model.StartWorkflowRequest{
		Name:    "onboard_employee_workflow",
		Version: int32(1),
		Input:   workflowInput,
	}
	workflowID, err := wfExecutor.StartWorkflow(startReq)
	if err != nil {
		log.Printf("Error starting workflow: %v", err)
		http.Error(w, "Failed to start workflow: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Workflow 'onboard_employee_workflow' started with ID: %s", workflowID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":      "Workflow started successfully",
		"workflow_id": workflowID,
	})
}

// UserCreateRequest is the payload to create a user directly via API
type UserCreateRequest struct {
	EnterpriseID int    `json:"enterprise_id"`
	UserName     string `json:"user_name"`
}

// User represents a user record
type User struct {
	ID           int    `json:"id"`
	EnterpriseID int    `json:"enterprise_id"`
	UserName     string `json:"user_name"`
}

// createUserHandler inserts a new user into the DB
func createUserHandler(w http.ResponseWriter, r *http.Request) {
	var req UserCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.EnterpriseID <= 0 || req.UserName == "" {
		http.Error(w, "enterprise_id and user_name are required", http.StatusBadRequest)
		return
	}

	var userID int
	err := db.QueryRow(`INSERT INTO "user" (enterprise_id, username) VALUES ($1, $2) RETURNING id`, req.EnterpriseID, req.UserName).Scan(&userID)
	if err != nil {
		log.Printf("API: failed to create user: %v", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"user_id": userID})
}

// listUsersHandler returns all users
func listUsersHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id, enterprise_id, username FROM "user" ORDER BY id`)
	if err != nil {
		log.Printf("API: failed to list users: %v", err)
		http.Error(w, "Failed to list users", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.EnterpriseID, &u.UserName); err != nil {
			http.Error(w, "Failed to read users", http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// getUserHandler returns a single user by ID
func getUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		http.Error(w, "Invalid user id", http.StatusBadRequest)
		return
	}
	var u User
	err = db.QueryRow(`SELECT id, enterprise_id, username FROM "user" WHERE id=$1`, id).Scan(&u.ID, &u.EnterpriseID, &u.UserName)
	if err == sql.ErrNoRows {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("API: failed to get user: %v", err)
		http.Error(w, "Failed to get user", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(u)
}

func main() {
	// Initialize DB for user service
	if err := initDB(); err != nil {
		log.Fatalf("API: DB initialization failed: %v", err)
	}

	router := mux.NewRouter()
	// Workflow trigger endpoint
	router.HandleFunc("/onboard", onboardHandler).Methods("POST")

	// User service endpoints
	router.HandleFunc("/users", createUserHandler).Methods("POST")
	router.HandleFunc("/users", listUsersHandler).Methods("GET")
	router.HandleFunc("/users/{id}", getUserHandler).Methods("GET")

	log.Println("API Service running on :8081")
	if err := http.ListenAndServe(":8081", router); err != nil {
		log.Fatal(err)
	}
}
