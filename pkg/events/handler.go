package events

import (
	"context"
)

type Handler interface {
}

type IssueCommentHandler interface {
	OnIssueCommentEvent(ctx context.Context, scope *Scope, ev *IssueCommentEvent) error
}

type IssueCommentEvent struct {
	CommentBody string

	PRAuthor      string
	CommentAuthor string
	PRNumber      int
}

type PullRequestHandler interface {
	OnPullRequestUpdated(ctx context.Context, scope *Scope, pr *PullRequest) error
}

type PullRequest struct {
	Number  int
	Title   string
	Author  string
	BaseRef string
	Merged  bool

	Comments []Comment
	Labels   []string
}

type Comment struct {
	Body   string
	Author string
}
