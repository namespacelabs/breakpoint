package githuboidc

import "github.com/golang-jwt/jwt/v4"

type Claims struct {
	jwt.RegisteredClaims

	JobWorkflowRef    string `json:"job_workflow_ref"`
	Sha               string `json:"sha"`
	EventName         string `json:"event_name"`
	Repository        string `json:"repository"`
	Workflow          string `json:"workflow"`
	Ref               string `json:"ref"`
	JobWorkflowSha    string `json:"job_workflow_sha"`
	RunnerEnvironment string `json:"runner_environment"`
	RepositoryID      string `json:"repository_id"`
	RepositoryOwner   string `json:"repository_owner"`
	RepositoryOwnerID string `json:"repository_owner_id"`
	WorkflowRef       string `json:"workflow_ref"`
	WorkflowSha       string `json:"workflow_sha"`
	RunID             string `json:"run_id"`
	RunAttempt        string `json:"run_attempt"`
}
