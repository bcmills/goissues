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
	"strings"

	"golang.org/x/build/maintner"
	"golang.org/x/build/maintner/godata"
)

// GitHub label IDs.
//
// Extract using:
// 	curl -sn https://api.github.com/repos/golang/go/labels/$LABELNAME | jq .id
const (
	go2ID                = 150880249
	documentationID      = 150880209
	earlyInCycleID       = 626114143
	featureRequestID     = 373540105
	helpWantedID         = 150880243
	needsDecisionID      = 373401956
	needsFixID           = 373399998
	needsInvestigationID = 373402289
	performanceID        = 150880191
	proposalID           = 236419512
	proposalHoldID       = 477156222
	releaseBlockerID     = 626114820
	soonID               = 936464699
	testingID            = 150880205
	toolSpeedID          = 358732225
	waitingForInfoID     = 357033853
	frozenDueToAgeID     = 398069301
)

// GitHub Milestone numbers for the golang/go repo.
//
// Extract using:
// 	curl -sn https://api.github.com/repos/golang/go/milestones | jq ".[] | select(.title == \"$MILESTONE\") | .id"
const (
	unplannedMilestone  = 6
	unreleasedMilestone = 22
	proposalMilestone   = 30
	go2Milestone        = 72
	gccgoMilestone      = 23
	gollvmMilestone     = 100
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

	repo := corpus.GitHub().Repo("golang", "go")
	if repo == nil {
		log.Fatal("github.com/golang/go not found")
	}

	issueHasCL := map[int32]bool{}
	err = project.ForeachOpenCL(func(cl *maintner.GerritCL) error {
		switch cl.Status {
		case "merged", "abandoned":
			return nil
		}
		hasRef := false
		for _, ref := range cl.GitHubIssueRefs {
			if ref.Repo == repo {
				hasRef = true
				break
			}
		}
		if !hasRef {
			return nil
		}
		if len(cl.Metas) >= 1 {
			meta := cl.Metas[len(cl.Metas)-1]
			for _, vote := range meta.LabelVotes()["Code-Review"] {
				if vote == -2 {
					return nil
				}
			}
		}
		for _, ref := range cl.GitHubIssueRefs {
			if ref.Repo == repo {
				issueHasCL[ref.Number] = true
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	w := csv.NewWriter(os.Stdout)

	err = repo.ForeachIssue(func(i *maintner.GitHubIssue) error {
		if i.NotExist || i.PullRequest || (i.Locked && i.HasLabelID(frozenDueToAgeID)) {
			return nil
		}

		number := strconv.FormatInt(int64(i.Number), 10)
		updated := i.Updated.Format("2006-01-02")

		state := ""
		switch {
		case i.Closed:
			state = "closed"
		case i.Locked:
			state = "locked"
		}

		when := ""
		if i.Milestone != nil {
			switch i.Milestone.Number {
			case unplannedMilestone:
				if i.HasLabelID(helpWantedID) {
					when = "help"
				} else {
					when = "unplanned"
				}
			case unreleasedMilestone:
				when = "unreleased"
			case proposalMilestone:
				when = "proposal"
			case go2Milestone:
				when = "go2"
			case gccgoMilestone:
				when = "gccgo"
			case gollvmMilestone:
				when = "gollvm"
			}
		}

		for _, l := range i.Labels {
			switch l.ID {
			case waitingForInfoID, proposalHoldID:
				switch state {
				case "", "deciding":
					state = "waiting"
				}
			case needsDecisionID:
				switch state {
				case "":
					state = "deciding"
				}

			case soonID:
				when = "soon"
			case releaseBlockerID:
				switch when {
				case "", "early", "feature", "performance", "test", "doc":
					if i.Milestone != nil {
						when = i.Milestone.Title
					} else {
						when = "release"
					}
				}
			case earlyInCycleID:
				switch when {
				case "", "feature", "performance", "test", "doc":
					when = "early"
				}
			case featureRequestID:
				switch when {
				case "", "performance", "test", "doc":
					when = "feature"
				}
			case performanceID, toolSpeedID:
				switch when {
				case "", "test", "doc":
					when = "performance"
				}
			case testingID:
				switch when {
				case "", "doc":
					when = "test"
				}
			case documentationID:
				switch when {
				case "":
					when = "doc"
				}
			}
		}

		if state == "" {
			if issueHasCL[i.Number] {
				state = "pending"
			} else {
				state = "open"
			}
		}

		var who strings.Builder
		for _, a := range i.Assignees {
			if a.Login == "" {
				continue
			}
			if who.Len() > 0 {
				who.WriteString(",")
			}
			who.WriteString(a.Login)
		}

		return w.Write([]string{number, updated, state, when, who.String(), i.Title})
	})
	if err != nil {
		log.Fatal(err)
	}

	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatal(err)
	}
}
