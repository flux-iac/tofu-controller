package webhook

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/jenkins-x/go-scm/scm"
)

func handlePush(log logr.Logger, hook scm.Webhook) error {
	event, ok := hook.(*scm.PushHook)
	if !ok {
		return fmt.Errorf("unable to cast type")
	}

	log.Info(
		"push event received",
		"sender", event.Sender.Login,
		"event", event,
	)

	return nil
}

func handlePullRequest(log logr.Logger, hook scm.Webhook) error {
	event, ok := hook.(*scm.PullRequestHook)
	if !ok {
		return fmt.Errorf("unable to cast type")
	}

	log.Info(
		"pullrequest event received",
		"sender", event.Sender.Login,
		"action", event.Action,
		"base_branch", event.PullRequest.Base.Ref,
		"head_branch", event.PullRequest.Head.Ref,
		"id", event.PullRequest.Number,
	)

	return nil
}

func handleComment(log logr.Logger, hook scm.Webhook) error {
	event, ok := hook.(*scm.IssueCommentHook)
	if !ok {
		return fmt.Errorf("unable to cast type")
	}

	log.Info(
		event.Comment.Body,
		"sender", event.Sender.Login,
		"action", event.Action,
		"issue_id", event.Issue.Number,
	)

	return nil
}
