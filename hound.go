// Hound is a command line tool to display developer's activity stream
package main

import (
	"fmt"
	"flag"
	"os"
	"time"
	"bufio"
	"sort"
	"github.com/foize/go.sgr"
	"github.com/msbranco/goconfig"
	"github.com/plouc/gogithub"
	"github.com/plouc/gojira"
	"github.com/kennygrant/sanitize"
	"regexp"
)

const (
	welcomeMsg   = "[bg-157]             [reset]\n[bg-157]  [reset]  [fg-157]HOUND[reset]  [bg-157]  [reset]\n[bg-157]             [reset]\n"
	errorWrapper = "[fg-196]%s[reset]"
	confWrapper  = "[fg-237]>[reset] [fg-94]%s[reset] [fg-87 bold]%s[reset]"
	confFilePath = ".houndfile"
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
	v, err := c.GetString(section, key);
  	if err != nil {
  		error(err.Error())
  		os.Exit(0)
  	}
  	//fmt.Printf(sgr.MustParseln(confWrapper), desc, v)

  	return v
}

func formatDate(date string) string {
	layout := "2006-01-02T15:04:05.000-0700"
    t, _ := time.Parse(layout, date)

	return t.Format("06/01/02 15:04")
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

// Interactive command configuration
func setup() {
	fmt.Println(sgr.MustParse("[fg-87]hound interactive setup"))

	scanner := bufio.NewScanner(os.Stdin)

	// github
 	ghApiUrl := ask(scanner, sgr.MustParse("[fg-237]> [fg-87]github [fg-94]api url"))
 	ghUser   := ask(scanner, sgr.MustParse("[fg-237]> [fg-87]github [fg-94]user"))

 	// jira
 	jiraBaseUrl      := ask(scanner, sgr.MustParse("[fg-237]> [fg-87]jira [fg-94]base url"))
 	jiraApiPath      := ask(scanner, sgr.MustParse("[fg-237]> [fg-87]jira [fg-94]api path"))
 	jiraActivityPath := ask(scanner, sgr.MustParse("[fg-237]> [fg-87]jira [fg-94]activity path"))
 	jiraUser         := ask(scanner, sgr.MustParse("[fg-237]> [fg-87]jira [fg-94]user"))
 	jiraActivityUser := ask(scanner, sgr.MustParse("[fg-237]> [fg-87]jira [fg-94]activity user"))

 	// creating config file
 	c := goconfig.NewConfigFile();

 	// github
  	c.AddSection("github")
  	c.AddOption("github", "apiUrl", ghApiUrl)
  	c.AddOption("github", "user", ghUser)

  	// jira
  	c.AddSection("jira")
  	c.AddOption("jira", "baseUrl", jiraBaseUrl)
  	c.AddOption("jira", "apiPath", jiraApiPath)
  	c.AddOption("jira", "user", jiraUser);
  	c.AddOption("jira", "activityPath", jiraActivityPath)
  	c.AddOption("jira", "activityUser", jiraActivityUser)

  	c.WriteConfigFile(confFilePath, 0644, "Hound configuration file");

  	fmt.Println(sgr.MustParse("[fg-87]Config file successfully created!\n"))
}

func main() {
	
	fmt.Print(sgr.MustParseln(welcomeMsg))

	startedAt := time.Now()

	flag.Parse()

	cmd := flag.Arg(0)
	switch cmd {
	case "setup":
		setup()
	}

    c, err := goconfig.ReadConfigFile(confFilePath);
    if err != nil {
    	fmt.Println(sgr.MustParseln("[fg-94]Config file doesn't exists, please run[reset]\n[fg-237]>[reset] [fg-157 bold]./hound setup[reset]"))
    	os.Exit(0)
    }
  	
  	// fetch github config
    githubApiUrl := getConfValue(c, "github", "apiUrl", "github apiUrl")
    githubUser   := getConfValue(c, "github", "user",   "github user")

    // fetch jira config
    jiraBaseUrl      := getConfValue(c, "jira", "baseUrl",      "jira base url")
 	jiraApiPath      := getConfValue(c, "jira", "apiPath",      "jira api path")
 	jiraUser         := getConfValue(c, "jira", "user",         "jira user")
 	jiraActivityPath := getConfValue(c, "jira", "activityPath", "jira activity path")
 	jiraActivityUser := getConfValue(c, "jira", "activityUser", "jira activity user")

	github := gogithub.NewGithub(githubApiUrl)
	jira   := gojira.NewJira(jiraBaseUrl, jiraApiPath, jiraActivityPath)

	githubUserEvents := github.UserPerformedEvents(githubUser)
	jiraIssues       := jira.IssuesAssignedTo(jiraUser, 30, 0)
	jiraActivity     := jira.UserActivity(jiraActivityUser)

	events := make([]*HoundEvent, len(githubUserEvents) + len(jiraIssues.Issues) + len(jiraActivity.Entry))

	i := 0

	for _, event := range githubUserEvents {
		events[i] = &HoundEvent{
			Type:    "github",
			On:      event.CreatedAt,
			Payload: event,
		}
		i = i + 1
	}

	for _, issue := range jiraIssues.Issues {
		events[i] = &HoundEvent{
			Type:    "jira",
			On:      issue.CreatedAt,
			Payload: issue,
		}
		i = i + 1
	}

	for _, entry := range jiraActivity.Entry {
		events[i] = &HoundEvent{
			Type:    "jira",
			On:      entry.Updated,
			Payload: entry,
		}
		i = i + 1
	}

	sort.Sort(ByDate{events})

	//now        := time.Now()
	currentDay := new(time.Time)

	re := regexp.MustCompile(" +")

	for _, event := range events {
		if event.On.YearDay() != currentDay.YearDay() {
			fmt.Printf(sgr.MustParseln("[bg-157 fg-16 bold] %- 80s "), event.On.Format("Monday 02 January"))
			currentDay = &event.On
		}
		switch T := event.Payload.(type) {
		default:
			fmt.Printf("unexpected type %T", T)
    	case *gogithub.Event:
    		payload := event.Payload.(*gogithub.Event)
    		fmt.Printf(sgr.MustParseln("[fg-87]%s [fg-94]%s [fg-157]%s"),
		               event.On.Format("15:04"), payload.Type, payload.Repo.Name)
    	case *gojira.Issue:
    		payload := event.Payload.(*gojira.Issue)
    		fmt.Printf(sgr.MustParseln("[fg-87]%s [fg-94]%s [fg-157]%s"),
				       event.On.Format("15:04"), payload.Key, payload.Fields.Summary)
    	case *gojira.ActivityItem:
    		payload := event.Payload.(*gojira.ActivityItem)
    		fmt.Printf(sgr.MustParseln("[fg-87]%s [fg-157]%s"),
				       event.On.Format("15:04"), re.ReplaceAllString(sanitize.HTML(payload.Title), " "))
    	}
	}

	endedAt := time.Now()
	fmt.Printf(sgr.MustParseln("\n[fg-237]> processed in [fg-157]%v"), endedAt.Sub(startedAt))
}
