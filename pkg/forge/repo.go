package forge

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/GoogleContainerTools/kpt/tools/github-actions/pkg/permissions"
	"github.com/google/go-github/v53/github"
)

type Repo struct {
	githubClient *github.Client
	owner        string
	repoName     string

	mutex sync.Mutex
	// TODO: Cache more persistently?
	ownersByRef map[string]*permissions.Owners
}

func NewRepo(client *github.Client, owner string, name string) *Repo {
	return &Repo{
		githubClient: client,
		owner:        owner,
		repoName:     name,
	}
}

func (o *Repo) PullRequest(ctx context.Context, prNumber int) (*PullRequest, error) {
	obj, _, err := o.githubClient.PullRequests.Get(ctx, o.owner, o.repoName, prNumber)
	if err != nil {
		return nil, fmt.Errorf("getting pull request: %w", err)
	}
	return &PullRequest{
		githubClient: o.githubClient,
		repo:         o,
		prNumber:     prNumber,
		obj:          obj,
	}, nil
}

func (o *Repo) CodeOwners(ctx context.Context, ref string) (*permissions.Owners, error) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if o.ownersByRef == nil {
		o.ownersByRef = make(map[string]*permissions.Owners)
	}

	// TODO: Enforce that ref is a sha?
	owner := o.ownersByRef[ref]
	if owner != nil {
		return owner, nil
	}

	owners := &permissions.Owners{}

	fileContent, _, _, err := o.githubClient.Repositories.GetContents(ctx, o.owner, o.repoName, "CODEOWNERS", &github.RepositoryContentGetOptions{Ref: ref})
	if err != nil {
		return nil, fmt.Errorf("reading CODEOWNERS file: %w", err)
	}
	if fileContent == nil {
		return nil, fmt.Errorf("CODEOWNERS file content is nil: %w", err)
	}
	content, err := fileContent.GetContent()
	if err != nil {
		return nil, fmt.Errorf("reading CODEOWNERS file: %w", err)
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		owners.Owners = append(owners.Owners, line)
	}
	// TODO: HACK!
	owners.Owners = append(owners.Owners, "justinsb")
	owners.Owners = append(owners.Owners, "droot")

	o.ownersByRef[ref] = owners

	return owners, nil
}
