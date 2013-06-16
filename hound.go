// Hound is a command line tool to display developer's activity stream
package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/foize/go.sgr"
	"github.com/kennygrant/sanitize"
	"github.com/msbranco/goconfig"
	"github.com/plouc/gogithub"
	"github.com/plouc/gogitlab"
	"github.com/plouc/gojira"
	"os"
	"regexp"
	"sort"
	"time"
)

const (
	welcomeMsg   = "[bg-157]             [reset]\n" +
				   "[bg-157]  [reset]  [fg-157]HOUND[reset]  [bg-157]  [reset] [fg-157]V0.1\n" +
				   "[bg-157]             [reset]"
	errorWrapper = "[fg-196]%s[reset]"
	confWrapper  = "[fg-237]>[reset] [fg-94]%s[reset] [fg-87 bold]%s[reset]"
	confFilePath = ".houndfile"

	eventTemplate = "[bg-87 fg-16] %s [reset][fg-87 bg-178]⮀[reset][bg-178 fg-16] %- 6s [reset][fg-178]⮀[reset] [fg-157]%s"
)

type HoundEvent struct {
	Type    string
	On      time.Time
	Payload interface{}
}

type HoundEvents []*HoundEvent

func (s HoundEvents) Len() int      { return len(s) }
func (s HoundEvents) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type ByDate struct{ HoundEvents }

func (s ByDate) Less(i, j int) bool {
	return s.HoundEvents[i].On.After(s.HoundEvents[j].On)
}

func error(errMsg string) {
	fmt.Printf(sgr.MustParseln(errorWrapper), errMsg)
}

func getConfValue(c *goconfig.ConfigFile, section string, key string, desc string) string {
	v, err := c.GetString(section, key)
	if err != nil {
		error(err.Error())
		os.Exit(0)
	}
	//fmt.Printf(sgr.MustParseln(confWrapper), desc, v)

	return v
}

func ask(s *bufio.Scanner, question string) string {
	fmt.Printf("%s ", question)
	s.Scan()
	v := s.Text()
	if err := s.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
	}

	return v
}

var command      string
var githubUser   string
var jiraUser     string
var jiraFeedUser string

func init() {
	const (
		defaultGithubUser   = ""
		githubUserUsage     = "the github user"
		defaultJiraUser     = ""
		jiraUserUsage       = "the jira user name"
		defaultJiraFeedUser = ""
		jiraFeedUserUsage   = "the jira user name to filter activity feed"
		defaultCommand      = "history"
		commandUsage        = "the command to run, available: setup, history"
	)

	flag.StringVar(&command, "command", defaultCommand, commandUsage)
	flag.StringVar(&command, "c",       defaultCommand, commandUsage+" (shorthand)")

	flag.StringVar(&githubUser, "github_user", defaultGithubUser, githubUserUsage)
	flag.StringVar(&githubUser, "ghu",         defaultGithubUser, githubUserUsage+" (shorthand)")

	flag.StringVar(&jiraUser, "jira_user", defaultJiraUser, jiraUserUsage)
	flag.StringVar(&jiraUser, "ju",        defaultJiraUser, jiraUserUsage+" (shorthand)")

	flag.StringVar(&jiraFeedUser, "jira_feed_user", defaultJiraFeedUser, jiraFeedUserUsage)
	flag.StringVar(&jiraFeedUser, "jfu",            defaultJiraFeedUser, jiraFeedUserUsage+" (shorthand)")
}

type Config struct {
	gitlabBaseUrl      string
	gitlabApiPath      string
	gitlabRepoFeedPath string
	gitlabToken        string
	jiraBaseUrl        string
	jiraApiPath        string
	jiraActivityPath   string
}

func checkConfig() *Config {
	c, err := goconfig.ReadConfigFile(confFilePath)
	if err != nil {
		fmt.Println(sgr.MustParseln("[fg-94]Config file doesn't exists, please run[reset]\n[fg-237]>[reset] [fg-157 bold]./hound setup[reset]"))
		os.Exit(0)
	}

	// fetch gitlab config
	gitlabBaseUrl      := getConfValue(c, "gitlab", "baseUrl", "gitlab base url")
	gitlabApiPath      := getConfValue(c, "gitlab", "apiPath", "gitlab api path")
	gitlabRepoFeedPath := getConfValue(c, "gitlab", "repoFeedPath", "gitlab repository feed path")
	gitlabToken        := getConfValue(c, "gitlab", "token", "gitlab token")

	// fetch jira config
	jiraBaseUrl      := getConfValue(c, "jira", "baseUrl", "jira base url")
	jiraApiPath      := getConfValue(c, "jira", "apiPath", "jira api path")
	jiraActivityPath := getConfValue(c, "jira", "activityPath", "jira activity path")

	return &Config{
		gitlabBaseUrl:      gitlabBaseUrl,
		gitlabApiPath:      gitlabApiPath,
		gitlabRepoFeedPath: gitlabRepoFeedPath,
		gitlabToken:        gitlabToken,
		jiraBaseUrl:        jiraBaseUrl,
		jiraApiPath:        jiraApiPath,
		jiraActivityPath:   jiraActivityPath,
	}
}

func main() {
	fmt.Print(sgr.MustParseln(welcomeMsg))
	
	startedAt := time.Now()
	defer func() {
		fmt.Printf(sgr.MustParseln("\n[fg-237]> processed in [fg-157]%v"), time.Now().Sub(startedAt))
	}()

	flag.Parse()

	switch command {
	default:
		fmt.Println("No command defined, available commands: setup, history")
	case "setup":
		setup()
	case "history":
		config := checkConfig()
		history(config)
	}	
}

// Interactive command configuration
func setup() {
	fmt.Println(sgr.MustParse("[fg-87]hound interactive setup"))

	scanner := bufio.NewScanner(os.Stdin)

	// github
	ghUser := ask(scanner, sgr.MustParse("[fg-237]> [fg-87]github [fg-94]user"))

	// jira
	jiraBaseUrl := ask(scanner, sgr.MustParse("[fg-237]> [fg-87]jira [fg-94]base url"))
	jiraApiPath := ask(scanner, sgr.MustParse("[fg-237]> [fg-87]jira [fg-94]api path"))
	jiraActivityPath := ask(scanner, sgr.MustParse("[fg-237]> [fg-87]jira [fg-94]activity path"))
	jiraUser := ask(scanner, sgr.MustParse("[fg-237]> [fg-87]jira [fg-94]user"))
	jiraActivityUser := ask(scanner, sgr.MustParse("[fg-237]> [fg-87]jira [fg-94]activity user"))

	// creating config file
	c := goconfig.NewConfigFile()

	// github
	c.AddSection("github")
	c.AddOption("github", "user", ghUser)

	// jira
	c.AddSection("jira")
	c.AddOption("jira", "baseUrl", jiraBaseUrl)
	c.AddOption("jira", "apiPath", jiraApiPath)
	c.AddOption("jira", "user", jiraUser)
	c.AddOption("jira", "activityPath", jiraActivityPath)
	c.AddOption("jira", "activityUser", jiraActivityUser)

	c.WriteConfigFile(confFilePath, 0644, "Hound configuration file")

	fmt.Println(sgr.MustParse("[fg-87]Config file successfully created!\n"))
}

// History
func history(c *Config) {
	github := gogithub.NewGithub()
	gitlab := gogitlab.NewGitlab(c.gitlabBaseUrl, c.gitlabApiPath, c.gitlabRepoFeedPath, c.gitlabToken)
	jira   := gojira.NewJira(c.jiraBaseUrl, c.jiraApiPath, c.jiraActivityPath)

	githubUserEvents := github.UserPerformedEvents(githubUser)
	gitlabCommits    := gitlab.RepoCommits("56")
	gitlabActivity   := gitlab.RepoActivityFeed()
	jiraIssues       := jira.IssuesAssignedTo(jiraUser, 30, 0)
	jiraActivity     := jira.UserActivity(jiraFeedUser)

	events := make([]*HoundEvent, len(githubUserEvents)   + len(jiraIssues.Issues) +
								  len(jiraActivity.Entry) + len(gitlabCommits) +
								  len(gitlabActivity.Entry))

	i := 0

	for _, commit := range gitlabCommits {
		events[i] = &HoundEvent{"gitlab", commit.CreatedAt, commit}
		i = i + 1
	}

	for _, entry := range gitlabActivity.Entry {
		events[i] = &HoundEvent{"gitlab", entry.Updated, entry}
		i = i + 1
	}

	for _, event := range githubUserEvents {
		events[i] = &HoundEvent{"github", event.CreatedAt, event}
		i = i + 1
	}

	for _, issue := range jiraIssues.Issues {
		events[i] = &HoundEvent{"jira", issue.CreatedAt, issue}
		i = i + 1
	}

	for _, entry := range jiraActivity.Entry {
		events[i] = &HoundEvent{"jira", entry.Updated, entry}
		i = i + 1
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
			fmt.Printf(sgr.MustParseln(eventTemplate), event.On.Format("15:04"), event.Type, re.ReplaceAllString(sanitize.HTML(payload.Title), " "))

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
