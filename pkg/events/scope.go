package events

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/GoogleContainerTools/kpt/tools/github-actions/pkg/permissions"
	"github.com/google/go-github/v53/github"
)

type Scope struct {
	Github *github.Client

	Repo  string
	Owner string

	mutex sync.Mutex
	// TODO: Cache more persistently?
	ownersByRef map[string]*permissions.Owners
}

func (s *Scope) CodeOwners(ctx context.Context, ref string) (*permissions.Owners, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.ownersByRef == nil {
		s.ownersByRef = make(map[string]*permissions.Owners)
	}

	// TODO: Enforce that ref is a sha?
	owner := s.ownersByRef[ref]
	if owner != nil {
		return owner, nil
	}

	owners := &permissions.Owners{}

	fileContent, _, _, err := s.Github.Repositories.GetContents(ctx, s.Owner, s.Repo, "CODEOWNERS", &github.RepositoryContentGetOptions{Ref: ref})
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

	s.ownersByRef[ref] = owners

	return owners, nil
}
