# Employee Onboarding System
## Overview
This project is a microservices-based onboarding workflow system using Netflix Conductor for orchestration, PostgreSQL as the DB, and Go API + worker services. This guide explains how to run all components together using Docker Compose.

## Prerequisites
- Docker installed and running

- Docker Compose installed

- Clone the project repository with all files including source code, workflow JSON, task definitions, and configuration

## Running the application
1) Build and Start Services. From the project root (where docker-compose.yml is located):
    - `docker-compose up --build`
    - This builds Go API and worker images, and starts PostgreSQL, Conductor, API, and worker services.


2) Confirm All Services Are Running
   Use:

    - `docker-compose ps`
   
   Ensure all services show as "Up".


3) Register Workflow and Task Definitions in Conductor
   - Before calling the workflow, register workflow and task metadata:
        
            curl -X POST http://localhost:8080/api/metadata/taskdefs -H "Content-Type: application/json" --data-binary @workflow/task_defs.json
   

            curl -X POST http://localhost:8080/api/metadata/workflow -H "Content-Type: application/json" --data-binary @workflow/onboard_entp_user_wf.json
   - This registers the task definitions and the onboarding workflow in Conductor.


4) Invoke Onboarding Workflow
   
    To start onboarding a new employee:
        
        curl -X POST http://localhost:8081/onboard -H "Content-Type: application/json" -d '{"entp_name": "AcmeCorp", "user_name": "jdoe"}'

    You should receive a workflow instance ID confirming success.


5) Monitor Workflows and Services

    Visit Conductor UI at http://localhost:8080 to view running workflows.

    Check logs for API and worker containers:

    `docker logs go-api-service`

    `docker logs go-worker-service`

## Troubleshooting
If containers fail to start, check logs for errors. 

Ensure Conductor server has metadata uploaded before invoking workflows.

If workflows aren’t found, re-upload metadata files.

For DB connection problems, verify environment variables and database readiness.

**Environment Variables Used**
`POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB for Postgres credentials
DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME for API and Worker DB connection, 
CONDUCTOR_API_URL for Conductor server's API endpoint`

## Notes
Data persistence for Postgres uses volume ./pgdata mapped inside the container.

Docker Compose ensures dependency ordering using depends_on with health checks.

Adjust any paths or ports in docker-compose.yml if conflicts occur.