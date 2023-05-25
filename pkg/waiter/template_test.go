package waiter

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	v1 "namespacelabs.dev/breakpoint/api/private/v1"
)

func TestExecTemplate(t *testing.T) {
	var webhook v1.Webhook

	if err := json.Unmarshal([]byte(`{
	"url": "foobar",
	"payload": {
        "blocks": [
          {
            "type": "header",
            "text": {
              "type": "plain_text",
              "text": "Workflow failed",
              "emoji": true
            }
          },
          {
            "type": "section",
            "text": {
              "type": "mrkdwn",
              "text": "*Repository:* <https://${GITHUB_REPOSITORY}/tree/${GITHUB_REF_NAME}|${GITHUB_REPOSITORY}> (${GITHUB_REF_NAME})"
            }
          }
		]
	}
}`), &webhook); err != nil {
		t.Fatal(err)
	}

	got := execTemplate(webhook.Payload, func(str string) string {
		switch str {
		case "GITHUB_REPOSITORY":
			return "arepo"

		case "GITHUB_REF_NAME":
			return "main"
		}

		return ""
	})

	if d := cmp.Diff(map[string]any{
		"blocks": []any{
			map[string]any{
				"text": map[string]any{
					"emoji": bool(true),
					"text":  string("Workflow failed"),
					"type":  string("plain_text"),
				},
				"type": string("header"),
			},
			map[string]any{
				"text": map[string]any{
					"text": string("*Repository:* <https://arepo/tree/main|arepo> (main)"),
					"type": string("mrkdwn"),
				},
				"type": string("section"),
			},
		},
	}, got); d != "" {
		t.Errorf("mismatch (-want +got):\n%s", d)
	}

}
