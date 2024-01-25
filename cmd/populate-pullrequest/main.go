package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/GoogleContainerTools/kpt/tools/github-actions/pkg/appinstall"
	"github.com/GoogleContainerTools/kpt/tools/github-actions/pkg/jwt"
	github "github.com/google/go-github/v53/github"
	"golang.org/x/oauth2"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

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
	clientOptions := client.Options{}
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

			// The events are very slow, sadly
			// if _, err := installScope.PollOrgEventsOnce(ctx); err != nil {
			// 	return fmt.Errorf("polling events for org %q: %w", org, err)
			// }

			// if _, err := installScope.PollRepoEventsOnce(ctx, "kptdev", "kpt"); err != nil {
			// 	return fmt.Errorf("polling events for org %q: %w", org, err)
			// }

			owner := "kptdev"
			repo := "kpt"
			if err := appInstall.PollForUpdates(ctx, owner, repo); err != nil {
				return fmt.Errorf("polling events for %v/%v: %w", owner, repo, err)
			}
		}
		time.Sleep(time.Minute)
	}

	return nil
}
