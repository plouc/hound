package hound

import (
	"strconv"
	"fmt"
	"time"
	"github.com/foize/go.sgr"
)

type StatItem struct {
	Name        string
	Description string
}

type Stat struct {
	Name  string
	Stats []*StatItem
}

func (h *Hound) getStats() []*Stat {

	ch := make(chan *Stat)
  	stats := []*Stat{}

  	opCount := 0

  	if h.Config.Github.Active {
  		opCount = opCount + 1
  		go func() {
  			stat := Stat{}
			repos, err := h.Github.UserRepos(h.Config.Github.User)
			if err == nil {
				for _, repo := range repos {
					stat.Stats = append(stat.Stats, &StatItem{repo.Name, repo.Description})
				}
			}
			ch <- &stat
		}()
	}

	if h.Config.Gitlab.Active {
		opCount = opCount + 1
		go func() {
			stat := Stat{}
			projects, err := h.Gitlab.Projects()
			if err == nil {
				for _, project := range projects {
					stat.Stats = append(stat.Stats, &StatItem{project.Name, project.Description})
				}
			}
			ch <- &stat
		}()
	}

	if h.Config.Jira.Active {
		opCount = opCount + 1
		go func() {
			stat := Stat{}
			issues := h.Jira.IssuesAssignedTo(h.Config.Jira.User, 1000, 0)
			for _, issue := range issues.Issues {
				stat.Stats = append(stat.Stats, &StatItem{issue.Key, issue.Fields.Summary})
			}
			ch <- &stat
		}()
	}

	for {
	    select {
	    case stat := <-ch:
	        stats = append(stats, stat)
	        if len(stats) == opCount {
	        	return stats
	        }
	    case <-time.After(50 * time.Millisecond):
	        //fmt.Printf(".")
	    }
	}

	return stats
}

// Stats
func (h *Hound) Stats() {

	stats := h.getStats()

	for _, stat := range stats {
		fmt.Printf(sgr.MustParseln("[bg-94 fg-184] %- 80s "), "Github")
		fmt.Printf(sgr.MustParseln("[bg-87 fg-16] %- 80s "), strconv.Itoa(len(stat.Stats)) + " items")
		for _, statItem := range stat.Stats {
			fmt.Printf(sgr.MustParse("[bg-178 fg-16] %s [reset][fg-178]⮀[fg-157] %s\n"), statItem.Name, statItem.Description)
		}
	}
}