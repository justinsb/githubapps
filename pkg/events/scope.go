package events

import (
	"github.com/google/go-github/v53/github"
)

type Scope struct {
	Github *github.Client

	Repo  string
	Owner string
}
