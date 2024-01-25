package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GoogleContainerTools/kpt/tools/github-actions/api/v1alpha1"
	"github.com/GoogleContainerTools/kpt/tools/github-actions/pkg/appinstall"
	"github.com/GoogleContainerTools/kpt/tools/github-actions/pkg/forge"
	"github.com/GoogleContainerTools/kpt/tools/github-actions/pkg/jwt"
	github "github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const LabelHold = "do-not-merge/hold"
const LabelLGTM = "lgtm"
const LabelApproved = "approved"

func main() {
	err := run(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	appSecretPath := ""
	flag.StringVar(&appSecretPath, "app-secret", appSecretPath, "path to application secret, if running as a github app")
	flag.Parse()

	restConfig, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("getting kubernetes configuration: %w", err)
	}
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("adding to scheme: %w", err)
	}
	clientOptions := client.Options{
		Scheme: scheme,
	}
	k8s, err := client.New(restConfig, clientOptions)
	if err != nil {
		return fmt.Errorf("building kubernetes client: %w", err)
	}
	var githubTokenSource oauth2.TokenSource
	if appSecretPath != "" {
		// export APP_ID=$(cat /etc/github/app-id)
		appID := os.Getenv("APP_ID")
		if appID == "" {
			return fmt.Errorf("APP_ID is not set")
		}

		tokenSource, err := jwt.NewJWTAccessTokenSource(appSecretPath, appID)
		if err != nil {
			return fmt.Errorf("creating jwt access token source: %w", err)
		}
		githubTokenSource = tokenSource
	} else {
		token := os.Getenv("GITHUB_TOKEN")
		if token == "" {
			return fmt.Errorf("GITHUB_TOKEN is not set")
		}
		githubTokenSource = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	}

	appGithubClient := github.NewClient(oauth2.NewClient(ctx, githubTokenSource))

	installations, _, err := appGithubClient.Apps.ListInstallations(ctx, &github.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing installations: %w", err)
	}
	klog.Infof("installations: %+v", installations)

	var appInstalls []*appinstall.Installation

	for _, installation := range installations {
		if installation.GetAccount().GetType() != "Organization" {
			klog.Infof("skipping installation into non-organization: %+v", installation.GetAccount())
			continue
		}
		org := installation.GetAccount().GetLogin()
		if org == "" {
			return fmt.Errorf("cannot determine org from installation %+v", installation)
		}
		klog.Infof("org: %+v", org)

		org = "kptdev"

		installationID := installation.GetID()

		appInstall, err := appinstall.NewInstallation(ctx, k8s, appGithubClient, installationID, org)
		if err != nil {
			return err
		}

		// lgtm := &lgtm.EventHandler{}
		// installScope.AddEventHandler(lgtm)

		// retest := &retest.RetestHandler{}
		// appInstall.AddEventHandler(retest)

		appInstalls = append(appInstalls, appInstall)
	}

	for {
		for _, appInstall := range appInstalls {
			repo := forge.NewRepo(appInstall.GithubClient(), "kptdev", "kpt")

			// The events are very slow, sadly
			// if _, err := installScope.PollOrgEventsOnce(ctx); err != nil {
			// 	return fmt.Errorf("polling events for org %q: %w", org, err)
			// }

			// if _, err := installScope.PollRepoEventsOnce(ctx, "kptdev", "kpt"); err != nil {
			// 	return fmt.Errorf("polling events for org %q: %w", org, err)
			// }
			klog.Infof("appInstall %+v", appInstall)

			var prList v1alpha1.PullRequestList
			if err := k8s.List(ctx, &prList); err != nil {
				return fmt.Errorf("listing pull requests: %w", err)
			}

			if err := runHoldRobot(ctx, repo, prList.Items); err != nil {
				return err
			}

			if err := runLGTMRobot(ctx, repo, prList.Items); err != nil {
				return err
			}
			if err := runApprovedRobot(ctx, repo, prList.Items); err != nil {
				return err
			}

			if err := runMergeRobot(ctx, repo, prList.Items); err != nil {
				return err
			}

			if err := runCloseRobot(ctx, appInstall.GithubClient(), prList.Items); err != nil {
				return err
			}

			if err := runRetestRobot(ctx, appInstall.GithubClient(), prList.Items); err != nil {
				return err
			}
		}
		klog.Infof("completed poll; will poll again in a minute")
		time.Sleep(1 * time.Minute)
	}

	return nil
}

func runCloseRobot(ctx context.Context, githubClient *github.Client, pullRequests []v1alpha1.PullRequest) error {
	owner := "kptdev"
	repo := "kpt"

	for _, pr := range pullRequests {
		var closeAt metav1.Time
		for _, comment := range pr.Spec.Comments {
			for _, line := range strings.Split(comment.Body, "\n") {
				line = strings.TrimSpace(line)
				line += " "
				if strings.HasPrefix(line, "/close ") {
					closeAt = comment.CreatedAt
				}
			}
		}

		if closeAt.IsZero() {
			continue
		}

		if pr.Spec.State == "closed" {
			continue
		}

		// TODO: What about reopen etc?  Maybe check timeline or closedAt?

		klog.Infof("closing issue %v", pr.Name)
		prNumber, err := strconv.Atoi(strings.TrimPrefix(pr.Name, "github-"))
		if err != nil {
			return fmt.Errorf("parsing name %q", pr.Name)
		}
		if _, _, err := githubClient.Issues.Edit(ctx, owner, repo, prNumber, &github.IssueRequest{State: PtrTo("closed")}); err != nil {
			return fmt.Errorf("updating issue: %w", err)
		}
	}
	return nil
}

func PtrTo[T any](t T) *T {
	return &t
}

func runRetestRobot(ctx context.Context, githubClient *github.Client, pullRequests []v1alpha1.PullRequest) error {
	owner := "kptdev"
	repo := "kpt"

	for _, pr := range pullRequests {
		var retestAt metav1.Time
		for _, comment := range pr.Spec.Comments {
			for _, line := range strings.Split(comment.Body, "\n") {
				line += " "
				if strings.HasPrefix(line, "/retest ") {
					retestAt = comment.CreatedAt
				}
			}
		}

		if retestAt.IsZero() {
			continue
		}

		for _, checkSuite := range pr.Spec.CheckSuites {
			retest := false
			for _, checkRun := range checkSuite.Checks {
				if checkRun.Status != "completed" || checkRun.Conclusion != "failure" {
					continue
				}
				if checkRun.CompletedAt != nil && retestAt.After(checkRun.CompletedAt.Time) {
					klog.Infof("retest requested for pr %v on check %q", pr.GetName(), checkRun.Name)
					retest = true

					// checkRunID := checkRun.ID

					workflowRuns, _, err := githubClient.Actions.ListRepositoryWorkflowRuns(ctx, owner, repo, &github.ListWorkflowRunsOptions{
						CheckSuiteID: checkSuite.ID,
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
					result, err := githubClient.Actions.RerunFailedJobsByID(ctx, owner, repo, workflowRun.GetID())
					// result, err := appInstall.GithubClient().Checks.ReRequestCheckRun(ctx, owner, repo, checkRunID)
					if err != nil {
						klog.Warningf("requesting check re-run: %v", err)
						// return fmt.Errorf("requesting check run: %w", err)
					} else {
						klog.Infof("re-requested check run %+v", result)
					}
				}
			}

			if retest {

				// checkSuiteID := checkSuite.ID

				// result, err := appInstall.GithubClient().Checks.ReRequestCheckSuite(ctx, owner, repo, checkSuiteID)
				// if err != nil {
				// 	// klog.Warningf("requesting check suite re-run: %v", err)
				// 	return fmt.Errorf("requesting check suite re-run: %w", err)
				// }
				// klog.Infof("re-requested check run %+v", result)

				// result, err := appInstall.GithubClient().Actions.RerunFailedJobsByID(ctx, ReRequestCheckSuite(ctx, owner, repo, checkRunID)
				// if err != nil {
				// 	// klog.Warningf("requesting check suite re-run: %v", err)
				// 	return fmt.Errorf("requesting check suite re-run: %w", err)
				// }
				// klog.Infof("re-requested check run %+v", result)

			}
		}
	}

	return nil
}

func runHoldRobot(ctx context.Context, repo *forge.Repo, pullRequests []v1alpha1.PullRequest) error {
	for i := range pullRequests {
		obj := &pullRequests[i]

		if !isOpen(obj) {
			// May not matter
			continue
		}

		r := ThreadReader{
			ApplyCommands:  []string{"/hold"},
			CancelCommands: []string{"/hold cancel", "/unhold", "/remove-hold"},
		}

		applyAt, cancelAt, err := r.ParseForCommands(ctx, repo, obj)
		if err != nil {
			return err
		}
		if applyAt == nil && cancelAt == nil {
			continue
		}

		if applyAt != nil {
			if err := addLabels(ctx, repo, obj, []string{LabelHold}); err != nil {
				return fmt.Errorf("adding hold label: %w", err)
			}
		} else if cancelAt != nil {
			if err := removeLabels(ctx, repo, obj, []string{LabelHold}); err != nil {
				return fmt.Errorf("adding hold label: %w", err)
			}
		}
	}

	return nil
}

func isApprover(ctx context.Context, repo *forge.Repo, baseRef string, userName string) (bool, error) {
	codeOwners, err := repo.CodeOwners(ctx, baseRef)
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

// func canApprove(ctx context.Context, repo *forge.Repo, baseRef string, userName string) (bool, error) {
// 	codeOwners, err := repo.CodeOwners(ctx, baseRef)
// 	if err != nil {
// 		return false, err
// 	}

// 	for _, owner := range codeOwners.Owners {
// 		if owner == userName {
// 			return true, nil
// 		}
// 	}
// 	return false, nil
// }

func runLGTMRobot(ctx context.Context, repo *forge.Repo, pullRequests []v1alpha1.PullRequest) error {
	for i := range pullRequests {
		obj := &pullRequests[i]

		if !isOpen(obj) {
			continue
		}

		r := ThreadReader{
			ApplyCommands:  []string{"/lgtm"},
			CancelCommands: []string{"/lgtm cancel"},
		}
		r.AddPermissionCheck(requireCodeOwnerPermission(repo, obj, "approve"))
		r.AddPermissionCheck(noSelfApproval(repo, obj, "approve"))

		applyAt, cancelAt, err := r.ParseForCommands(ctx, repo, obj)
		if err != nil {
			return err
		}
		if applyAt == nil && cancelAt == nil {
			continue
		}

		if applyAt != nil {
			if !hasLabel(obj, LabelLGTM) {
				if err := addLabels(ctx, repo, obj, []string{LabelLGTM}); err != nil {
					return fmt.Errorf("updating lgtm: %w", err)
				}
			}
		} else if cancelAt == nil {
			if hasLabel(obj, LabelLGTM) {
				if err := removeLabels(ctx, repo, obj, []string{LabelLGTM}); err != nil {
					return fmt.Errorf("updating lgtm: %w", err)
				}
			}
		}
	}

	return nil
}

type PermissionCheckFunction func(ctx context.Context, comment *v1alpha1.Comment) (bool, error)

type ThreadReader struct {
	ApplyCommands  []string
	CancelCommands []string

	permissionChecks []PermissionCheckFunction
}

func (r *ThreadReader) AddPermissionCheck(permissionCheck PermissionCheckFunction) {
	r.permissionChecks = append(r.permissionChecks, permissionCheck)
}

func (r *ThreadReader) ParseForCommands(ctx context.Context, repo *forge.Repo, obj *v1alpha1.PullRequest) (*v1alpha1.Comment, *v1alpha1.Comment, error) {
	var applyAt *v1alpha1.Comment
	var cancelAt *v1alpha1.Comment
	for i := range obj.Spec.Comments {
		comment := &obj.Spec.Comments[i]

		vote := 0
		for _, line := range strings.Split(comment.Body, "\n") {
			line = strings.TrimSpace(line)
			line += " "
			for _, applyCommand := range r.ApplyCommands {
				if strings.HasPrefix(line, applyCommand+" ") {
					vote = 1
				}
			}

			for _, cancelCommand := range r.CancelCommands {
				if strings.HasPrefix(line, cancelCommand+" ") {
					vote = -1
				}
			}
		}

		if vote == 0 {
			continue
		}

		for _, permissionCheck := range r.permissionChecks {
			hasPermission, err := permissionCheck(ctx, comment)
			if err != nil {
				return nil, nil, err
			}
			if !hasPermission {
				vote = 0
				continue
			}
		}

		if vote == 1 {
			applyAt = comment
			cancelAt = nil
		}
		if vote == -1 {
			applyAt = nil
			cancelAt = comment
		}

	}

	return applyAt, cancelAt, nil
}

func requireCodeOwnerPermission(repo *forge.Repo, obj *v1alpha1.PullRequest, verb string) PermissionCheckFunction {
	return func(ctx context.Context, comment *v1alpha1.Comment) (bool, error) {
		if obj.Spec.Base == nil {
			return false, fmt.Errorf("base not populated")
		}
		baseRef := obj.Spec.Base.SHA

		isApprover, err := isApprover(ctx, repo, baseRef, comment.Author)
		if err != nil {
			return false, err
		}

		if !isApprover {
			// TODO: post message?
			klog.Warningf("user %q cannot %q PRs", verb, comment.Author)
		}

		return isApprover, nil
	}
}

func noSelfApproval(repo *forge.Repo, obj *v1alpha1.PullRequest, verb string) PermissionCheckFunction {
	return func(ctx context.Context, comment *v1alpha1.Comment) (bool, error) {

		if comment.Author == obj.Spec.Author {
			// TODO: post message?
			klog.Warningf("user %q cannot %q their own PR", verb, comment.Author)
			return false, nil
		}

		return true, nil
	}
}

func runApprovedRobot(ctx context.Context, repo *forge.Repo, pullRequests []v1alpha1.PullRequest) error {
	for i := range pullRequests {
		obj := &pullRequests[i]

		if !isOpen(obj) {
			continue
		}

		r := ThreadReader{
			ApplyCommands:  []string{"/approve"},
			CancelCommands: []string{"/remove-approve"},
		}
		r.AddPermissionCheck(requireCodeOwnerPermission(repo, obj, "approve"))
		r.AddPermissionCheck(noSelfApproval(repo, obj, "approve"))

		applyAt, cancelAt, err := r.ParseForCommands(ctx, repo, obj)
		if err != nil {
			return err
		}
		if applyAt == nil && cancelAt == nil {
			continue
		}

		if applyAt != nil {
			if err := addLabels(ctx, repo, obj, []string{LabelApproved}); err != nil {
				return fmt.Errorf("updating approved: %w", err)
			}
		} else if cancelAt == nil {

			if err := removeLabels(ctx, repo, obj, []string{LabelApproved}); err != nil {
				return fmt.Errorf("updating approved: %w", err)
			}
		}
	}

	return nil
}

func addLabels(ctx context.Context, repo *forge.Repo, obj *v1alpha1.PullRequest, labels []string) error {
	hasAllLabels := true
	for _, label := range labels {
		if !hasLabel(obj, label) {
			hasAllLabels = false
		}
	}
	if hasAllLabels {
		return nil
	}

	prNumber, err := strconv.Atoi(strings.TrimPrefix(obj.Name, "github-"))
	if err != nil {
		return fmt.Errorf("parsing name %q", obj.Name)
	}

	pr, err := repo.PullRequest(ctx, prNumber)
	if err != nil {
		return fmt.Errorf("getting pull request: %w", err)
	}

	if err := pr.AddLabels(ctx, labels); err != nil {
		return fmt.Errorf("updating labels: %w", err)
	}
	return nil
}

func removeLabels(ctx context.Context, repo *forge.Repo, obj *v1alpha1.PullRequest, labels []string) error {
	hasAnyLabel := false
	for _, label := range labels {
		if hasLabel(obj, label) {
			hasAnyLabel = true
		}
	}
	if !hasAnyLabel {
		return nil
	}
	prNumber, err := strconv.Atoi(strings.TrimPrefix(obj.Name, "github-"))
	if err != nil {
		return fmt.Errorf("parsing name %q", obj.Name)
	}

	pr, err := repo.PullRequest(ctx, prNumber)
	if err != nil {
		return fmt.Errorf("getting pull request: %w", err)
	}

	if err := pr.RemoveLabels(ctx, labels); err != nil {
		return fmt.Errorf("updating labels: %w", err)
	}
	return nil
}

func runMergeRobot(ctx context.Context, repo *forge.Repo, pullRequests []v1alpha1.PullRequest) error {
	for i := range pullRequests {
		obj := &pullRequests[i]

		if !isOpen(obj) {
			continue
		}

		prNumber, err := strconv.Atoi(strings.TrimPrefix(obj.Name, "github-"))
		if err != nil {
			return fmt.Errorf("parsing name %q", obj.Name)
		}

		hasLGTM := hasLabel(obj, LabelLGTM)
		hasApproved := hasLabel(obj, LabelApproved)
		hasHold := hasLabel(obj, LabelHold)

		if hasHold || !hasLGTM || !hasApproved {
			continue
		}

		allTestsPassing := true
		testCount := 0
		for _, checkSuites := range obj.Spec.CheckSuites {
			for _, check := range checkSuites.Checks {
				testCount++
				switch check.Conclusion {
				case "success":
				default:
					allTestsPassing = false
				}
			}
		}

		if testCount == 0 {
			klog.Warningf("pr %v has lgtm/approved, but no check results", prNumber)
			continue
		}

		if !allTestsPassing {
			continue
		}

		pr, err := repo.PullRequest(ctx, prNumber)
		if err != nil {
			return fmt.Errorf("getting pull request: %w", err)
		}
		if err := pr.Merge(ctx, "merge"); err != nil {
			return fmt.Errorf("merging pr: %w", err)
		}
	}

	return nil
}

func hasLabel(pr *v1alpha1.PullRequest, findLabel string) bool {
	for _, label := range pr.Spec.Labels {
		if label.Name == findLabel {
			return true
		}
	}
	return false
}

func isOpen(pr *v1alpha1.PullRequest) bool {
	return pr.Spec.State == "open"
}
