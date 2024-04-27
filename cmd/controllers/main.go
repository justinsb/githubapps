package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GoogleContainerTools/kpt/tools/github-actions/api/v1alpha1"
	"github.com/GoogleContainerTools/kpt/tools/github-actions/pkg/appinstall"
	"github.com/GoogleContainerTools/kpt/tools/github-actions/pkg/forge"
	"github.com/GoogleContainerTools/kpt/tools/github-actions/pkg/jwt"
	github "github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
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

			for i := range prList.Items {
				obj := &prList.Items[i]
				op := &Op{
					Repo:        repo,
					PullRequest: obj,
				}

				// klog.Infof("running reconcile for %v", obj.GetName())
				if err := runAllReconcilers(ctx, k8s, op); err != nil {
					klog.Warningf("error reconciling %v: %v", obj.GetName(), err)
				}
			}
		}
		klog.Infof("completed poll; will poll again in a minute")
		time.Sleep(1 * time.Minute)
	}

	return nil
}

func runAllReconcilers(ctx context.Context, k8s client.Client, op *Op) error {
	id := types.NamespacedName{Namespace: op.PullRequest.GetNamespace(), Name: op.PullRequest.GetName()}

	var reconcilers []func(ctx context.Context, op *Op) error

	reconcilers = append(reconcilers, runRetestRobot)
	reconcilers = append(reconcilers, runHoldRobot)
	reconcilers = append(reconcilers, runLGTMRobot)
	reconcilers = append(reconcilers, runApprovedRobot)
	reconcilers = append(reconcilers, runCloseRobot)
	reconcilers = append(reconcilers, runMergeRobot)
	reconcilers = append(reconcilers, runAssignRobot)

	for {
		op.Changed = false

		for _, reconciler := range reconcilers {
			if err := reconciler(ctx, op); err != nil {
				return err
			}
			if op.Changed {
				break
			}
		}

		if !op.Changed {
			break
		}

		prNumber, err := strconv.Atoi(strings.TrimPrefix(op.PullRequest.Name, "github-"))
		if err != nil {
			return fmt.Errorf("parsing name %q", op.PullRequest.Name)
		}

		if err := forge.SyncObjectFromGithub(ctx, k8s, op.Repo, prNumber); err != nil {
			return err
		}

		updated := &v1alpha1.PullRequest{}
		if err := k8s.Get(ctx, id, updated); err != nil {
			return fmt.Errorf("getting updated object: %w", err)
		}
		j, _ := json.Marshal(updated)
		klog.Infof("updated object is %v", string(j))
		op.PullRequest = updated
	}

	return nil
}

func runCloseRobot(ctx context.Context, op *Op) error {
	pullRequest := op.PullRequest

	if !isOpen(pullRequest) {
		return nil
	}

	r := ThreadReader{
		ApplyCommands: []string{"/close"},
	}

	applyAt, cancelAt, err := r.ParseForCommands(ctx, op.Repo, pullRequest)
	if err != nil {
		return err
	}
	if applyAt == nil && cancelAt == nil {
		return err
	}

	// TODO: What about reopen etc?  Maybe check timeline or closedAt?

	klog.Infof("closing issue %v", pullRequest.Name)
	if err := op.ClosePullRequest(ctx); err != nil {
		return err
	}

	return nil
}

func PtrTo[T any](t T) *T {
	return &t
}

func runRetestRobot(ctx context.Context, op *Op) error {
	pullRequest := op.PullRequest
	if !isOpen(pullRequest) {
		return nil
	}

	r := ThreadReader{
		ApplyCommands: []string{"/retest"},
	}

	applyAt, cancelAt, err := r.ParseForCommands(ctx, op.Repo, pullRequest)
	if err != nil {
		return err
	}
	if applyAt == nil && cancelAt == nil {
		return err
	}

	if applyAt != nil {
		for _, checkSuite := range pullRequest.Spec.CheckSuites {
			for _, checkRun := range checkSuite.Checks {
				if checkRun.Status != "completed" || checkRun.Conclusion != "failure" {
					continue
				}
				if checkRun.CompletedAt != nil && applyAt.CreatedAt.After(checkRun.CompletedAt.Time) {
					klog.Infof("retest requested for pr %v on check %q", pullRequest.GetName(), checkRun.Name)

					if err := op.Retest(ctx, checkSuite.ID); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

type Op struct {
	Repo        *forge.Repo
	PullRequest *v1alpha1.PullRequest
	Changed     bool
}

func runHoldRobot(ctx context.Context, op *Op) error {
	pullRequest := op.PullRequest

	if !isOpen(pullRequest) {
		// It may be OK to apply labels to old PRs... IDK
		return nil
	}

	r := ThreadReader{
		ApplyCommands:  []string{"/hold"},
		CancelCommands: []string{"/hold cancel", "/unhold", "/remove-hold"},
	}

	applyAt, cancelAt, err := r.ParseForCommands(ctx, op.Repo, pullRequest)
	if err != nil {
		return err
	}
	if applyAt == nil && cancelAt == nil {
		return err
	}

	if applyAt != nil {
		if err := op.AddLabels(ctx, []string{LabelHold}); err != nil {
			return fmt.Errorf("adding hold label: %w", err)
		}
	} else if cancelAt != nil {
		if err := op.RemoveLabels(ctx, []string{LabelHold}); err != nil {
			return fmt.Errorf("adding hold label: %w", err)
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

func runLGTMRobot(ctx context.Context, op *Op) error {

	pullRequest := op.PullRequest

	if !isOpen(pullRequest) {
		return nil
	}

	r := ThreadReader{
		ApplyCommands:  []string{"/lgtm"},
		CancelCommands: []string{"/lgtm cancel"},
	}
	r.AddPermissionCheck(op.requireCodeOwnerPermission("lgtm"))
	r.AddPermissionCheck(op.noSelfApproval("lgtm"))

	applyAt, cancelAt, err := r.ParseForCommands(ctx, op.Repo, pullRequest)
	if err != nil {
		return err
	}
	if applyAt == nil && cancelAt == nil {
		return nil
	}

	if applyAt != nil {
		if err := op.AddLabels(ctx, []string{LabelLGTM}); err != nil {
			return fmt.Errorf("updating lgtm: %w", err)
		}
	} else if cancelAt == nil {
		if err := op.RemoveLabels(ctx, []string{LabelLGTM}); err != nil {
			return fmt.Errorf("updating lgtm: %w", err)
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

type ThreadScanner struct {
	matchers []*matcher
}

type matcher struct {
	regex    *regexp.Regexp
	callback func(comment *v1alpha1.Comment, line string, matches []string)
}

func (r *ThreadScanner) AddMatcher(regex string, callback func(comment *v1alpha1.Comment, line string, matches []string)) {
	matcher := &matcher{
		regex:    regexp.MustCompile(regex),
		callback: callback,
	}
	r.matchers = append(r.matchers, matcher)
}

func (r *ThreadScanner) MatchCommands(ctx context.Context, repo *forge.Repo, obj *v1alpha1.PullRequest) error {
	for i := range obj.Spec.Comments {
		comment := &obj.Spec.Comments[i]

		for _, line := range strings.Split(comment.Body, "\n") {
			line = strings.TrimSpace(line)
			for _, matcher := range r.matchers {
				if matches := matcher.regex.FindStringSubmatch(line); matches != nil {
					matcher.callback(comment, line, matches)
				}
			}
		}
	}

	return nil
}

func (op *Op) requireCodeOwnerPermission(verb string) PermissionCheckFunction {
	return func(ctx context.Context, comment *v1alpha1.Comment) (bool, error) {
		if op.PullRequest.Spec.Base == nil {
			return false, fmt.Errorf("base not populated")
		}
		baseRef := op.PullRequest.Spec.Base.SHA

		isApprover, err := isApprover(ctx, op.Repo, baseRef, comment.Author)
		if err != nil {
			return false, err
		}

		if !isApprover {
			// TODO: post message?
			klog.Warningf("user %q cannot %q PRs", comment.Author, verb)
		}

		return isApprover, nil
	}
}

func (op *Op) noSelfApproval(verb string) PermissionCheckFunction {
	return func(ctx context.Context, comment *v1alpha1.Comment) (bool, error) {
		if comment.Author == op.PullRequest.Spec.Author {
			// TODO: post message?
			klog.Warningf("user %q cannot %q their own PR", comment.Author, verb)
			return false, nil
		}

		return true, nil
	}
}

func runApprovedRobot(ctx context.Context, op *Op) error {
	pullRequest := op.PullRequest

	if !isOpen(pullRequest) {
		return nil
	}

	r := ThreadReader{
		ApplyCommands:  []string{"/approve"},
		CancelCommands: []string{"/remove-approve"},
	}
	r.AddPermissionCheck(op.requireCodeOwnerPermission("approve"))
	r.AddPermissionCheck(op.noSelfApproval("approve"))

	applyAt, cancelAt, err := r.ParseForCommands(ctx, op.Repo, pullRequest)
	if err != nil {
		return err
	}
	if applyAt == nil && cancelAt == nil {
		return nil
	}

	if applyAt != nil {
		if err := op.AddLabels(ctx, []string{LabelApproved}); err != nil {
			return fmt.Errorf("updating approved: %w", err)
		}
	} else if cancelAt == nil {
		if err := op.RemoveLabels(ctx, []string{LabelApproved}); err != nil {
			return fmt.Errorf("updating approved: %w", err)
		}
	}

	return nil
}

func (op *Op) Retest(ctx context.Context, checkSuiteID int64) error {
	prNumber, err := strconv.Atoi(strings.TrimPrefix(op.PullRequest.Name, "github-"))
	if err != nil {
		return fmt.Errorf("parsing name %q", op.PullRequest.Name)
	}

	pr, err := op.Repo.PullRequest(ctx, prNumber)
	if err != nil {
		return fmt.Errorf("getting pull request: %w", err)
	}

	klog.Infof("triggering retest for PR %v", prNumber)
	if err := pr.Retest(ctx, checkSuiteID); err != nil {
		return fmt.Errorf("triggering retest: %w", err)
	}
	op.Changed = true
	return nil
}

func (op *Op) ClosePullRequest(ctx context.Context) error {
	if !isOpen(op.PullRequest) {
		return nil
	}

	prNumber, err := strconv.Atoi(strings.TrimPrefix(op.PullRequest.Name, "github-"))
	if err != nil {
		return fmt.Errorf("parsing name %q", op.PullRequest.Name)
	}

	pr, err := op.Repo.PullRequest(ctx, prNumber)
	if err != nil {
		return fmt.Errorf("getting pull request: %w", err)
	}

	klog.Infof("closing PR %v", prNumber)
	if err := pr.Close(ctx); err != nil {
		return fmt.Errorf("closing pull request: %w", err)
	}
	op.Changed = true
	return nil
}

func (op *Op) AddLabels(ctx context.Context, labels []string) error {
	hasAllLabels := true
	for _, label := range labels {
		if !hasLabel(op.PullRequest, label) {
			klog.Infof("object %+v is missing label %v", op.PullRequest, label)
			hasAllLabels = false
		}
	}
	if hasAllLabels {
		return nil
	}

	prNumber, err := strconv.Atoi(strings.TrimPrefix(op.PullRequest.Name, "github-"))
	if err != nil {
		return fmt.Errorf("parsing name %q", op.PullRequest.Name)
	}

	pr, err := op.Repo.PullRequest(ctx, prNumber)
	if err != nil {
		return fmt.Errorf("getting pull request: %w", err)
	}

	klog.Infof("adding labels %v", labels)
	if err := pr.AddLabels(ctx, labels); err != nil {
		return fmt.Errorf("updating labels: %w", err)
	}
	op.Changed = true
	return nil
}

func (op *Op) RemoveLabels(ctx context.Context, labels []string) error {
	hasAnyLabel := false
	for _, label := range labels {
		if hasLabel(op.PullRequest, label) {
			hasAnyLabel = true
		}
	}
	if !hasAnyLabel {
		return nil
	}
	prNumber, err := strconv.Atoi(strings.TrimPrefix(op.PullRequest.Name, "github-"))
	if err != nil {
		return fmt.Errorf("parsing name %q", op.PullRequest.Name)
	}

	pr, err := op.Repo.PullRequest(ctx, prNumber)
	if err != nil {
		return fmt.Errorf("getting pull request: %w", err)
	}

	klog.Infof("removing labels %v", labels)
	if err := pr.RemoveLabels(ctx, labels); err != nil {
		return fmt.Errorf("updating labels: %w", err)
	}
	op.Changed = true
	return nil
}

func runMergeRobot(ctx context.Context, op *Op) error {
	pullRequest := op.PullRequest

	if !isOpen(pullRequest) {
		return nil
	}

	prNumber, err := strconv.Atoi(strings.TrimPrefix(pullRequest.Name, "github-"))
	if err != nil {
		return fmt.Errorf("parsing name %q", pullRequest.Name)
	}

	hasLGTM := hasLabel(pullRequest, LabelLGTM)
	hasApproved := hasLabel(pullRequest, LabelApproved)
	hasHold := hasLabel(pullRequest, LabelHold)

	if hasHold || !hasLGTM || !hasApproved {
		return nil
	}

	allTestsPassing := true
	testCount := 0
	for _, checkSuites := range pullRequest.Spec.CheckSuites {
		for _, check := range checkSuites.Checks {
			testCount++
			switch check.Conclusion {
			case "success", "neutral":
			default:
				klog.Infof("test failure: %+v", check)
				allTestsPassing = false
			}
		}
	}

	if testCount == 0 {
		klog.Warningf("pr %v has lgtm/approved, but no check results", prNumber)
		return nil
	}

	if !allTestsPassing {
		return nil
	}

	pr, err := op.Repo.PullRequest(ctx, prNumber)
	if err != nil {
		return fmt.Errorf("getting pull request: %w", err)
	}
	klog.Infof("merging PR %v", prNumber)
	if err := pr.Merge(ctx, "merge"); err != nil {
		return fmt.Errorf("merging pr: %w", err)
	}
	op.Changed = true

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

func hasAssignee(pr *v1alpha1.PullRequest, findAssignee string) bool {
	for _, assignee := range pr.Spec.Assigned {
		if assignee.Name == findAssignee {
			return true
		}
	}
	return false
}

func isOpen(pr *v1alpha1.PullRequest) bool {
	return pr.Spec.State == "open"
}

func runAssignRobot(ctx context.Context, op *Op) error {
	pullRequest := op.PullRequest

	if !isOpen(pullRequest) {
		return nil
	}

	assignees := sets.New[string]()
	assign := func(comment *v1alpha1.Comment, assignee string) {
		assignees.Insert(strings.TrimPrefix(assignee, "@"))
	}
	threadScanner := &ThreadScanner{}
	threadScanner.AddMatcher("/assign (@[[:alpha:]]+)", func(comment *v1alpha1.Comment, line string, matches []string) {
		assign(comment, matches[1])
	})

	err := threadScanner.MatchCommands(ctx, op.Repo, pullRequest)
	if err != nil {
		return err
	}

	if assignees.Len() != 0 {
		if err := op.AddAssignees(ctx, assignees.UnsortedList()); err != nil {
			return fmt.Errorf("adding assignees: %w", err)
		}
	}

	return nil
}

func (op *Op) AddAssignees(ctx context.Context, assignees []string) error {
	hasAllAssignees := true
	for _, assignee := range assignees {
		if !hasAssignee(op.PullRequest, assignee) {
			klog.Infof("object %+v is missing assignee %v", op.PullRequest, assignee)
			hasAllAssignees = false
		}
	}
	if hasAllAssignees {
		return nil
	}

	prNumber, err := strconv.Atoi(strings.TrimPrefix(op.PullRequest.Name, "github-"))
	if err != nil {
		return fmt.Errorf("parsing name %q", op.PullRequest.Name)
	}

	pr, err := op.Repo.PullRequest(ctx, prNumber)
	if err != nil {
		return fmt.Errorf("getting pull request: %w", err)
	}

	klog.Infof("adding assignees %v", assignees)
	if err := pr.AddAssignees(ctx, assignees); err != nil {
		return fmt.Errorf("updating assignees: %w", err)
	}
	op.Changed = true

	return nil
}
