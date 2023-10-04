package appinstall

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GoogleContainerTools/kpt/tools/github-actions/pkg/events"
	github "github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"
	"k8s.io/klog/v2"
)

type Installation struct {
	installationID int64
	githubClient   *github.Client
	org            string

	eventHandlers []events.Handler

	lastUpdateMutex sync.Mutex
	lastUpdate      map[string]time.Time
}

func NewInstallation(ctx context.Context, appGithubClient *github.Client, installationID int64, org string) (*Installation, error) {
	i := &Installation{
		installationID: installationID,
		org:            org,
	}

	token, _, err := appGithubClient.Apps.CreateInstallationToken(ctx, installationID, &github.InstallationTokenOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating github app installation token: %w", err)
	}

	klog.Infof("token is %+v", token)
	// klog.Infof("response is %+v", response)

	githubTokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token.GetToken()})

	ghClient := github.NewClient(oauth2.NewClient(ctx, githubTokenSource))
	i.githubClient = ghClient

	i.lastUpdate = make(map[string]time.Time)

	return i, nil
}

func (i *Installation) AddEventHandler(handler events.Handler) {
	i.eventHandlers = append(i.eventHandlers, handler)
}

// PollEventsOnce queries for events once
func (i *Installation) PollOrgEventsOnce(ctx context.Context) (time.Duration, error) {
	pollInterval := time.Minute

	page, response, err := i.githubClient.Activity.ListEventsForOrganization(ctx, i.org, &github.ListOptions{})
	if err != nil {
		return pollInterval, fmt.Errorf("listing events for org %q: %w", i.org, err)
	}

	for _, ev := range page {
		scope := &events.Scope{
			Github: i.githubClient,
		}

		klog.Infof("%s id %v, type %v", ev.GetRepo().GetName(), ev.GetID(), ev.GetType())
		repoName := ev.GetRepo().GetName() // <org>/<name>
		if tokens := strings.Split(repoName, "/"); len(tokens) != 2 {
			return pollInterval, fmt.Errorf("unexpected repo name %q", repoName)
		} else {
			scope.Owner = tokens[0]
			scope.Repo = tokens[1]
		}

		payload, err := ev.ParsePayload()
		if err != nil {
			return pollInterval, fmt.Errorf("parsing event: %w", err)
		}

		switch payload := payload.(type) {
		case *github.IssueCommentEvent:
			for _, handler := range i.eventHandlers {
				if h, ok := handler.(events.IssueCommentHandler); ok {
					ev := &events.IssueCommentEvent{
						CommentBody:   payload.GetComment().GetBody(),
						CommentAuthor: payload.GetComment().GetUser().GetLogin(),
						PRAuthor:      payload.GetIssue().GetUser().GetLogin(),
						PRNumber:      payload.GetIssue().GetNumber(),
					}
					if err := h.OnIssueCommentEvent(ctx, scope, ev); err != nil {
						return pollInterval, fmt.Errorf("handling event: %w", err)
					}
				}
			}

		default:
			klog.V(2).Infof("ignoring payload type %T", payload)
		}

	}
	// klog.Infof("response = %+v", response)
	xPollInterval := response.Header.Get("X-Poll-Interval")
	if xPollInterval == "" {
		xPollInterval = "60"
	}
	interval, err := time.ParseDuration(xPollInterval + "s")
	if err != nil {
		return pollInterval, fmt.Errorf("parsing X-Poll-Interval %q: %w", xPollInterval, err)
	}
	pollInterval = interval
	klog.Infof("X-Poll-Interval = %+v", pollInterval)

	return pollInterval, nil
}

// PollRepoEventsOnce queries for events once
func (i *Installation) PollRepoEventsOnce(ctx context.Context, owner string, repo string) (time.Duration, error) {
	pollInterval := time.Minute

	page, response, err := i.githubClient.Activity.ListRepositoryEvents(ctx, owner, repo, &github.ListOptions{})
	if err != nil {
		return pollInterval, fmt.Errorf("listing events for repo %s/%s: %w", owner, repo, err)
	}

	for _, ev := range page {
		scope := &events.Scope{
			Github: i.githubClient,
		}

		klog.Infof("%s id %v, type %v", ev.GetRepo().GetName(), ev.GetID(), ev.GetType())
		repoName := ev.GetRepo().GetName() // <org>/<name>
		if tokens := strings.Split(repoName, "/"); len(tokens) != 2 {
			return pollInterval, fmt.Errorf("unexpected repo name %q", repoName)
		} else {
			scope.Owner = tokens[0]
			scope.Repo = tokens[1]
		}

		payload, err := ev.ParsePayload()
		if err != nil {
			return pollInterval, fmt.Errorf("parsing event: %w", err)
		}

		switch payload := payload.(type) {
		case *github.IssueCommentEvent:
			for _, handler := range i.eventHandlers {
				if h, ok := handler.(events.IssueCommentHandler); ok {
					ev := &events.IssueCommentEvent{
						CommentBody:   payload.GetComment().GetBody(),
						CommentAuthor: payload.GetComment().GetUser().GetLogin(),
						PRAuthor:      payload.GetIssue().GetUser().GetLogin(),
						PRNumber:      payload.GetIssue().GetNumber(),
					}
					if err := h.OnIssueCommentEvent(ctx, scope, ev); err != nil {
						return pollInterval, fmt.Errorf("handling event: %w", err)
					}
				}
			}

		default:
			klog.V(2).Infof("ignoring payload type %T", payload)
		}

	}
	// klog.Infof("response = %+v", response)
	xPollInterval := response.Header.Get("X-Poll-Interval")
	if xPollInterval == "" {
		xPollInterval = "60"
	}
	interval, err := time.ParseDuration(xPollInterval + "s")
	if err != nil {
		return pollInterval, fmt.Errorf("parsing X-Poll-Interval %q: %w", xPollInterval, err)
	}
	pollInterval = interval
	klog.Infof("X-Poll-Interval = %+v", pollInterval)

	return pollInterval, nil
}

func (i *Installation) PollForUpdates(ctx context.Context, owner string, repo string) error {
	klog.Infof("polling for updates on %s/%s", owner, repo)
	opt := &github.IssueListByRepoOptions{
		State:     "open",
		Sort:      "updated",
		Direction: "desc",
	}
	opt.PerPage = 100
	issues, _, err := i.githubClient.Issues.ListByRepo(ctx, owner, repo, opt)
	if err != nil {
		return fmt.Errorf("listing events for repo %s/%s: %w", owner, repo, err)
	}

	for _, issue := range issues {
		if i.seenUpdated(ctx, issue.GetNodeID(), issue.GetUpdatedAt().Time) {
			continue
		}
		if !issue.IsPullRequest() {
			continue
		}
		pullRequest, _, err := i.githubClient.PullRequests.Get(ctx, owner, repo, issue.GetNumber())
		if err != nil {
			klog.Infof("issue is %+v", issue)
			return fmt.Errorf("getting pr %s/%s %v: %w", owner, repo, issue.GetNumber(), err)
		}

		labels, _, err := i.githubClient.Issues.ListLabelsByIssue(ctx, owner, repo, issue.GetNumber(), &github.ListOptions{})
		if err != nil {
			return fmt.Errorf("getting issue labels %s/%s %v: %w", owner, repo, issue.GetNumber(), err)
		}

		klog.Infof("PR %v", pullRequest.GetTitle())
		scope := &events.Scope{
			Github: i.githubClient,
			Owner:  owner,
			Repo:   repo,
		}

		listCommentsOpts := &github.IssueListCommentsOptions{
			Sort:      PtrTo("updated"),
			Direction: PtrTo("desc"),
			// TODO: Since?
		}
		listCommentsOpts.PerPage = 100
		comments, _, err := i.githubClient.Issues.ListComments(ctx, owner, repo, pullRequest.GetNumber(), listCommentsOpts)
		if err != nil {
			return fmt.Errorf("listing comments for pr %s/%s %v: %w", owner, repo, pullRequest.GetNumber(), err)
		}

		ev := &events.PullRequest{
			Number:  pullRequest.GetNumber(),
			Title:   pullRequest.GetTitle(),
			Author:  pullRequest.GetUser().GetLogin(),
			BaseRef: pullRequest.GetBase().GetRef(),
			Merged:  pullRequest.GetMerged(),
		}

		for _, label := range labels {
			ev.Labels = append(ev.Labels, label.GetName())
		}

		for _, comment := range comments {
			ev.Comments = append(ev.Comments, events.Comment{
				Body:   comment.GetBody(),
				Author: comment.GetUser().GetLogin(),
			})
		}

		for _, handler := range i.eventHandlers {
			if h, ok := handler.(events.PullRequestHandler); ok {
				if err := h.OnPullRequestUpdated(ctx, scope, ev); err != nil {
					return fmt.Errorf("handling event: %w", err)
				}
			} else {
				klog.Warningf("unhandled handler type %T", handler)
			}
		}

		i.recordSeen(ctx, issue.GetNodeID(), issue.GetUpdatedAt().Time)
	}

	return nil
}

func PtrTo[T any](t T) *T {
	return &t
}

func (i *Installation) seenUpdated(ctx context.Context, nodeID string, updatedAt time.Time) bool {
	i.lastUpdateMutex.Lock()
	lastUpdate, found := i.lastUpdate[nodeID]
	i.lastUpdateMutex.Unlock()
	if found && lastUpdate == updatedAt {
		return true
	}
	return false
}
func (i *Installation) recordSeen(ctx context.Context, nodeID string, updatedAt time.Time) {
	i.lastUpdateMutex.Lock()
	i.lastUpdate[nodeID] = updatedAt
	i.lastUpdateMutex.Unlock()
}

/*
Maybe a fast query could be:

{
  repository(owner: "kptdev", name: "kpt") {
    name
    updatedAt
    issues(first: 1, orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes {
        updatedAt
        number
        comments(first: 1, orderBy: {field: UPDATED_AT, direction: DESC}) {
          nodes {
            updatedAt
            databaseId
          }
        }
      }
    }
    pullRequests(first: 1, orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes {
        updatedAt
        number
        comments(first: 1, orderBy: {field: UPDATED_AT, direction: DESC}) {
          nodes {
            updatedAt
            databaseId
          }
        }
      }
    }
    discussions(first: 1, orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes {
        updatedAt
        number
        comments(first: 1) {
          nodes {
            updatedAt
            databaseId
          }
        }
      }
    }
  }
}
*/
