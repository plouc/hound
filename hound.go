package hound

import (
	"github.com/plouc/go-github-client"
	"github.com/plouc/go-gitlab-client"
	"github.com/plouc/go-jira-client"
	"fmt"
	"time"
	"io/ioutil"
	"encoding/json"
	"regexp"
	"sort"
	"github.com/foize/go.sgr"
)

const (
	eventTemplate = "[bg-87 fg-16] %s [reset][fg-87 bg-178]⮀[reset][bg-178 fg-16] %- 6s [reset][fg-178]⮀[reset] [fg-157]%s"
)

type Hound struct {
	ConfigFilePath string
	Config         *Configuration
	Github         *gogithub.Github
	Gitlab         *gogitlab.Gitlab
	Jira           *gojira.Jira
}

type Configuration struct {
	Name   string
	Github *GithubConfig
	Gitlab *GitlabConfig
	Jira   *JiraConfig
}

type GithubConfig struct {
	Active bool
	User   string
}

type GitlabConfig struct {
	Active  bool
	BaseUrl string
	ApiPath string
	Token   string
}

type JiraConfig struct {
	Active       bool
	BaseUrl      string
	ApiPath      string
	User         string
	FeedPath     string
	FeedUser     string
	AuthType     string
	AuthLogin    string
	AuthPassword string
}

type Event struct {
	Type    string
	On      time.Time
	Payload interface{}
}

type Events []*Event

func (s Events) Len() int      { return len(s) }
func (s Events) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type ByDate struct{
	Events
}

func (s ByDate) Less(i, j int) bool {
	return s.Events[i].On.After(s.Events[j].On)
}

func (h *Hound) loadConfig() {

	var config *Configuration

	contents, err := ioutil.ReadFile(h.ConfigFilePath)
	if err != nil {
		panic(err.Error())
	}

	err = json.Unmarshal(contents, &config)
	if err != nil {
		panic(err.Error())
	}

	h.Config = config
}

func NewHound(configFilePath string) *Hound {

	hound := Hound{}

	hound.ConfigFilePath = configFilePath

	hound.loadConfig()

	hound.Github = gogithub.NewGithub()
	hound.Gitlab = gogitlab.NewGitlab(
		hound.Config.Gitlab.BaseUrl,
		hound.Config.Gitlab.ApiPath,
		hound.Config.Gitlab.Token)
	hound.Jira = gojira.NewJira(
		hound.Config.Jira.BaseUrl,
		hound.Config.Jira.ApiPath,
		hound.Config.Jira.FeedPath)

	return &hound
}

func (h *Hound) getHistoryEvents() [][]*Event {

	ch := make(chan []*Event)
  	allEvents := [][]*Event{}

  	opCount := 0

  	if h.Config.Github.Active {
  		opCount = opCount + 1
  		go func() {
  			//fmt.Println("fetching github user events")
  			events := []*Event{}
			githubUserEvents, err := h.Github.UserPerformedEvents(h.Config.Github.User)
			if err == nil {
				//fmt.Println("fetched github user events")
				for _, event := range githubUserEvents {
					events = append(events, &Event{"github", event.CreatedAt, event})
				}
			}
			ch <- events
		}()
	}

	if h.Config.Gitlab.Active {
		opCount = opCount + 2
		go func() {
			//fmt.Println("fetching gitlab commits")
			gitlabCommits, err := h.Gitlab.RepoCommits("56")
			events := []*Event{}
			if err == nil {
				//fmt.Println("fetched gitlab commits")
				for _, commit := range gitlabCommits {
					events = append(events, &Event{"github", commit.CreatedAt, commit})
				}
			}
			ch <- events
		}()

		go func() {
			//fmt.Println("fetching gitlab activity")
			gitlabActivity := h.Gitlab.RepoActivityFeed(h.Config.Gitlab.BaseUrl)
			//fmt.Println("fetched gitlab activity")
			events := []*Event{}
			for _, entry := range gitlabActivity.Entry {
				events = append(events, &Event{"gitlab", entry.Updated, entry})
			}
			ch <- events
		}()
	}
	
	if h.Config.Jira.Active {
		opCount = opCount + 2
		go func() {
			//fmt.Println("fetching jira issues")
			jiraIssues := h.Jira.IssuesAssignedTo(h.Config.Jira.User, 30, 0)
			//fmt.Println("fetched jira issues")
			events := []*Event{}
			for _, issue := range jiraIssues.Issues {
				events = append(events, &Event{"jira", issue.CreatedAt, issue})
			}
			ch <- events	
		}()

		go func() {
			//fmt.Println("fetching jira activity")
			jiraActivity := h.Jira.UserActivity(h.Config.Jira.FeedUser)
			//fmt.Println("fetched jira activity")
			events := []*Event{}
			for _, entry := range jiraActivity.Entry {
				events = append(events, &Event{"jira", entry.Updated, entry})
			}
			ch <- events
		}()
	}

	for {
	    select {
	    case events := <-ch:
	        allEvents = append(allEvents, events)
	        if len(allEvents) == opCount {
	        	return allEvents
	        }
	    case <-time.After(50 * time.Millisecond):
	        //fmt.Printf(".")
	    }
	}

	return allEvents
}

// History
func (h *Hound) History() {

	eventsSlices := h.getHistoryEvents()
	events := make([]*Event, 0)
	for _, eventsSlice := range eventsSlices {
		for _, event := range eventsSlice {
			events = append(events, event)
		}
	}

	sort.Sort(ByDate{events})

	now        := time.Now()
	currentDay := new(time.Time)

	re := regexp.MustCompile(" +")

	for _, event := range events {
		if event.On.YearDay() != currentDay.YearDay() {
			var dateStr string
			if event.On.YearDay() == now.YearDay() {
				dateStr = "Today"
			} else {
				dateStr = event.On.Format("Monday 02 January")
			}
			fmt.Printf(sgr.MustParseln("[bg-94 fg-184] %- 80s "), dateStr)
			currentDay = &event.On
		}

		switch T := event.Payload.(type) {
		default:
			fmt.Printf("unexpected type %T", T)

		case *gogithub.Event:
			payload := event.Payload.(*gogithub.Event)
			fmt.Printf(sgr.MustParseln(eventTemplate), event.On.Format("15:04"), event.Type, payload.Message(""))

		case *gojira.Issue:
			payload := event.Payload.(*gojira.Issue)
			fmt.Printf(sgr.MustParseln(eventTemplate), event.On.Format("15:04"), event.Type, payload.Key+" - "+payload.Fields.Summary)

		case *gojira.ActivityItem:
			payload := event.Payload.(*gojira.ActivityItem)
			fmt.Printf(sgr.MustParseln(eventTemplate), event.On.Format("15:04"), event.Type, re.ReplaceAllString(payload.Title, " "))

		case *gogitlab.Commit:
			payload := event.Payload.(*gogitlab.Commit)
			description := fmt.Sprintf(sgr.MustParse("%s - %s by [bold]%s"), payload.Short_Id, payload.Title, payload.Author_Name)
			fmt.Printf(sgr.MustParseln(eventTemplate), event.On.Format("15:04"), event.Type, description)

		case *gogitlab.FeedCommit:
			payload := event.Payload.(*gogitlab.FeedCommit)
			fmt.Printf(sgr.MustParseln(eventTemplate), event.On.Format("15:04"), event.Type, payload.Title)
		}
	}
}