package forge

import (
	"context"

	"github.com/GoogleContainerTools/kpt/tools/github-actions/pkg/appinstall"
	"github.com/google/go-github/v53/github"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func SyncObjectFromGithub(ctx context.Context, kubeClient client.Client, repo *Repo, prNumber int) error {
	syncer := &appinstall.ObjectSyncer{
		GithubClient: repo.githubClient,
		KubeClient:   kubeClient,
	}

	return syncer.SyncObjectFromGithub(ctx, repo.owner, repo.repoName, prNumber)
}

// func SyncObjectFromGithub(ctx context.Context, k8s client.Client, repo *Repo, prNumber int) error {
// 	pullRequest, err := repo.PullRequest(ctx, prNumber)
// 	if err != nil {
// 		return fmt.Errorf("getting pr: %w", err)
// 	}
// 	prObj := pullRequest.obj

// 	githubClient := repo.githubClient

// 	labels, _, err := githubClient.Issues.ListLabelsByIssue(ctx, repo.owner, repo.repoName, prNumber, &github.ListOptions{})
// 	if err != nil {
// 		return fmt.Errorf("getting issue labels %s/%s %v: %w", repo.owner, repo.repoName, prNumber, err)
// 	}

// 	klog.Infof("PR %v", prNumber)

// 	listCommentsOpts := &github.IssueListCommentsOptions{
// 		Sort:      PtrTo("updated"),
// 		Direction: PtrTo("desc"),
// 		// TODO: Since?
// 	}
// 	listCommentsOpts.PerPage = 100
// 	comments, _, err := githubClient.Issues.ListComments(ctx, repo.owner, repo.repoName, prNumber, listCommentsOpts)
// 	if err != nil {
// 		return fmt.Errorf("listing comments for pr %s/%s %v: %w", repo.owner, repo.repoName, prNumber, err)
// 	}

// 	id := types.NamespacedName{Namespace: repo.repoName, Name: fmt.Sprintf("github-%d", prObj.GetNumber())}
// 	u := &unstructured.Unstructured{}
// 	u.SetName(id.Name)
// 	u.SetNamespace(id.Namespace)
// 	u.SetGroupVersionKind(v1alpha1.PullRequestGVK)
// 	spec := map[string]any{}
// 	spec["title"] = prObj.GetTitle()
// 	spec["url"] = prObj.GetHTMLURL()
// 	spec["state"] = prObj.GetState()
// 	spec["author"] = prObj.GetUser().GetLogin()
// 	spec["body"] = prObj.GetBody()
// 	spec["createdAt"] = asTime(prObj.GetCreatedAt())
// 	spec["updatedAt"] = asTime(prObj.GetUpdatedAt())

// 	specLabels := []v1alpha1.Label{} // empty array to keep CRDs happy
// 	for _, label := range labels {
// 		out := v1alpha1.Label{
// 			Name: label.GetName(),
// 			// CreatedAt: asTime(label.GetCreatedAt()),
// 		}
// 		specLabels = append(specLabels, out)
// 	}
// 	spec["labels"] = specLabels

// 	specBase := v1alpha1.BaseRef{}
// 	specBase.Ref = prObj.GetBase().GetRef()
// 	specBase.SHA = prObj.GetBase().GetSHA()
// 	for _, label := range labels {
// 		out := v1alpha1.Label{
// 			Name: label.GetName(),
// 			// CreatedAt: asTime(label.GetCreatedAt()),
// 		}
// 		specLabels = append(specLabels, out)
// 	}
// 	spec["base"] = specBase

// 	specComments := []v1alpha1.Comment{} // empty array to keep CRDs happy
// 	for _, comment := range comments {
// 		out := v1alpha1.Comment{
// 			Body:      comment.GetBody(),
// 			Author:    comment.GetUser().GetLogin(),
// 			CreatedAt: asTime(comment.GetCreatedAt()),
// 		}
// 		specComments = append(specComments, out)
// 	}
// 	spec["comments"] = specComments

// 	ref := prObj.GetHead().GetSHA()

// 	checks, _, err := githubClient.Checks.ListCheckRunsForRef(ctx, repo.owner, repo.repoName, ref, &github.ListCheckRunsOptions{})
// 	if err != nil {
// 		return fmt.Errorf("getting checks %s/%s %v: %w", repo.owner, repo.repoName, ref, err)
// 	}

// 	checkSuites := []*v1alpha1.CheckSuite{} // empty array to keep CRDs happy
// 	checkSuiteMap := make(map[int64]*v1alpha1.CheckSuite)

// 	for _, checkRun := range checks.CheckRuns {
// 		checkSuiteID := checkRun.GetCheckSuite().GetID()
// 		checkSuite := checkSuiteMap[checkSuiteID]
// 		if checkSuite == nil {
// 			checkSuite = &v1alpha1.CheckSuite{
// 				ID: checkSuiteID,
// 			}
// 			checkSuiteMap[checkSuiteID] = checkSuite
// 			checkSuites = append(checkSuites, checkSuite)
// 		}
// 		out := v1alpha1.Check{
// 			Name:        checkRun.GetName(),
// 			Status:      checkRun.GetStatus(),
// 			Conclusion:  checkRun.GetConclusion(),
// 			CompletedAt: asOptionalTime(checkRun.CompletedAt),
// 			ID:          checkRun.GetID(),
// 		}
// 		checkSuite.Checks = append(checkSuite.Checks, out)
// 	}
// 	spec["checkSuites"] = checkSuites

// 	u.Object["spec"] = spec

// 	if err := k8s.Patch(ctx, u, client.Apply, &client.PatchOptions{
// 		FieldManager: "github-syncer",
// 	}); err != nil {
// 		return fmt.Errorf("updating PullRequest: %w", err)
// 	}

// 	return nil
// }

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
