package forge

import (
	"context"
	"fmt"

	"github.com/google/go-github/v53/github"
	"k8s.io/klog/v2"
)

type PullRequest struct {
	repo     *Repo
	prNumber int

	githubClient *github.Client
	obj          *github.PullRequest
}

func (o *PullRequest) AddLabels(ctx context.Context, wantLabels []string) error {
	if o.obj.GetMerged() {
		// TODO: post to issue
		klog.Warningf("cannot update merged pull request %v", o.prNumber)
		return nil
	}

	var labelsToAdd []string
	for _, wantLabel := range wantLabels {
		found := false
		for _, label := range o.obj.Labels {
			if label.GetName() == wantLabel {
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

	if _, _, err := o.githubClient.Issues.AddLabelsToIssue(ctx, o.repo.owner, o.repo.repoName, o.prNumber, labelsToAdd); err != nil {
		return fmt.Errorf("adding labels %v: %w", labelsToAdd, err)
	}

	return nil
}

func (o *PullRequest) RemoveLabels(ctx context.Context, labels []string) error {
	// owner := ev.GetRepo().GetOwner().GetName()
	// repo := ev.GetRepo().GetName()

	if o.obj.GetMerged() {
		// TODO: post to issue
		klog.Warningf("cannot update merged pull request %v", o.prNumber)
		return nil
	}

	var labelsThatExist []string
	for _, removeLabel := range labels {
		for _, label := range o.obj.Labels {
			if label.GetName() == removeLabel {
				labelsThatExist = append(labelsThatExist, label.GetName())
				continue
			}
		}
	}
	if len(labelsThatExist) == 0 {
		return nil
	}

	klog.Infof("removing labels: %+v", labelsThatExist)

	for _, label := range labelsThatExist {
		if _, err := o.githubClient.Issues.RemoveLabelForIssue(ctx, o.repo.owner, o.repo.repoName, o.prNumber, label); err != nil {
			return fmt.Errorf("removing label %q: %w", label, err)
		}
	}

	return nil
}

func (o *PullRequest) Merge(ctx context.Context, mergeMethod string) error {
	if o.obj.GetMerged() {
		klog.Warningf("pr already merged; cannot merge")
		return nil
	}

	klog.Infof("merging pr: %v", o.prNumber)

	options := &github.PullRequestOptions{MergeMethod: mergeMethod}
	if _, _, err := o.githubClient.PullRequests.Merge(ctx, o.repo.owner, o.repo.repoName, o.prNumber, "", options); err != nil {
		return fmt.Errorf("merging pull request: %w", err)
	}

	return nil
}

func (o *PullRequest) Close(ctx context.Context) error {
	if o.obj.GetMerged() {
		klog.Warningf("pr already merged; cannot close")
		return nil
	}

	klog.Infof("merging pr: %v", o.prNumber)

	options := &github.IssueRequest{State: PtrTo("closed")}
	if _, _, err := o.githubClient.Issues.Edit(ctx, o.repo.owner, o.repo.repoName, o.prNumber, options); err != nil {
		return fmt.Errorf("closing issue: %w", err)
	}
	return nil
}

func PtrTo[T any](t T) *T {
	return &t
}

func (o *PullRequest) Retest(ctx context.Context, checkSuiteID int64) error {
	workflowRuns, _, err := o.githubClient.Actions.ListRepositoryWorkflowRuns(ctx, o.repo.owner, o.repo.repoName, &github.ListWorkflowRunsOptions{
		CheckSuiteID: checkSuiteID,
	})
	// result, err := appInstall.GithubClient().Checks.ReRequestCheckRun(ctx, owner, repo, checkRunID)
	if err != nil {
		return fmt.Errorf("listing working runs: %w", err)
	}
	for _, run := range workflowRuns.WorkflowRuns {
		klog.Infof("workflow runs %+v", run)
	}

	if len(workflowRuns.WorkflowRuns) != 1 {
		klog.Fatalf("unexpected number of workflow runs: %v", len(workflowRuns.WorkflowRuns))
	}
	workflowRun := workflowRuns.WorkflowRuns[0]
	result, err := o.githubClient.Actions.RerunFailedJobsByID(ctx, o.repo.owner, o.repo.repoName, workflowRun.GetID())
	// result, err := appInstall.GithubClient().Checks.ReRequestCheckRun(ctx, owner, repo, checkRunID)
	if err != nil {
		return fmt.Errorf("requesting check run: %w", err)
	} else {
		klog.Infof("re-requested check run %+v", result)
	}

	return nil
}
