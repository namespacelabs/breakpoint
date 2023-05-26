package waiter

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/google/go-github/v52/github"
	"github.com/rs/zerolog"
	"github.com/slack-go/slack"
	v1 "namespacelabs.dev/breakpoint/api/private/v1"
	"namespacelabs.dev/breakpoint/pkg/jsonfile"
)

type botInstance struct {
	client      *slack.Client
	m           *Manager
	githubProps renderGitHubProps

	channelID string
	ts        string
}

func startBot(ctx context.Context, m *Manager, conf v1.SlackBot) *botInstance {
	bot := &botInstance{
		client:      slack.New(os.ExpandEnv(conf.Token)),
		m:           m,
		githubProps: prepareGitHubProps(ctx),
	}

	chid, ts, err := bot.client.PostMessageContext(ctx, os.ExpandEnv(conf.Channel), bot.makeBlocks(false))
	if err != nil {
		zerolog.Ctx(ctx).Err(err).Msg("SlackBot failed")
		return nil
	}

	bot.channelID = chid
	bot.ts = ts

	go bot.loop(ctx)

	return bot
}

func (b *botInstance) Close() error {
	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()

	return b.sendUpdate(ctx, true)
}

func (b *botInstance) makeBlocks(leaving bool) slack.MsgOption {
	if leaving {
		return slack.MsgOptionBlocks(renderGitHubMessage(b.githubProps, "", time.Time{})...)
	}

	return slack.MsgOptionBlocks(renderGitHubMessage(b.githubProps, b.m.Endpoint(), b.m.Expiration())...)
}

func (b *botInstance) sendUpdate(ctx context.Context, leaving bool) error {
	_, _, _, err := b.client.UpdateMessageContext(ctx, b.channelID, b.ts, b.makeBlocks(leaving))
	return err
}

func (b *botInstance) loop(ctx context.Context) error {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-t.C:
			if err := b.sendUpdate(ctx, false); err != nil {
				return err
			}
		}
	}
}

type renderGitHubProps struct {
	Repository string
	RefName    string
	Workflow   string
	RunID      string
	RunNumber  string
	Actor      string
	PushEvent  *github.PushEvent // Only set on push events.
}

func prepareGitHubProps(ctx context.Context) renderGitHubProps {
	props := renderGitHubProps{
		Repository: os.Getenv("GITHUB_REPOSITORY"),
		RefName:    os.Getenv("GITHUB_REF_NAME"),
		Workflow:   os.Getenv("GITHUB_WORKFLOW"),
		RunID:      os.Getenv("GITHUB_RUN_ID"),
		RunNumber:  os.Getenv("GITHUB_RUN_NUMBER"),
		Actor:      os.Getenv("GITHUB_ACTOR"),
	}

	if eventFile := os.Getenv("GITHUB_EVENT_PAH"); os.Getenv("GITHUB_EVENT_NAME") == "push" && eventFile != "" {
		var pushEvent github.PushEvent
		if err := jsonfile.Load(eventFile, &pushEvent); err != nil {
			zerolog.Ctx(ctx).Warn().Err(err).Msg("Failed to load event file")
		} else {
			props.PushEvent = &pushEvent
		}
	}

	return props
}

func renderGitHubMessage(props renderGitHubProps, endpoint string, exp time.Time) []slack.Block {
	blocks := []slack.Block{
		slack.NewHeaderBlock(slack.NewTextBlockObject(slack.PlainTextType, "Workflow failed", false, false)),
		slack.NewSectionBlock(slack.NewTextBlockObject(
			slack.MarkdownType,
			fmt.Sprintf("*Repository:* <https://github.com/%s/tree/%s|github.com/%s> (%s)", props.Repository, props.RefName, props.Repository, props.RefName),
			false, false,
		), nil, nil),
		slack.NewSectionBlock(slack.NewTextBlockObject(
			slack.MarkdownType,
			fmt.Sprintf("*Workflow:* %s (<https://github.com/%s/actions/runs/%s|Run #%s>)", props.Workflow, props.Repository, props.RunID, props.RunNumber),
			false, false,
		), nil, nil),
	}

	if props.PushEvent != nil && props.PushEvent.HeadCommit != nil && props.PushEvent.HeadCommit.Message != nil {
		blocks = append(blocks,
			slack.NewSectionBlock(slack.NewTextBlockObject(
				slack.MarkdownType,
				fmt.Sprintf("*<%s|Commit>:* %s`", maybeCommitURL(props.Repository, *props.PushEvent), *props.PushEvent.HeadCommit.Message),
				false, false,
			), nil, nil))
	}

	if endpoint != "" && !exp.IsZero() {
		host, port, _ := net.SplitHostPort(endpoint)

		blocks = append(blocks,
			slack.NewSectionBlock(slack.NewTextBlockObject(
				slack.MarkdownType,
				fmt.Sprintf("*SSH:* `ssh -p %s runner@%s`", port, host),
				false, false,
			), nil, nil),
			slack.NewSectionBlock(slack.NewTextBlockObject(
				slack.MarkdownType,
				fmt.Sprintf("*Expires:* %s (%s)", humanize.Time(exp), exp.Format(Stamp)),
				false, false,
			), nil, nil),
		)
	}

	blocks = append(blocks, slack.NewContextBlock("",
		slack.NewTextBlockObject(slack.PlainTextType, fmt.Sprintf("Actor: %s", props.Actor), false, false)))

	return blocks
}

func maybeCommitURL(repo string, event github.PushEvent) string {
	if event.HeadCommit == nil || event.HeadCommit.URL == nil {
		if event.Repo == nil {
			return "https://github.com/" + repo
		}

		return *event.Repo.URL
	}

	return *event.HeadCommit.URL
}
