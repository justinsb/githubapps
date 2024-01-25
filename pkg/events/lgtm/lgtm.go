package lgtm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GoogleContainerTools/kpt/tools/github-actions/pkg/events"
	"github.com/GoogleContainerTools/kpt/tools/github-actions/pkg/githubgraphql"
	"github.com/google/go-github/v53/github"
	"k8s.io/klog/v2"
)

type EventHandler struct {
}

func (op *op) canLGTM(ctx context.Context, userName string) (bool, error) {
	codeOwners, err := op.scope.CodeOwners(ctx, op.pr.BaseRef)
	if err != nil {
		return false, err
	}

	for _, owner := range codeOwners.Owners {
		if owner == userName {
			return true, nil
		}
	}
	return false, nil
}

type op struct {
	scope *events.Scope
	pr    *events.PullRequest

	lgtm int
}

func (op *op) processComment(ctx context.Context, comment *events.Comment) error {
	body := comment.Body

	lgtm := 0

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line) // Remove /r in particular
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		line += " "
		klog.Infof("line %q", line)
		if strings.HasPrefix(line, "/lgtm ") {
			lgtm = 1
		}
		if strings.HasPrefix(line, "/lgtm cancel ") {
			lgtm = -1
		}
	}

	if lgtm == 0 {
		return nil
	}

	if lgtm != 0 {
		canLGTM, err := op.canLGTM(ctx, comment.Author)
		if err != nil {
			return err
		}
		if comment.Author == op.pr.Author {
			// TODO: post message?
			klog.Warningf("user %q cannot lgtm their own PR", comment.Author)
		} else if !canLGTM {
			// TODO: post message?
			klog.Warningf("user %q cannot lgtm PRs", comment.Author)
		} else {
			op.lgtm = lgtm
		}
	}

	return nil
}

func (r *EventHandler) OnPullRequestUpdated(ctx context.Context, scope *events.Scope, pr *events.PullRequest) error {
	op := &op{
		scope: scope,
		pr:    pr,
	}
	for _, comment := range pr.Comments {
		if err := op.processComment(ctx, &comment); err != nil {
			return err
		}
	}

	if op.lgtm == 0 {
		return nil
	}

	if op.lgtm == 1 {
		if err := r.addLabels(ctx, scope, pr, []string{"lgtm"}); err != nil {
			return fmt.Errorf("updating lgtm: %w", err)
		}
	} else if op.lgtm == -1 {
		if err := r.removeLabels(ctx, scope, pr, []string{"lgtm"}); err != nil {
			return fmt.Errorf("updating lgtm: %w", err)
		}
	}

	return nil
}

// func (r *EventHandler) OnIssueCommentEvent(ctx context.Context, scope *events.Scope, ev *events.IssueCommentEvent) error {

// 	// hasLGTM := false

// 	if lgtm == 0 && wip == 0 {
// 		return nil
// 	}

// 	// owner := ev.GetRepo().GetOwner().GetName()
// 	// repo := ev.GetRepo().GetName()
// 	number := ev.PRNumber

// 	if number != 3901 {
// 		klog.Warningf("skipping pr (hack) #%v", number)
// 		return nil
// 	}

// 	klog.Infof("issuecommentEvent %+v", ev)

// 	pr, _, err := scope.Github.PullRequests.Get(ctx, scope.Owner, scope.Repo, number)
// 	if err != nil {
// 		return fmt.Errorf("getting pull request: %w", err)
// 	}
// 	if pr.GetMerged() {
// 		// TODO: post to issue
// 		klog.Warningf("cannot update merged pull request")
// 		return nil
// 	}

// 	commentAuthor := ev.CommentAuthor
// 	prAuthor := ev.PRAuthor

// 	if lgtm != 0 {
// 		canLGTM, err := r.canLGTM(ctx, scope, ev, pr)
// 		if err != nil {
// 			return err
// 		}
// 		if commentAuthor == prAuthor {
// 			// TODO: post message?
// 			klog.Warningf("user %q cannot lgtm their own PR", ev.CommentAuthor)
// 		} else if !canLGTM {
// 			// TODO: post message?
// 			klog.Warningf("user %q cannot lgtm PRs", ev.CommentAuthor)
// 		} else if lgtm == 1 {
// 			if err := r.addLabels(ctx, scope, ev, []string{"lgtm"}); err != nil {
// 				return fmt.Errorf("updating lgtm: %w", err)
// 			}
// 		} else if lgtm == -1 {
// 			if err := r.removeLabels(ctx, scope, ev, []string{"lgtm"}); err != nil {
// 				return fmt.Errorf("updating lgtm: %w", err)
// 			}
// 		}
// 	}

// 	if wip != 0 {
// 		canWIP, err := r.canLGTM(ctx, scope, ev, pr)
// 		if err != nil {
// 			return err
// 		}
// 		if commentAuthor != prAuthor && !canWIP {
// 			// Users can always WIP their own PR (?)
// 			// TODO: post message?
// 			klog.Warningf("user %q cannot wip other people's PRs", ev.CommentAuthor)
// 		} else if wip == 1 {
// 			// Doesn't work .... get github /graphql response data: {"data":{"convertPullRequestToDraft":null},"errors":[{"type":"FORBIDDEN","path":["convertPullRequestToDraft"],"extensions":{"saml_failure":false},"locations":[{"line":1,"column":78}],"message":"Resource not accessible by integration"}]}

// 			// if err := r.convertPullRequestToDraft(ctx, scope, ev, pr); err != nil {
// 			// 	return fmt.Errorf("updating draft: %w", err)
// 			// }
// 		} else if wip == -1 {
// 			// if err := r.updatePullRequest(ctx, scope, ev, &github.PullRequest{State: "open"}); err != nil {
// 			// 	return fmt.Errorf("updating draft: %w", err)
// 			// }
// 		}
// 	}

// 	// if approve == 1 {
// 	// 	review := &github.PullRequestReviewRequest{
// 	// 		Event: PtrTo("APPROVE"),
// 	// 	}

// 	// 	if err := r.createReview(ctx, ev, review); err != nil {
// 	// 		return fmt.Errorf("updating lgtm: %w", err)
// 	// 	}
// 	// }

// 	return nil
// }

func PtrTo[T any](t T) *T {
	return &t
}

func (r *EventHandler) createReview(ctx context.Context, scope *events.Scope, ev *github.IssueCommentEvent, review *github.PullRequestReviewRequest) error {
	klog.Infof("posting review: %+v", review)

	number := ev.GetIssue().GetNumber()

	if _, _, err := scope.Github.PullRequests.CreateReview(ctx, scope.Owner, scope.Repo, number, review); err != nil {
		return fmt.Errorf("creating review: %w", err)
	}

	return nil
}

func (r *EventHandler) convertPullRequestToDraft(ctx context.Context, scope *events.Scope, ev *github.IssueCommentEvent, pr *github.PullRequest) error {
	if pr.GetNumber() != 3568 {
		klog.Warningf("skipping PR %v", pr.GetNumber())
		return nil
	}
	klog.Infof("convertPullRequestToDraft pr: %+v", pr.GetNodeID())

	// Doesn't work .... get github /graphql response data: {"data":{"convertPullRequestToDraft":null},"errors":[{"type":"FORBIDDEN","path":["convertPullRequestToDraft"],"extensions":{"saml_failure":false},"locations":[{"line":1,"column":78}],"message":"Resource not accessible by integration"}]}

	input := map[string]any{
		"clientMutationId": fmt.Sprintf("%v", time.Now().UnixNano()),
		"pullRequestId":    pr.GetNodeID(),
	}
	query := githubgraphql.Query{
		Query: `mutation ConvertPullRequestToDraft($input:ConvertPullRequestToDraftInput!) { convertPullRequestToDraft(input: $input) { clientMutationId } }`,
		Variables: map[string]any{
			"input": input,
		},
	}

	// query := graphqlQuery{
	// 	Query: `query ($repoOwner: String!, $repoName: String!, $prNumber: Int!) { repository(owner: $repoOwner, name: $repoName) { pullRequest(number: $prNumber) {  id title isDraft }  } }`,
	// 	Variables: map[string]any{
	// 		"repoOwner": "kptdev",
	// 		"repoName":  "kpt",
	// 		"prNumber":  pr.GetNumber(),
	// 	},
	// }

	// query := graphqlQuery{
	// 	Query: `query ($id: ID!) { node(id: $id) { id } }`,
	// 	Variables: map[string]any{
	// 		"id": pr.GetNodeID(),
	// 	},
	// }

	out := make(map[string]any)
	if err := query.Run(ctx, scope, out); err != nil {
		return fmt.Errorf("calling convertPullRequestToDraft: %w", err)
	}

	return nil
}

func (r *EventHandler) addLabels(ctx context.Context, scope *events.Scope, pr *events.PullRequest, wantLabels []string) error {

	if pr.Merged {
		// TODO: post to issue
		klog.Warningf("cannot update merged pull request")
		return nil
	}

	var labelsToAdd []string
	for _, wantLabel := range wantLabels {
		found := false
		for _, label := range pr.Labels {
			if label == wantLabel {
				found = true
			}
		}
		if !found {
			labelsToAdd = append(labelsToAdd, wantLabel)
		}
	}
	if len(labelsToAdd) == 0 {
		return nil
	}

	klog.Infof("adding labels: %+v", labelsToAdd)

	if _, _, err := scope.Github.Issues.AddLabelsToIssue(ctx, scope.Owner, scope.Repo, pr.Number, labelsToAdd); err != nil {
		return fmt.Errorf("adding labels %v: %w", labelsToAdd, err)
	}

	return nil
}

func (r *EventHandler) removeLabels(ctx context.Context, scope *events.Scope, pr *events.PullRequest, labels []string) error {
	// owner := ev.GetRepo().GetOwner().GetName()
	// repo := ev.GetRepo().GetName()

	if pr.Merged {
		// TODO: post to issue
		klog.Warningf("cannot update merged pull request")
		return nil
	}

	var labelsThatExist []string
	for _, removeLabel := range labels {
		for _, label := range pr.Labels {
			if label == removeLabel {
				labelsThatExist = append(labelsThatExist, label)
				continue
			}
		}
	}
	if len(labelsThatExist) == 0 {
		return nil
	}

	klog.Infof("removing labels: %+v", labelsThatExist)

	for _, label := range labelsThatExist {
		if _, err := scope.Github.Issues.RemoveLabelForIssue(ctx, scope.Owner, scope.Repo, pr.Number, label); err != nil {
			return fmt.Errorf("removing label %q: %w", label, err)
		}
	}

	return nil
}
