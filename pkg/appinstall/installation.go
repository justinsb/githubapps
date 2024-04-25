package appinstall

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/GoogleContainerTools/kpt/tools/github-actions/api/v1alpha1"
	"github.com/GoogleContainerTools/kpt/tools/github-actions/pkg/events"
	github "github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Installation struct {
	k8s client.Client

	installationID int64
	githubClient   *github.Client
	org            string

	// eventHandlers []events.Handler

	lastUpdateMutex sync.Mutex
	lastUpdate      map[string]time.Time
}

func newInstallationTokenSource(appGithubClient *github.Client, installationID int64) (oauth2.TokenSource, error) {
	ts := &installationTokenSource{
		appGithubClient: appGithubClient,
		installationID:  installationID,
	}
	tok, err := ts.Token()
	if err != nil {
		return nil, err
	}
	rts := oauth2.ReuseTokenSource(tok, ts)
	return rts, nil
}

type installationTokenSource struct {
	appGithubClient *github.Client
	installationID  int64
}

func (ts *installationTokenSource) Token() (*oauth2.Token, error) {
	ctx := context.Background()

	token, _, err := ts.appGithubClient.Apps.CreateInstallationToken(ctx, ts.installationID, &github.InstallationTokenOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating github app installation token: %w", err)
	}

	return &oauth2.Token{AccessToken: token.GetToken(), TokenType: "Bearer", Expiry: token.GetExpiresAt().Time}, nil
}

func NewInstallation(ctx context.Context, k8s client.Client, appGithubClient *github.Client, installationID int64, org string) (*Installation, error) {
	i := &Installation{
		k8s:            k8s,
		installationID: installationID,
		org:            org,
	}

	githubTokenSource, err := newInstallationTokenSource(appGithubClient, installationID)
	if err != nil {
		return nil, err
	}

	ghClient := github.NewClient(oauth2.NewClient(ctx, githubTokenSource))
	i.githubClient = ghClient

	i.lastUpdate = make(map[string]time.Time)

	return i, nil
}

// func (i *Installation) AddEventHandler(handler events.Handler) {
// 	i.eventHandlers = append(i.eventHandlers, handler)
// }

func (i *Installation) GithubClient() *github.Client {
	return i.githubClient
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
			// for _, handler := range i.eventHandlers {
			// 	if h, ok := handler.(events.IssueCommentHandler); ok {
			// 		ev := &events.IssueCommentEvent{
			// 			CommentBody:   payload.GetComment().GetBody(),
			// 			CommentAuthor: payload.GetComment().GetUser().GetLogin(),
			// 			PRAuthor:      payload.GetIssue().GetUser().GetLogin(),
			// 			PRNumber:      payload.GetIssue().GetNumber(),
			// 		}
			// 		if err := h.OnIssueCommentEvent(ctx, scope, ev); err != nil {
			// 			return pollInterval, fmt.Errorf("handling event: %w", err)
			// 		}
			// 	}
			// }

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
			// for _, handler := range i.eventHandlers {
			// 	if h, ok := handler.(events.IssueCommentHandler); ok {
			// 		ev := &events.IssueCommentEvent{
			// 			CommentBody:   payload.GetComment().GetBody(),
			// 			CommentAuthor: payload.GetComment().GetUser().GetLogin(),
			// 			PRAuthor:      payload.GetIssue().GetUser().GetLogin(),
			// 			PRNumber:      payload.GetIssue().GetNumber(),
			// 		}
			// 		if err := h.OnIssueCommentEvent(ctx, scope, ev); err != nil {
			// 			return pollInterval, fmt.Errorf("handling event: %w", err)
			// 		}
			// 	}
			// }

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
		State:     "all", // we want to update state after prs/issues are closed
		Sort:      "updated",
		Direction: "desc",
	}
	// TODO: Use Since?
	opt.PerPage = 100
	issues, _, err := i.githubClient.Issues.ListByRepo(ctx, owner, repo, opt)
	if err != nil {
		return fmt.Errorf("listing events for repo %s/%s: %w", owner, repo, err)
	}

	s := &ObjectSyncer{
		GithubClient: i.githubClient,
		KubeClient:   i.k8s,
	}

	for _, issue := range issues {
		if i.seenUpdated(ctx, issue.GetNodeID(), issue.GetUpdatedAt().Time) {
			continue
		}
		if !issue.IsPullRequest() {
			continue
		}
		if err := s.SyncObjectFromGithub(ctx, owner, repo, issue.GetNumber()); err != nil {
			klog.Infof("issue is %+v", issue)
			return err
		}
		i.recordSeen(ctx, issue.GetNodeID(), issue.GetUpdatedAt().Time)
	}

	return nil
}

type ObjectSyncer struct {
	GithubClient *github.Client
	KubeClient   client.Client
}

func (s *ObjectSyncer) SyncObjectFromGithub(ctx context.Context, owner string, repo string, issueNumber int) error {

	pullRequest, _, err := s.GithubClient.PullRequests.Get(ctx, owner, repo, issueNumber)
	if err != nil {
		return fmt.Errorf("getting pr %s/%s %v: %w", owner, repo, issueNumber, err)
	}

	labels, _, err := s.GithubClient.Issues.ListLabelsByIssue(ctx, owner, repo, issueNumber, &github.ListOptions{})
	if err != nil {
		return fmt.Errorf("getting issue labels %s/%s %v: %w", owner, repo, issueNumber, err)
	}

	klog.Infof("PR %v", pullRequest.GetTitle())
	// scope := &events.Scope{
	// 	Github: i.githubClient,
	// 	Owner:  owner,
	// 	Repo:   repo,
	// }

	listCommentsOpts := &github.IssueListCommentsOptions{
		Sort:      PtrTo("updated"),
		Direction: PtrTo("desc"),
		// TODO: Since?
	}
	listCommentsOpts.PerPage = 100
	comments, _, err := s.GithubClient.Issues.ListComments(ctx, owner, repo, pullRequest.GetNumber(), listCommentsOpts)
	if err != nil {
		return fmt.Errorf("listing comments for pr %s/%s %v: %w", owner, repo, pullRequest.GetNumber(), err)
	}

	// ev := &events.PullRequest{
	// 	Number:  pullRequest.GetNumber(),
	// 	Title:   pullRequest.GetTitle(),
	// 	Author:  pullRequest.GetUser().GetLogin(),
	// 	BaseRef: pullRequest.GetBase().GetRef(),
	// 	Merged:  pullRequest.GetMerged(),
	// }

	// for _, label := range labels {
	// 	ev.Labels = append(ev.Labels, label.GetName())
	// }

	// for _, comment := range comments {
	// 	ev.Comments = append(ev.Comments, events.Comment{
	// 		Body:   comment.GetBody(),
	// 		Author: comment.GetUser().GetLogin(),
	// 	})
	// }

	// for _, handler := range i.eventHandlers {
	// 	if h, ok := handler.(events.PullRequestHandler); ok {
	// 		if err := h.OnPullRequestUpdated(ctx, scope, ev); err != nil {
	// 			return fmt.Errorf("handling event: %w", err)
	// 		}
	// 	} else {
	// 		klog.Warningf("unhandled handler type %T", handler)
	// 	}
	// }

	id := types.NamespacedName{Namespace: repo, Name: fmt.Sprintf("github-%d", pullRequest.GetNumber())}
	u := &unstructured.Unstructured{}
	u.SetName(id.Name)
	u.SetNamespace(id.Namespace)
	u.SetGroupVersionKind(v1alpha1.PullRequestGVK)
	spec := map[string]any{}
	spec["title"] = pullRequest.GetTitle()
	spec["url"] = pullRequest.GetHTMLURL()
	spec["state"] = pullRequest.GetState()
	spec["author"] = pullRequest.GetUser().GetLogin()
	spec["body"] = pullRequest.GetBody()
	spec["createdAt"] = asTime(pullRequest.GetCreatedAt())
	spec["updatedAt"] = asTime(pullRequest.GetUpdatedAt())

	specLabels := []v1alpha1.Label{} // empty array to keep CRDs happy
	for _, label := range labels {
		out := v1alpha1.Label{
			Name: label.GetName(),
			// CreatedAt: asTime(label.GetCreatedAt()),
		}
		specLabels = append(specLabels, out)
	}
	spec["labels"] = specLabels

	specBase := v1alpha1.BaseRef{}
	specBase.Ref = pullRequest.GetBase().GetRef()
	specBase.SHA = pullRequest.GetBase().GetSHA()
	// for _, label := range labels {
	// 	out := v1alpha1.Label{
	// 		Name: label.GetName(),
	// 		// CreatedAt: asTime(label.GetCreatedAt()),
	// 	}
	// 	specLabels = append(specLabels, out)
	// }
	spec["base"] = specBase

	specComments := []v1alpha1.Comment{} // empty array to keep CRDs happy
	for _, comment := range comments {
		out := v1alpha1.Comment{
			Body:      comment.GetBody(),
			Author:    comment.GetUser().GetLogin(),
			CreatedAt: asTime(comment.GetCreatedAt()),
		}
		specComments = append(specComments, out)
	}
	spec["comments"] = specComments

	ref := pullRequest.GetHead().GetSHA()
	// if pullRequest.GetNumber() == 4106 {
	// 	checkSuiteList, _, err := i.githubClient.Checks.ListCheckSuitesForRef(ctx, owner, repo, ref, &github.ListCheckSuiteOptions{})
	// 	if err != nil {
	// 		return fmt.Errorf("listing comments for pr %s/%s %v: %w", owner, repo, pullRequest.GetNumber(), err)
	// 	}
	// 	klog.Infof("list check suites %q: %+v", ref, checkSuiteList.CheckSuites)
	// 	klog.Fatalf("foo")
	// }

	checks, _, err := s.GithubClient.Checks.ListCheckRunsForRef(ctx, owner, repo, ref, &github.ListCheckRunsOptions{})
	if err != nil {
		return fmt.Errorf("getting checks %s/%s %v: %w", owner, repo, issueNumber, err)
	}
	// klog.Infof("list checks: %+v", checks.CheckRuns)

	checkSuites := []*v1alpha1.CheckSuite{} // empty array to keep CRDs happy
	checkSuiteMap := make(map[int64]*v1alpha1.CheckSuite)

	for _, checkRun := range checks.CheckRuns {
		checkSuiteID := checkRun.GetCheckSuite().GetID()
		checkSuite := checkSuiteMap[checkSuiteID]
		if checkSuite == nil {
			checkSuite = &v1alpha1.CheckSuite{
				ID: checkSuiteID,
			}
			checkSuiteMap[checkSuiteID] = checkSuite
			checkSuites = append(checkSuites, checkSuite)
		}
		out := v1alpha1.Check{
			Name:        checkRun.GetName(),
			Status:      checkRun.GetStatus(),
			Conclusion:  checkRun.GetConclusion(),
			CompletedAt: asOptionalTime(checkRun.CompletedAt),
			ID:          checkRun.GetID(),
		}
		checkSuite.Checks = append(checkSuite.Checks, out)
	}
	spec["checkSuites"] = checkSuites

	u.Object["spec"] = spec

	klog.Infof("patching PullRequest %v", AsJSON(u))
	if err := s.KubeClient.Patch(ctx, u, client.Apply, &client.PatchOptions{
		FieldManager: "github-syncer",
	}); err != nil {
		return fmt.Errorf("updating PullRequest: %w", err)
	}

	return nil
}

func AsJSON(obj any) string {
	b, err := json.Marshal(obj)
	if err != nil {
		return fmt.Sprintf("<error:%v>", err)
	}
	return string(b)
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

func asOptionalTime(t *github.Timestamp) *metav1.Time {
	if t == nil {
		return nil
	}
	mt := asTime(*t)
	return &mt
}

func asTime(t github.Timestamp) metav1.Time {
	mt := metav1.NewTime(t.Time)
	return mt
}
