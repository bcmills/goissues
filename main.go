// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// goissues exports issues from the golang/go project (via the Maintner mirror
// service) to CSV for analysis.
package main

import (
	"context"
	"encoding/csv"
	"log"
	"os"
	"strconv"

	"golang.org/x/build/maintner"
	"golang.org/x/build/maintner/godata"
)

func main() {
	corpus, err := godata.Get(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	project := corpus.Gerrit().Project("go.googlesource.com", "go")
	if project == nil {
		log.Fatal("go.googlesource.com/go not found")
	}

	issueHasCL := map[int32]bool{}
	err = project.ForeachOpenCL(func(cl *maintner.GerritCL) error {
		for _, ref := range cl.GitHubIssueRefs {
			issueHasCL[ref.Number] = true
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	repo := corpus.GitHub().Repo("golang", "go")
	if repo == nil {
		log.Fatal("github.com/golang/go not found")
	}

	w := csv.NewWriter(os.Stdout)

	err = repo.ForeachIssue(func(i *maintner.GitHubIssue) error {
		if i.NotExist || i.PullRequest {
			return nil
		}

		number := strconv.FormatInt(int64(i.Number), 10)
		updated := i.Updated.Format("2006-01-02")

		state := "open"
		if i.Closed {
			state = "closed"
		} else if issueHasCL[i.Number] {
			state = "pending"
		}

		when := ""
		for _, l := range i.Labels {
			switch l.Name {
			case "WaitingForInfo":
				if state != "closed" {
					state = "waiting"
				}
			case "NeedsDecision":
				if state != "closed" && state != "waiting" {
					state = "deciding"
				}

			case "release-blocker":
				if i.Milestone != nil {
					when = i.Milestone.Title
				} else {
					when = "release"
				}
			case "early-in-cycle":
				switch when {
				case "", "feature", "test", "doc":
					when = "early"
				}
			case "FeatureRequest":
				switch when {
				case "", "test", "doc":
					when = "feature"
				}
			case "Testing":
				switch when {
				case "", "doc":
					when = "test"
				}
			case "Documentation":
				switch when {
				case "":
					when = "doc"
				}
			}
		}

		return w.Write([]string{number, updated, state, when, i.Title})
	})
	if err != nil {
		log.Fatal(err)
	}

	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatal(err)
	}
}
