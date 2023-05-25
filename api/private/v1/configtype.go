package v1

type WaitConfig struct {
	Endpoint              string    `json:"endpoint"`
	Duration              string    `json:"duration"`
	AuthorizedKeys        []string  `json:"authorized_keys"`
	AuthorizedGithubUsers []string  `json:"authorized_github_users"`
	Shell                 []string  `json:"shell"`
	AllowedSSHUsers       []string  `json:"allowed_ssh_users"`
	Enable                []string  `json:"enable"`
	Webhooks              []Webhook `json:"webhooks"`
}

type Webhook struct {
	URL     string         `json:"url"`
	Payload map[string]any `json:"payload"`
}
